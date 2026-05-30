package chunker

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"memoria/memory/store"
	"strings"
)

type sentence struct {
	text     string
	startPos int64
	endPos   int64
}

type SemanticChunker struct {
	embedService    store.EmbeddingService
	windowSize      int64
	thresholdFactor float64
	minChunkTokens  int64
}

func NewSemanticChunker(embServ store.EmbeddingService,
	windowSize int64,
	thresholdFactor float64,
	minChunkTokens int64) (Chunker, error) {
	if embServ == nil {
		return nil, errors.New("EmbeddingService is nil")
	}
	ch := &SemanticChunker{
		embedService:    embServ,
		windowSize:      windowSize,
		thresholdFactor: thresholdFactor,
		minChunkTokens:  minChunkTokens,
	}
	if ch.windowSize == 0 {
		ch.windowSize = 1
	}
	if ch.thresholdFactor == 0 {
		ch.thresholdFactor = 1
	}
	if ch.minChunkTokens == 0 {
		ch.minChunkTokens = 50
	}
	return ch, nil
}

func (sc *SemanticChunker) Chunk(content string, opts ChunkOptions) ([]Chunk, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return nil, ErrEmptyContent
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 512
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 0
	}
	if opts.ChunkOverlap >= opts.ChunkSize {
		opts.ChunkOverlap = opts.ChunkSize / 4
	}

	chunks := sc.semanticSplit(content, 0, opts)

	for i := range chunks {
		chunks[i].Index = i
	}
	applyOverlap(chunks, opts.ChunkOverlap)

	return chunks, nil
}

// semanticSplit 对一段文本执行语义边界分块
func (sc *SemanticChunker) semanticSplit(content string, startPos int64, opts ChunkOptions) []Chunk {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	tokens := estimateTokens(content)
	if tokens <= opts.ChunkSize {
		return []Chunk{
			{
				Content:  content,
				TokenEst: tokens,
				StartPos: int(startPos),
				EndPos:   int(startPos) + len([]rune(content)),
			},
		}
	}

	// Step 1: 句子原子化
	sentences := sc.splitSentences(content)
	if len(sentences) <= 1 {
		// 无法做语义分割，回退到段落/行/字符硬切
		var fallback []Chunk
		splitIntoChunks(&fallback, content, int(startPos), "", opts)
		return fallback
	}
	slog.Debug("split sentences complete", "count", len(sentences))

	// Step 2: 句子向量化
	ctx := context.Background()
	vectors, err := sc.embedSentences(ctx, sentences)
	if err != nil || len(vectors) != len(sentences) {
		// embedding 失败，回退
		var fallback []Chunk
		splitIntoChunks(&fallback, content, int(startPos), "", opts)
		return fallback
	}

	// Step 3-4: 计算相邻语义距离，寻找断点
	breakpoints := sc.findBreakpoints(vectors, sentences)
	if len(breakpoints) <= 2 {
		// 未找到语义断点，回退硬切
		var fallback []Chunk
		splitIntoChunks(&fallback, content, int(startPos), "", opts)
		return fallback
	}

	// Step 5: 动态重组成 chunk
	return sc.buildChunksFromSentences(sentences, breakpoints, startPos, opts)
}

// splitSentences 按句末标点将文本拆分为句子序列
func (sc *SemanticChunker) splitSentences(content string) []sentence {
	runes := []rune(content)
	n := len(runes)
	if n == 0 {
		return nil
	}

	var sentences []sentence
	sentStart := 0

	for i := 0; i < n; i++ {
		r := runes[i]
		var boundary bool
		nextStart := i + 1

		if r == '。' || r == '！' || r == '？' || r == '；' {
			// CJK 句末标点 + 分号（法律/散文中的并列独立从句边界）
			boundary = true
		} else if r == '…' {
			// 中文省略号，表示语气延宕或话题转换
			boundary = true
		} else if r == '—' && i+1 < n && runes[i+1] == '—' {
			// 中文破折号 ——，语义转折或插入语
			boundary = true
			nextStart = i + 2
		} else if r == '.' && i+2 < n && runes[i+1] == '.' && runes[i+2] == '.' {
			// 英文省略号 ...
			boundary = true
			nextStart = i + 3
		} else if r == '.' || r == '!' || r == '?' {
			// 英文句末标点，需判断是否是真正的句子边界
			if r == '.' && i > 0 && isDigitRune(runes[i-1]) {
				// 数字中的小数点，跳过
				continue
			}
			boundary, nextStart = englishSentenceEnd(runes, i, n)
		} else if r == '\n' && i+1 < n && runes[i+1] == '\n' {
			// 段落分隔，强制断句
			boundary = true
			nextStart = i + 2
		} else if r == '\n' && i > sentStart {
			// 单换行也是句子边界（非紧随标点后的空句）
			boundary = true
		}

		if boundary && nextStart > sentStart {
			sentences = append(sentences, sentence{
				text:     string(runes[sentStart:nextStart]),
				startPos: int64(sentStart),
				endPos:   int64(nextStart),
			})
			sentStart = nextStart
			i = nextStart - 1 // 循环 i++ 后将指向 nextStart
		}
	}

	// 最后一个句子（剩余文本）
	if sentStart < n {
		end := n
		for end > sentStart && isWhitespaceRune(runes[end-1]) {
			end--
		}
		if end > sentStart {
			sentences = append(sentences, sentence{
				text:     string(runes[sentStart:end]),
				startPos: int64(sentStart),
				endPos:   int64(end),
			})
		}
	}

	return sentences
}

