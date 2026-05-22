package chunker

import (
	"strings"
)

type RecursiveChunker struct{}

func NewRecursiveChunker() *RecursiveChunker {
	return &RecursiveChunker{}
}

func (r *RecursiveChunker) Chunk(content string, opts ChunkOptions) ([]Chunk, error) {
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

	headings := parseHeadings(content)
	buildContentRanges(headings, len([]rune(content)))

	var chunks []Chunk
	if len(headings) == 0 {
		splitIntoChunks(&chunks, content, 0, "", opts)
	} else {
		// 虚拟根节点：包住所有标题，确保同级标题都被处理
		minLevel := headings[0].Level
		for _, h := range headings {
			if h.Level < minLevel {
				minLevel = h.Level
			}
		}
		virtualRoot := &headingNode{
			Level:        minLevel - 1,
			ContentStart: 0,
			ContentEnd:   len([]rune(content)),
		}
		allNodes := append([]*headingNode{virtualRoot}, headings...)
		chunkNode(&chunks, 0, &allNodes, content, "", opts)
	}

	for i := range chunks {
		chunks[i].Index = i
	}
	applyOverlap(chunks, opts.ChunkOverlap)

	return chunks, nil
}

type headingNode struct {
	Level        int
	Title        string
	LineStart    int // start of heading line in original content
	ContentStart int // after heading line
	ContentEnd   int
}

func parseHeadings(content string) []*headingNode {
	text := []rune(content)
	var nodes []*headingNode
	pos := 0

	for pos < len(text) {
		lineStart := pos
		lineEnd := scanLine(text, pos)

		level, title, ok := scanHeadingLine(text, lineStart, lineEnd)
		if ok {
			nodes = append(nodes, &headingNode{
				Level:        level,
				Title:        title,
				LineStart:    lineStart,
				ContentStart: lineEnd,
			})
		}
		pos = lineEnd
	}
	return nodes
}

func scanLine(text []rune, start int) int {
	for i := start; i < len(text); i++ {
		if text[i] == '\n' {
			return i + 1
		}
	}
	return len(text)
}

func scanHeadingLine(text []rune, start, end int) (level int, title string, ok bool) {
	i := start
	for i < end && text[i] == '#' {
		i++
	}
	level = i - start
	if level < 1 || level > 6 || i >= end || text[i] != ' ' {
		return 0, "", false
	}
	i++ // skip space
	title = string(text[i : end-1])
	title = strings.TrimSpace(title)
	return level, title, true
}

func buildContentRanges(nodes []*headingNode, totalRunes int) {
	for i := range nodes {
		nodes[i].ContentEnd = totalRunes
		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].Level <= nodes[i].Level {
				nodes[i].ContentEnd = nodes[j].LineStart
				break
			}
		}
	}
}

func chunkNode(chunks *[]Chunk, idx int, all *[]*headingNode, content string, parentPath string, opts ChunkOptions) {
	node := (*all)[idx]
	text := []rune(content)

	// 当前节点的完整路径（含自身标题），虚拟根节点（Title 为空）不加路径
	localPath := parentPath
	if node.Title != "" {
		prefix := strings.Repeat("#", node.Level)
		localPath += prefix + " " + node.Title + "\n"
	}

	totalRunes := node.ContentEnd - node.ContentStart
	if totalRunes <= 0 {
		return
	}

	// 节点内容能放入一个 chunk
	if estimateRuneTokens(totalRunes) <= opts.ChunkSize {
		*chunks = append(*chunks, Chunk{
			Content:  localPath + string(text[node.ContentStart:node.ContentEnd]),
			TokenEst: estimateRuneTokens(totalRunes),
			StartPos: node.ContentStart,
			EndPos:   node.ContentEnd,
		})
		return
	}

	// 有子标题 → 递归
	myChildren := directChildren(idx, all)
	if len(myChildren) > 0 {
		// 第一个子标题之前的引导文本
		firstChild := (*all)[myChildren[0]]
		if node.ContentStart < firstChild.ContentStart {
			gap := string(text[node.ContentStart:firstChild.ContentStart])
			splitIntoChunks(chunks, gap, node.ContentStart, localPath, opts)
		}
		// 子标题之间（含第一个）
		for _, ci := range myChildren {
			chunkNode(chunks, ci, all, content, localPath, opts)
		}
		// 最后一个子标题之后的尾部文本
		lastIdx := myChildren[len(myChildren)-1]
		tailStart := (*all)[lastIdx].ContentEnd
		if tailStart < node.ContentEnd {
			tail := string(text[tailStart:node.ContentEnd])
			splitIntoChunks(chunks, tail, tailStart, localPath, opts)
		}
		return
	}

	// 无子标题 → 按段落/行/字符切
	body := string(text[node.ContentStart:node.ContentEnd])
	splitIntoChunks(chunks, body, node.ContentStart, localPath, opts)
}

