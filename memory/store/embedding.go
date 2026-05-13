package store

import "context"

// EmbeddingService 文本向量化服务
type EmbeddingService interface {
	// Embed 单条文本向量化
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch 批量向量化
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

	// Dimension 返回向量维度
	Dimension() uint64
}
