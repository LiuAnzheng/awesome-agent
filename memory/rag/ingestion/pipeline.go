package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"awesome-agent/core"
	"awesome-agent/memory/rag/ingestion/chunker"
	"awesome-agent/memory/rag/ingestion/parser"
	"awesome-agent/memory/store"
	"awesome-agent/memory/store/impl"

	"github.com/google/uuid"
)

// http4xxCodes HTTP 4xx 客户端错误状态码（排除 429 Too Many Requests，该错误可重试）
var http4xxCodes = map[string]bool{
	"400": true, "401": true, "402": true, "403": true, "404": true,
	"405": true, "406": true, "407": true, "408": true, "409": true,
	"410": true, "411": true, "412": true, "413": true, "414": true,
	"415": true, "416": true, "417": true, "418": true, "421": true,
	"422": true, "423": true, "424": true, "425": true, "426": true,
	"428": true, "431": true, "451": true,
}

// isNonRetryableHTTPError 检查错误是否为不可重试的 HTTP 4xx 错误
func isNonRetryableHTTPError(err error) bool {
	msg := err.Error()
	for code := range http4xxCodes {
		if strings.Contains(msg, "status "+code) {
			return true
		}
	}
	return false
}

type IngestOptions struct {
	ChunkSize    int
	ChunkOverlap int
}

type IngestResult struct {
	DocID      string
	DocName    string
	Format     string
	ChunkCount int
	Duplicate  bool
}

type Pipeline struct {
	config      core.AppConfig
	parserReg   *parser.Registry
	chunker     chunker.Chunker
	EmbedSvc    store.EmbeddingService
	VectorStore store.VectorStore
	DocStore    store.StructuredStore
}

func NewPipeline(
	config core.AppConfig,
	parserReg *parser.Registry,
	iChunker chunker.Chunker,
	embedSvc store.EmbeddingService,
	vectorStore store.VectorStore,
	docStore store.StructuredStore,
) (*Pipeline, error) {
	p := &Pipeline{
		parserReg:   parserReg,
		chunker:     iChunker,
		EmbedSvc:    embedSvc,
		VectorStore: vectorStore,
		DocStore:    docStore,
		config:      config,
	}
	if p.parserReg == nil {
		p.parserReg = parser.NewParserRegistry()
	}
	if p.chunker == nil {
		p.chunker = chunker.NewRecursiveChunker()
	}
	if p.EmbedSvc == nil {
		p.EmbedSvc = impl.NewOpenAIEmbedding(config.Memory.Embedding.Options)
	}
	if p.VectorStore == nil {
		collection := config.RAGConfig.Collection
		if collection == "" {
			collection = "rag_chunks"
		}
		p.VectorStore = impl.NewQdrantStore(config.Memory.VectorStore.Options)
		err := p.VectorStore.Init(context.Background(), collection, p.EmbedSvc.Dimension())
		if err != nil {
			return nil, err
		}
	}
	if p.DocStore == nil {
		p.DocStore = impl.NewSQLiteStore(config.Memory.Structured.Options)
		err := p.DocStore.Init(context.Background())
		if err != nil {
			return nil, err
		}
	}
	err := p.init(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize pipeline: %w", err)
	}
	return p, nil
}

func (p *Pipeline) Ingest(ctx context.Context, reader io.Reader, filename string, opts IngestOptions) (*IngestResult, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if ext == "" {
		ext = "txt"
	}
	suiteParser, format, err := p.parserReg.Find(ext)
	if err != nil {
		return nil, fmt.Errorf("unsupported format %q: %w", ext, err)
	}

	var maxDocSize int64 = 50 * 1024 * 1024
	if p.config.RAGConfig.MaxDocSize > 0 {
		maxDocSize = p.config.RAGConfig.MaxDocSize
	}
	doc, err := suiteParser.Parse(reader, parser.ParseOptions{
		MaxSize:  maxDocSize,
		Encoding: "utf-8",
		Filename: filepath.Base(filename),
	})
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	doc.ID = uuid.New().String()
	doc.Name = filepath.Base(filename)

	// SHA256 去重
	sha := sha256Sum(doc.Content)
	if existingID := p.findDuplicate(ctx, sha); existingID != "" {
		return &IngestResult{
			DocID:     existingID,
			DocName:   doc.Name,
			Format:    format,
			Duplicate: true,
		}, nil
	}

	chunks, err := p.chunker.Chunk(doc.Content, chunker.ChunkOptions{
		ChunkSize:    opts.ChunkSize,
		ChunkOverlap: opts.ChunkOverlap,
	})
	if err != nil {
		return nil, fmt.Errorf("chunk: %w", err)
	}

	vectors, err := p.embedChunks(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("embed: got %d vectors for %d chunks", len(vectors), len(chunks))
	}

	result, err := p.store(ctx, doc, chunks, vectors, sha)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	return result, nil
}