func directChildren(idx int, all *[]*headingNode) []int {
	parent := (*all)[idx]
	var children []int
	for i := range *all {
		node := (*all)[i]
		if node.ContentStart >= parent.ContentStart &&
			node.ContentEnd <= parent.ContentEnd &&
			node.Level == parent.Level+1 {
			children = append(children, i)
		}
	}
	return children
}

func splitIntoChunks(chunks *[]Chunk, body string, startPos int, path string, opts ChunkOptions) {
	if strings.TrimSpace(body) == "" {
		return
	}
	if estimateTokens(body) <= opts.ChunkSize {
		*chunks = append(*chunks, Chunk{
			Content:  path + body,
			TokenEst: estimateTokens(body),
			StartPos: startPos,
			EndPos:   startPos + len([]rune(body)),
		})
		return
	}

	// 按段落 (\n\n) 切
	paras := splitParagraphs(body)
	if len(paras) > 1 {
		emitParagraphChunks(chunks, paras, startPos, path, opts)
		return
	}

	// 按行 (\n) 切
	lines := strings.Split(body, "\n")
	if len(lines) > 1 {
		emitLineChunks(chunks, lines, startPos, path, opts)
		return
	}

	// 硬切
	emitCharChunks(chunks, body, startPos, path, opts)
}

func splitParagraphs(body string) []string {
	var result []string
	rest := body
	for {
		idx := strings.Index(rest, "\n\n")
		if idx < 0 {
			if rest != "" {
				result = append(result, rest)
			}
			break
		}
		if idx > 0 {
			result = append(result, rest[:idx])
		}
		rest = rest[idx+2:]
	}
	return result
}

