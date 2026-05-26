package chunker

type ChunkStrategy string

const (
	Title    ChunkStrategy = "title"
	Semantic ChunkStrategy = "semantic"
)

type Chunk struct {
	Index    int
	Content  string
	TokenEst int
	StartPos int
	EndPos   int
	Metadata map[string]string
}

// ChunkOptions 分块参数
type ChunkOptions struct {
	ChunkSize    int
	ChunkOverlap int
}

// Chunker 分块器接口
type Chunker interface {
	Chunk(content string, opts ChunkOptions) ([]Chunk, error)
}
