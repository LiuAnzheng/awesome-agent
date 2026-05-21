package retrieval

import (
	"math"
	"sort"
)

// BM25 参数默认值（Lucene 标准值，适用于绝大多数场景）
const (
	defaultK1 = 1.5  // 控制 TF 饱和速度，值越大高频词加分越多
	defaultB  = 0.75 // 控制文档长度归一化力度，0=不归一化，1=完全归一化
)

type ScoredDoc struct {
	DocID int     // BuildIndex 时传入的 doc 下标
	Score float64 // BM25 得分，越高越相关
}

// BM25Scorer BM25 评分器。
// BM25 是一种基于词频的排序算法，核心思想：
//  1. 一个词在文档中出现次数越多（TF 高），该文档越相关 —— 但有上限（饱和）
//  2. 一个词在越少文档中出现（DF 低），该词区分度越高（IDF 大）
//  3. 长文档天然词多，需要惩罚：短文档中匹配到查询词更"难得"
type BM25Scorer struct {
	k1, b     float64    // 可调参数
	tokenizer *Tokenizer // 分词器

	// 索引期间计算的统计量
	avgDocLen   float64                    // 所有文档的平均 token 数
	invertedIdx map[string]map[int]float64 // 倒排索引: term -> {docID: 该词在此文档中的出现次数(TF)}
	docLengths  map[int]int                // 每个文档的 token 总数
	docs        []string                   // 文档原始文本（保留引用，一般仅调试用）
	docCount    int                        // 文档总数（即 len(docs)）
}

func NewBM25Scorer(tokenizer *Tokenizer) *BM25Scorer {
	return &BM25Scorer{
		k1:          defaultK1,
		b:           defaultB,
		tokenizer:   tokenizer,
		invertedIdx: make(map[string]map[int]float64),
		docLengths:  make(map[int]int),
	}
}

// BuildIndex 全量重建倒排索引。
// docs 是所有文档文本，doc 在 Search 返回结果中通过 ScoredDoc.DocID 定位。
func (s *BM25Scorer) BuildIndex(docs []string) {
	// 清空旧索引
	s.invertedIdx = make(map[string]map[int]float64)
	s.docLengths = make(map[int]int)
	s.docs = docs
	s.docCount = len(docs)

	totalLen := 0
	for docID, doc := range docs {
		// 索引模式分词：尽可能多切词以提升召回
		tokens := s.tokenizer.CutForIndex(doc)
		s.docLengths[docID] = len(tokens)
		totalLen += len(tokens)

		// 统计本文档内每个词的词频（TF = Term Frequency）
		tf := make(map[string]float64)
		for _, t := range tokens {
			tf[t]++
		}

		// 写入倒排索引: term -> {docID: tf}
		for term, freq := range tf {
			if s.invertedIdx[term] == nil {
				s.invertedIdx[term] = make(map[int]float64)
			}
			s.invertedIdx[term][docID] = freq
		}
	}

	if s.docCount > 0 {
		s.avgDocLen = float64(totalLen) / float64(s.docCount)
	}
}

// Search 按 BM25 算法对查询进行评分，返回 TopK 结果（按得分降序）。
func (s *BM25Scorer) Search(query string, topK int) []ScoredDoc {
	if s.docCount == 0 {
		return nil
	}

	// 查询模式分词：精确切分，减少噪声词
	queryTokens := s.tokenizer.CutForQuery(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// 查询词去重，同一个词不重复算分
	seen := make(map[string]bool)
	uniqueTokens := make([]string, 0, len(queryTokens))
	for _, t := range queryTokens {
		if !seen[t] {
			seen[t] = true
			uniqueTokens = append(uniqueTokens, t)
		}
	}

	// 对每个"文档-查询词"对累加 BM25 得分
	scores := make(map[int]float64)
	for _, term := range uniqueTokens {
		posting, ok := s.invertedIdx[term] // 拿到包含该词的所有文档
		if !ok {
			continue // 所有文档都不含这个词，跳过
		}

		// IDF: 包含该词的文档越少，该词区分度越高
		// 标准 IDF 公式，+0.5 平滑避免 df=N 时 log(0)
		df := float64(len(posting))
		idf := math.Log((float64(s.docCount)-df+0.5)/(df+0.5) + 1)

		for docID, tf := range posting {
			docLen := float64(s.docLengths[docID])

			// BM25 核心公式：IDF × (TF归一化)
			// 分子: tf * (k1+1)     —— 增大 k1 会让高频词获得更多加分
			// 分母: tf + 惩罚项      —— 文档越长惩罚越大（由 b 控制力度）
			numerator := tf * (s.k1 + 1)
			denominator := tf + s.k1*(1-s.b+s.b*docLen/s.avgDocLen)
			scores[docID] += idf * numerator / denominator
		}
	}

	// 转为切片并按得分降序排列
	scored := make([]ScoredDoc, 0, len(scores))
	for docID, score := range scores {
		scored = append(scored, ScoredDoc{DocID: docID, Score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if topK > 0 && len(scored) > topK {
		scored = scored[:topK]
	}

	return scored
}

// Score 计算单个文档对某个查询的 BM25 得分。
func (s *BM25Scorer) Score(query string, docID int) float64 {
	if docID < 0 || docID >= s.docCount {
		return 0
	}

	queryTokens := s.tokenizer.CutForQuery(query)
	if len(queryTokens) == 0 {
		return 0
	}

	docLen := float64(s.docLengths[docID])
	seen := make(map[string]bool)
	var totalScore float64

	for _, term := range queryTokens {
		if seen[term] {
			continue
		}
		seen[term] = true

		posting, ok := s.invertedIdx[term]
		if !ok {
			continue
		}

		tf, ok := posting[docID]
		if !ok {
			continue
		}

		df := float64(len(posting))
		idf := math.Log((float64(s.docCount)-df+0.5)/(df+0.5) + 1)
		numerator := tf * (s.k1 + 1)
		denominator := tf + s.k1*(1-s.b+s.b*docLen/s.avgDocLen)
		totalScore += idf * numerator / denominator
	}

	return totalScore
}

func (s *BM25Scorer) DocCount() int {
	return s.docCount
}