// englishSentenceEnd 判断英文句末标点后是否是真正的句子边界
// 策略：跳过 whitespace 后，如果是大写字母、换行、或文本末尾，则为边界
func englishSentenceEnd(runes []rune, punctPos, n int) (boundary bool, nextStart int) {
	nextStart = punctPos + 1
	for nextStart < n && (runes[nextStart] == ' ' || runes[nextStart] == '\t') {
		nextStart++
	}
	if nextStart >= n {
		return true, nextStart
	}
	if runes[nextStart] == '\n' {
		return true, nextStart
	}
	if isUpperRune(runes[nextStart]) {
		return true, nextStart
	}
	return false, 0
}

// embedSentences 批量对句子做向量化，支持滑动窗口上下文
func (sc *SemanticChunker) embedSentences(ctx context.Context, sentences []sentence) ([][]float64, error) {
	r := sc.windowSize - 1
	n := int64(len(sentences))

	texts := make([]string, len(sentences))
	for i := range sentences {
		start := max(0, int64(i)-r)
		end := min(n-1, int64(i)+r)
		if start == end {
			texts[i] = sentences[i].text
		} else {
			var sb strings.Builder
			for j := start; j <= end; j++ {
				if j > start {
					sb.WriteByte('\n')
				}
				sb.WriteString(sentences[j].text)
			}
			texts[i] = sb.String()
		}
	}

	// 分批调用 EmbedBatch，每批最多 256 条
	const batchLimit = 256
	var allVectors [][]float64
	for i := 0; i < len(texts); i += batchLimit {
		end := min(i+batchLimit, len(texts))
		batch := texts[i:end]
		vectors, err := sc.embedService.EmbedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		allVectors = append(allVectors, vectors...)
	}
	return allVectors, nil
}

// findBreakpoints 计算相邻余弦相似度，使用自适应阈值寻找语义断点
func (sc *SemanticChunker) findBreakpoints(vectors [][]float64, sentences []sentence) []int {
	n := len(vectors)
	if n <= 2 {
		return []int{0, n}
	}

	// 计算相邻句子向量的余弦相似度
	similarities := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		similarities[i] = cosine(vectors[i], vectors[i+1])
	}

	// 计算全局均值和标准差
	var sum, sumSq float64
	for _, s := range similarities {
		sum += s
		sumSq += s * s
	}
	m := float64(len(similarities))
	mean := sum / m
	variance := (sumSq - sum*sum/m) / m
	if variance < 0 {
		variance = 0
	}
	std := math.Sqrt(variance)

	// 语义均匀，不分割
	if std < 0.01 {
		return []int{0, n}
	}

	// 动态阈值：低于 μ - factor × σ 处标记为断点
	threshold := mean - sc.thresholdFactor*std

	breakpoints := []int{0}
	for i, sim := range similarities {
		if sim < threshold {
			breakpoints = append(breakpoints, i+1)
		}
	}
	breakpoints = append(breakpoints, n)

	// 合并过短的段
	return sc.mergeShortSegments(breakpoints, sentences)
}

// mergeShortSegments 合并 token 数不足 minChunkTokens 的段到下一段
func (sc *SemanticChunker) mergeShortSegments(breakpoints []int, sentences []sentence) []int {
	if len(breakpoints) <= 2 {
		return breakpoints
	}
	if sc.minChunkTokens <= 0 {
		return breakpoints
	}

	result := make([]int, 1, len(breakpoints))
	result[0] = 0

	for i := 1; i < len(breakpoints)-1; i++ {
		segStart := result[len(result)-1]
		segEnd := breakpoints[i]
		segText := joinSentences(sentences, segStart, segEnd)
		if estimateTokens(segText) < int(sc.minChunkTokens) {
			// 段太短，跳过该断点将其合并到下一段
			continue
		}
		result = append(result, segEnd)
	}
	result = append(result, len(sentences))
	return result
}

// buildChunksFromSentences 根据断点将句子重组为 chunk
func (sc *SemanticChunker) buildChunksFromSentences(sentences []sentence,
	breakpoints []int, startPos int64, opts ChunkOptions) []Chunk {

	var chunks []Chunk

	for k := 0; k < len(breakpoints)-1; k++ {
		segStart := breakpoints[k]
		segEnd := breakpoints[k+1]

		groupSentences := sentences[segStart:segEnd]
		groupText := joinSentences(groupSentences, 0, len(groupSentences))
		groupTok := estimateTokens(groupText)
		absStart := int(startPos) + int(sentences[segStart].startPos)

		if groupTok <= opts.ChunkSize {
			chunks = append(chunks, Chunk{
				Content:  groupText,
				TokenEst: groupTok,
				StartPos: absStart,
				EndPos:   absStart + len([]rune(groupText)),
			})
			continue
		}

		// 超限 chunk，递归语义分割
		if len(groupSentences) >= 2 {
			subChunks := sc.semanticSplit(groupText, int64(absStart), opts)
			chunks = append(chunks, subChunks...)
		} else {
			// 单个句子仍超限，回退硬切
			var fallback []Chunk
			splitIntoChunks(&fallback, groupText, absStart, "", opts)
			chunks = append(chunks, fallback...)
		}
	}

	return chunks
}

// joinSentences 将 sentences[start:end] 的 text 拼接为完整文本
func joinSentences(sentences []sentence, start, end int) string {
	if start >= end {
		return ""
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(sentences[i].text)
	}
	return sb.String()
}

// cosine 计算两个向量的余弦相似度
func cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func isDigitRune(r rune) bool {
	return r >= '0' && r <= '9'
}

func isUpperRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'А' && r <= 'Я')
}

func isWhitespaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