func emitParagraphChunks(chunks *[]Chunk, paras []string, startPos int, path string, opts ChunkOptions) {
	var buf strings.Builder
	bufStart := startPos
	pos := startPos

	for i, para := range paras {
		sep := ""
		if buf.Len() > 0 {
			sep = "\n\n"
		}
		if estimateTokens(buf.String()+sep+para) > opts.ChunkSize && buf.Len() > 0 {
			*chunks = append(*chunks, makeChunk(path, buf.String(), bufStart, opts))
			buf.Reset()
			bufStart = pos
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(para)
		pos += len([]rune(para))
		if i < len(paras)-1 {
			pos += 2 // \n\n 分隔符
		}
	}
	if buf.Len() > 0 {
		*chunks = append(*chunks, makeChunk(path, buf.String(), bufStart, opts))
	}
}

func emitLineChunks(chunks *[]Chunk, lines []string, startPos int, path string, opts ChunkOptions) {
	var buf strings.Builder
	bufStart := startPos
	pos := startPos

	for i, line := range lines {
		sep := ""
		if buf.Len() > 0 {
			sep = "\n"
		}
		if estimateTokens(buf.String()+sep+line) > opts.ChunkSize && buf.Len() > 0 {
			*chunks = append(*chunks, makeChunk(path, buf.String(), bufStart, opts))
			buf.Reset()
			bufStart = pos
		}
		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(line)
		pos += len([]rune(line))
		if i < len(lines)-1 {
			pos += 1 // \n 分隔符
		}
	}
	if buf.Len() > 0 {
		*chunks = append(*chunks, makeChunk(path, buf.String(), bufStart, opts))
	}
}

func emitCharChunks(chunks *[]Chunk, body string, startPos int, path string, opts ChunkOptions) {
	runes := []rune(body)
	targetTokens := opts.ChunkSize
	if targetTokens < 10 {
		targetTokens = 10
	}

	i := 0
	for i < len(runes) {
		end := i
		tok := 0
		// 逐字累加，直到 token 数达标
		for end < len(runes) && tok < targetTokens {
			if isCJK(runes[end]) {
				tok++
			} else if runes[end] == ' ' || runes[end] == '\t' || runes[end] == '\n' {
				// whitespace 不计 token
			} else {
				// 非 CJK 字符：按单词边界计数
				if end == i || runes[end-1] == ' ' || runes[end-1] == '\n' || isCJK(runes[end-1]) {
					tok++
				}
			}
			end++
		}
		if end == i {
			end = i + 1
		}
		// 往后微调到最近的句号
		if end < len(runes) {
			lookEnd := end + targetTokens
			if lookEnd > len(runes) {
				lookEnd = len(runes)
			}
			for j := end; j < lookEnd; j++ {
				if runes[j] == '。' || runes[j] == '.' || runes[j] == '\n' || runes[j] == '！' || runes[j] == '？' {
					end = j + 1
					break
				}
			}
		}
		*chunks = append(*chunks, makeChunk(path, string(runes[i:end]), startPos+i, opts))
		i = end
	}
}

func makeChunk(path, body string, startPos int, opts ChunkOptions) Chunk {
	return Chunk{
		Content:  path + body,
		TokenEst: estimateTokens(body),
		StartPos: startPos,
		EndPos:   startPos + len([]rune(body)),
	}
}

func applyOverlap(chunks []Chunk, overlapSize int) {
	if overlapSize <= 0 || len(chunks) < 2 {
		return
	}
	for i := 1; i < len(chunks); i++ {
		prevContent := extractBody(chunks[i-1].Content)
		overlapText := tailRunes(prevContent, overlapSize*2)
		if overlapText == "" {
			continue
		}
		currBody := extractBody(chunks[i].Content)
		currPath := extractPath(chunks[i].Content)
		chunks[i].Content = currPath + overlapText + " " + currBody
		chunks[i].TokenEst = estimateTokens(chunks[i].Content)
		chunks[i].StartPos = chunks[i-1].EndPos - len([]rune(overlapText))
	}
}

// extractPath 提取 heading path（# 标题 \n 前缀部分）
func extractPath(chunkContent string) string {
	// path 形如 "# 标题\n## 子标题\n" 以 \n 分隔
	// body 从第一个非 # 行开始
	lines := strings.Split(chunkContent, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "#") {
			if i > 0 {
				return strings.Join(lines[:i], "\n") + "\n"
			}
			return ""
		}
	}
	return ""
}

func extractBody(chunkContent string) string {
	lines := strings.Split(chunkContent, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "#") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return chunkContent
}

func tailRunes(s string, n int) string {
	runes := []rune(s)
	if n >= len(runes) {
		return s
	}
	return string(runes[len(runes)-n:])
}

func estimateTokens(text string) int {
	cjk := 0
	nonCJKWords := 0
	inWord := false

	for _, r := range text {
		if isCJK(r) {
			cjk++
			inWord = false
		} else if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			inWord = false
		} else {
			if !inWord {
				nonCJKWords++
				inWord = true
			}
		}
	}
	n := cjk + nonCJKWords
	if n < 1 {
		n = 1
	}
	return n
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK统一汉字
		(r >= 0x3400 && r <= 0x4DBF) || // CJK扩展A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK扩展B
		(r >= 0x2A700 && r <= 0x2B73F) || // CJK扩展C
		(r >= 0x2B740 && r <= 0x2B81F) || // CJK扩展D
		(r >= 0x2B820 && r <= 0x2CEAF) || // CJK扩展E
		(r >= 0xF900 && r <= 0xFAFF) // CJK兼容汉字
}

func estimateRuneTokens(runeCount int) int {
	// 回退，用于仅有 rune count 的场景
	return max(1, runeCount/2)
}
