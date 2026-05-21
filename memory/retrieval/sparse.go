package retrieval

import "math"

// SparseVector 稀疏向量：词 → 权重。
// 与稠密向量（[]float64, 几百上千维的浮点数组）不同，稀疏向量只存储非零维度，
// 维度就是词本身，权重 = 该词在当前上下文中的重要性（TF-IDF 或 IDF）。
//
// 示例：文档 "OpenAI API 鉴权规范" 的稀疏向量可能是：
//
//	{"openai": 0.35, "api": 0.28, "鉴权": 0.42, "规范": 0.15}
//
// 稀疏 vs 稠密的核心区别：
//   - 稀疏向量：维度=词，可解释（每个维度是一个具体的词），适合精确关键词匹配
//   - 稠密向量：维度=模型隐空间，不可解释，适合语义相似度匹配
type SparseVector map[string]float64

// CosineSimilarity 计算两个稀疏向量的余弦相似度。
//
// 余弦相似度 = 向量夹角，值域 [-1, 1]，越接近 1 方向越一致。
// 公式：cosθ = (a·b) / (|a| × |b|)
//
// 直观理解：两个文档共有的词越多且权重越接近，相似度越高。
// "OpenAI API 鉴权" 和 "OpenAI API 认证" 相似度高（共现 openai + api）
// "OpenAI API 鉴权" 和 "水果 种植 技术" 相似度 = 0（无共现词）
func CosineSimilarity(a, b SparseVector) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for term, weightA := range a {
		normA += weightA * weightA // 累加 |a|²
		if weightB, ok := b[term]; ok {
			dotProduct += weightA * weightB // a·b：只有共同出现的词才贡献
		}
	}
	for _, weightB := range b {
		normB += weightB * weightB // 累加 |b|²
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// DotProduct 计算两个稀疏向量的点积（内积）。
// 点积 = 共现词的权重乘积之和，值越大两个向量在共有维度上越"强"。
// 与余弦相似度的区别：点积不受归一化约束，长文档天然得分更高。
func DotProduct(a, b SparseVector) float64 {
	var result float64
	for term, weightA := range a {
		if weightB, ok := b[term]; ok {
			result += weightA * weightB
		}
	}
	return result
}

// ToSparseVector 将索引中已有的文档转为稀疏向量。
//
// 权重 = 归一化 TF × IDF（即 TF-IDF）。
//
//	TF/|d|  = 该词在本文档中的占比（消除文档长度差异）
//	IDF     = 该词的整体稀有度（"的"常见→IDF低，"BM25"罕见→IDF高）
//
// 常见用途：将 BM25 索引中的文档导出为稀疏向量，用于与稠密向量做混合检索。
func (s *BM25Scorer) ToSparseVector(docID int) SparseVector {
	if docID < 0 || docID >= s.docCount {
		return nil
	}

	vec := make(SparseVector)
	docLen := float64(s.docLengths[docID])
	if docLen == 0 {
		return vec
	}

	for term, posting := range s.invertedIdx {
		tf, ok := posting[docID]
		if !ok {
			continue
		}
		df := float64(len(posting))
		idf := math.Log((float64(s.docCount)-df+0.5)/(df+0.5) + 1)
		// TF/|d|：占比而非绝对值，避免长文档天然权重偏高
		// × IDF：稀有词获得更高权重
		vec[term] = tf / docLen * idf
	}
	return vec
}

// QueryToSparseVector 将查询文本转为稀疏向量。
//
// 权重 = 查询词频 × IDF。
// 与文档向量不同的是，查询通常很短（几个词），不需要归一化文档长度，
// 所以直接用词频 × IDF 作为权重。
//
// 常见用途：将用户查询转为稀疏向量，与文档的稀疏向量做 CosineSimilarity，
// 实现比 BM25 更灵活的相似度匹配（可以用余弦/点积等多种距离度量）。
func (s *BM25Scorer) QueryToSparseVector(query string) SparseVector {
	tokens := s.tokenizer.CutForQuery(query)
	if len(tokens) == 0 {
		return nil
	}

	vec := make(SparseVector)
	// 统计查询中每个词的词频
	seen := make(map[string]int)
	for _, t := range tokens {
		seen[t]++
	}

	for term, qtf := range seen {
		// 只保留在索引中出现过的词（未登录词没有 IDF 可算）
		posting, ok := s.invertedIdx[term]
		if !ok {
			continue
		}
		df := float64(len(posting))
		idf := math.Log((float64(s.docCount)-df+0.5)/(df+0.5) + 1)
		// 查询词频 × IDF：查询中重复的词获得更高权重
		vec[term] = float64(qtf) * idf
	}
	return vec
}