func (p *Pipeline) init(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS rag_documents (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		format      TEXT NOT NULL,
		size        INTEGER,
		chunk_count INTEGER DEFAULT 0,
		status      TEXT DEFAULT 'ready',
		sha256      TEXT,
		created_at  TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_rag_docs_status ON rag_documents(status);
	CREATE INDEX IF NOT EXISTS idx_rag_docs_sha256 ON rag_documents(sha256);

	CREATE TABLE IF NOT EXISTS rag_chunks (
		id          TEXT PRIMARY KEY,
		doc_id      TEXT NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
		chunk_index INTEGER NOT NULL,
		content     TEXT NOT NULL,
		token_est   INTEGER,
		start_pos   INTEGER,
		end_pos     INTEGER,
		metadata    TEXT DEFAULT '{}',
		created_at  TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_rag_chunks_doc ON rag_chunks(doc_id);`

	return p.DocStore.Exec(ctx, schema)
}

func sha256Sum(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func (p *Pipeline) findDuplicate(ctx context.Context, sha string) string {
	records, err := p.DocStore.Query(ctx, store.Query{
		Table: "rag_documents",
		Conditions: []store.Condition{
			{Field: "sha256", Operator: "=", Value: sha},
		},
		Limit: 1,
	})
	if err != nil || len(records) == 0 {
		return ""
	}
	if id, ok := records[0]["id"].(string); ok {
		return id
	}
	return ""
}

func (p *Pipeline) embedChunks(ctx context.Context, chunks []chunker.Chunk) ([][]float64, error) {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	batchSize := 32
	if v, ok := p.config.Memory.Embedding.Options["batch_size"]; ok {
		switch n := v.(type) {
		case int:
			batchSize = n
		case float64:
			batchSize = int(n)
		}
	}
	allVectors := make([][]float64, 0, len(chunks))

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		vectors, err := embedWithRetry(ctx, p.EmbedSvc, batch, 3)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", i, end, err)
		}
		allVectors = append(allVectors, vectors...)
	}
	return allVectors, nil
}

func embedWithRetry(ctx context.Context, svc store.EmbeddingService, texts []string, maxRetries int) ([][]float64, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		vectors, err := svc.EmbedBatch(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		lastErr = err

		// 4xx 客户端错误不可恢复，不重试
		if isNonRetryableHTTPError(err) {
			return nil, err
		}

		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
	}
	return nil, fmt.Errorf("embed failed after %d retries: %w", maxRetries, lastErr)
}

func (p *Pipeline) store(ctx context.Context, doc *parser.Document, chunks []chunker.Chunk, vectors [][]float64, sha string) (*IngestResult, error) {
	now := core.Now().Format(time.RFC3339)

	// 写 document 记录
	if err := p.DocStore.Save(ctx, "rag_documents", store.Record{
		"id":          doc.ID,
		"name":        doc.Name,
		"format":      doc.Format,
		"size":        doc.Size,
		"chunk_count": len(chunks),
		"status":      "processing",
		"sha256":      sha,
		"created_at":  now,
	}); err != nil {
		return nil, fmt.Errorf("save document: %w", err)
	}

	// chunk 写入失败时级联清理 document
	var chunksCommitted bool
	defer func() {
		if !chunksCommitted {
			_ = p.DocStore.Delete(ctx, "rag_documents", doc.ID)
		}
	}()

	collection := p.config.RAGConfig.Collection
	if collection == "" {
		collection = "rag_chunks"
	}

	points := make([]store.VectorPoint, len(chunks))
	for i, c := range chunks {
		chunkID := uuid.New().String()
		meta := c.Metadata
		if meta == nil {
			meta = map[string]string{}
		}

		if err := p.DocStore.Save(ctx, "rag_chunks", store.Record{
			"id":          chunkID,
			"doc_id":      doc.ID,
			"chunk_index": c.Index,
			"content":     c.Content,
			"token_est":   c.TokenEst,
			"start_pos":   c.StartPos,
			"end_pos":     c.EndPos,
			"metadata":    toJSON(meta),
			"created_at":  now,
		}); err != nil {
			return nil, fmt.Errorf("save chunk %d: %w", i, err)
		}

		points[i] = store.VectorPoint{
			ID:     chunkID,
			Vector: vectors[i],
			Payload: map[string]interface{}{
				"doc_id":      doc.ID,
				"doc_name":    doc.Name,
				"chunk_index": c.Index,
			},
		}
	}

	// chunk 全部写成功 → Qdrant 失败时保留数据，不触发 cleanup
	chunksCommitted = true

	// 批量写 Qdrant
	if err := p.VectorStore.BatchUpsert(ctx, collection, points); err != nil {
		if markErr := p.DocStore.Exec(ctx,
			"UPDATE rag_documents SET status = 'vector_pending' WHERE id = ?", doc.ID); markErr != nil {
			return nil, fmt.Errorf("vector store: %w (mark vector_pending: %v)", err, markErr)
		}
		return nil, fmt.Errorf("vector store: %w", err)
	}

	// 标记完成
	if err := p.DocStore.Exec(ctx,
		"UPDATE rag_documents SET status = 'ready' WHERE id = ?", doc.ID); err != nil {
		return nil, fmt.Errorf("mark ready: %w", err)
	}

	return &IngestResult{
		DocID:      doc.ID,
		DocName:    doc.Name,
		Format:     doc.Format,
		ChunkCount: len(chunks),
	}, nil
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
