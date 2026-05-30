package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"memoria/memory/rag/advanced_features"
	"memoria/memory/rag/ingestion/chunker"
	"os"
	"strings"
	"time"

	"memoria/core"
	"memoria/memory/rag/ingestion"
	"memoria/memory/store"
	"memoria/tools"
)

var ragDescription = `Search a pre-loaded document knowledge base. Documents are already ingested — use this tool to find and cite relevant information.

=== HOW SEARCH WORKS ===

  Search automatically uses multi-query expansion (MQE) and hypothetical
  document embedding (HyDE) behind the scenes. This means:
  - A single query will be expanded into 3 semantic variants + 1 hypothetical
    answer document, then searched in parallel, then fused via RRF.
  - Vague queries work better than you'd expect — just write naturally.
  - Empty results after this multi-pass pipeline means the info is genuinely
    not in the knowledge base — don't waste turns retrying.

=== Actions & Triggers ===

[search] — Vector search over document chunks (MQE + HyDE enhanced).
  Triggers: user asks about ANY topic that might be covered by documents.
    "what is X", "how does Y work", "find info about Z", "look up W"
  Params: query* (natural language, be specific but not verbose),
          top_k? (chunks to return, default 5, use 10-20 for broad topics)
  Returns: list of {chunk_id, doc_id, doc_name, content, score, chunk_index}
  On empty results: try ONE alternative phrasing (synonyms, broader terms,
    translate Chinese<->English). If still empty, tell the user.

[list] — List documents in knowledge base.
  Triggers: "what documents are available", "show me the knowledge base",
    "list all docs", "what's been ingested"
  Params: limit? (default 20)
  Returns: list of {doc_id, doc_name, format, chunk_count, status, created_at}

[delete] — Remove a document and all its chunks.
  Triggers: "delete/remove that document", "get rid of X doc"
  Params: doc_id* (get from [list] or [status] first)
  !! You MUST confirm the target document via [list] or [status] before deleting.

[status] — Get details for a specific document.
  Triggers: "show me details of doc X", "what's in document Y"
  Params: doc_id* (get from [list] first if unknown)
  Returns: {doc_id, doc_name, format, status, chunk_count, created_at}

* = required, ? = optional

=== Citation Format ===
After [search], cite every claim using the returned doc_name and chunk_index, like this:
  "The default timeout is 30 seconds [1]."
  At the end of your answer, list all references:
  ---
  References:
  [1] demo_OpenAIAPI规范.md, chunk 3 (score: 0.92)
  [2] 系统设计文档.md, chunk 7 (score: 0.85)

Always include: doc_name, chunk_index, and score for each reference you cite.

=== Workflow Rules ===
1. Factual question → [search] FIRST. Never answer from memory alone.
2. Cite sources: every fact from search results MUST have a [N] tag linking to the reference list.
3. Empty search results → try ONE rephrase (synonym, broader term, language switch). If still empty, tell the user clearly. Do NOT guess content.
4. Before [delete] → [list] to find doc_id → [status] to confirm.
5. When user asks what documents exist → [list]. For one doc's details → [status].
6. High-scoring chunks (>=0.85) are near-exact matches — prefer them.
7. Low-scoring chunks (<=0.60) are tangentially related — use with caution.

=== Search Retry Strategies ===
If initial search returns nothing or irrelevant results, try ONE of:
  - Use synonyms: "auth" -> "authentication", "config" -> "configuration"
  - Go broader: "Redis cluster timeout config" -> "Redis timeout"
  - Switch language: "认证" -> "authentication", "API timeout" -> "API超时"
  - Use key nouns only: "What is the default timeout?" -> "timeout default"
Do NOT try more than one retry — if both fail, the info isn't there.

=== Few-Shot ===
User: "What's the API timeout setting?"
  → search(query="API timeout configuration")
  → "The API timeout is set to 30 seconds [1]. Tokens expire after 3600 seconds [1]."
  → "References:\n[1] demo_OpenAIAPI规范.md, chunk 3 (score: 0.92)"

User: "What docs do we have?" → list()

User: "Delete the outdated PRD"
  → list() → status(doc_id="...") to confirm → delete(doc_id="...")

User: "数据库连接池怎么配？" (no results for first search)
  → search(query="database connection pool") (retry in English)
  → If still empty: "知识库中未找到数据库连接池配置的相关文档。"`

type RAGTool struct {
	pipeline *ingestion.Pipeline
	config   core.AppConfig

	llm        core.LLMInterface
	enableMQE  bool
	enableHyDE bool
}

type SearchItem struct {
	ChunkID    string     `json:"chunk_id"`
	DocID      string     `json:"doc_id"`
	DocName    string     `json:"doc_name"`
	Content    string     `json:"content"`
	Score      float64    `json:"score"`
	ChunkIndex int        `json:"chunk_index"`
	CreatedAt  *time.Time `json:"created_at"`
}

func NewRAGTool(
	embedSvc store.EmbeddingService,
	vectorStore store.VectorStore,
	docStore store.StructuredStore,
	config core.AppConfig,
	chunkStrategy chunker.ChunkStrategy,
	enableMQE bool,
	enableHyDE bool,
) (tools.Tool, error) {
	rt := &RAGTool{
		config:     config,
		enableMQE:  enableMQE,
		enableHyDE: enableHyDE,
	}
	pipeline, err := ingestion.NewPipeline(config, nil, nil, embedSvc, vectorStore, docStore, chunkStrategy)
	if err != nil {
		return nil, err
	}
	rt.pipeline = pipeline

	if enableMQE || enableHyDE {
		llm, err := core.NewLLM(core.LLMConfig{
			ModelID:         config.LLMConfig.ModelID,
			BaseURL:         config.LLMConfig.BaseURL,
			MaxTokens:       config.LLMConfig.MaxTokens,
			Temperature:     0.3,
			TopP:            config.LLMConfig.TopP,
			OpenAIExtraInfo: config.LLMConfig.OpenAIExtraInfo,
			Provider:        config.LLMConfig.Provider,
			APIKey:          config.LLMConfig.APIKey,
		})
		if err != nil {
			return nil, err
		}
		rt.llm = llm
	}

	return rt, nil
}

func (r *RAGTool) Name() string        { return "rag_tool" }
func (r *RAGTool) Description() string { return ragDescription }

func (r *RAGTool) Parameters() []tools.ToolParameter {
	return []tools.ToolParameter{
		{Name: "action", Type: tools.ParamString, Required: true,
			Description: "One of: search (find relevant chunks), list (browse all docs), delete (remove a doc), status (inspect a doc)"},
		{Name: "query", Type: tools.ParamString, Required: false,
			Description: "[search] Natural language search query, e.g. 'API timeout config'"},
		{Name: "doc_id", Type: tools.ParamString, Required: false,
			Description: "[delete, status] Target document ID. Must be obtained from a prior [list] call."},
		{Name: "top_k", Type: tools.ParamInteger, Required: false,
			Description: "[search] Number of chunks to return, default 5"},
		{Name: "limit", Type: tools.ParamInteger, Required: false,
			Description: "[list] Max documents to list, default 20"},
	}
}

func (r *RAGTool) Run(params map[string]interface{}) (string, error) {
	action, _ := params["action"].(string)
	switch action {
	case "search":
		items, err := r.RunSearch(params)
		if err != nil {
			return "", err
		}
		return jsonResult("searched", map[string]interface{}{
			"count":   len(items),
			"results": items,
		})
	case "list":
		return r.runList(params)
	case "delete":
		return r.runDelete(params)
	case "status":
		return r.runStatus(params)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *RAGTool) Ingest(ctx context.Context, source, name string) error {
	reader, filename, closeFn, err := openSource(source)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	if closeFn != nil {
		defer func(fn func() error) {
			e := closeFn()
			if e != nil {
				slog.Error("close source", "err", e)
			}
		}(closeFn)
	}
	if name != "" {
		filename = name
	}

	result, err := r.pipeline.Ingest(ctx, reader, filename, ingestion.IngestOptions{
		ChunkSize:    1024,
		ChunkOverlap: 50,
	})

	if err != nil {
		return fmt.Errorf("ingest error: %w", err)
	}
	slog.Debug("Ingest result", "result", result)
	if result.Duplicate {
		slog.Warn("Duplicate document", "source", source, "filename", filename)
	}
	return nil
}

func openSource(source string) (io.Reader, string, func() error, error) {
	info, err := os.Stat(source)
	if err == nil && !info.IsDir() {
		f, err := os.Open(source)
		if err != nil {
			return nil, "", nil, err
		}
		return f, source, f.Close, nil
	}
	return strings.NewReader(source), "paste.txt", nil, nil
}

func (r *RAGTool) RunSearch(params map[string]interface{}) ([]SearchItem, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required for search")
	}
	topK := getInt(params, "top_k", 5)

	collectionName := r.collection()
	results, err := advanced_features.Recall(context.Background(), r.llm, query, topK, collectionName,
		r.pipeline.EmbedSvc, r.pipeline.VectorStore, r.enableMQE, r.enableHyDE)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	if len(results) == 0 {
		return []SearchItem{}, nil
	}

	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}

	chunks, err := r.fetchChunks(context.Background(), ids)
	if err != nil {
		return nil, fmt.Errorf("fetch chunks: %w", err)
	}

	items := make([]SearchItem, 0, topK)
	skipped := 0
	for _, r := range results {
		ck, ok := chunks[r.ID]
		if !ok {
			skipped++
			slog.Warn("rag search: vector exists but chunk record missing",
				"chunk_id", r.ID, "score", r.Score)
			continue
		}
		items = append(items, SearchItem{
			ChunkID:    r.ID,
			DocID:      ck.DocID,
			DocName:    ck.DocName,
			Content:    ck.Content,
			Score:      r.Score,
			ChunkIndex: ck.Index,
			CreatedAt:  ck.CreatedAt,
		})
		if len(items) >= topK {
			break
		}
	}
	if skipped > 0 {
		slog.Warn("rag search: vector results point to missing chunk records",
			"skipped", skipped, "total_vector_results", len(results))
	}

	return items, nil
}

type chunkInfo struct {
	DocID     string
	DocName   string
	Content   string
	Index     int
	CreatedAt *time.Time
}

func (r *RAGTool) fetchChunks(ctx context.Context, ids []string) (map[string]chunkInfo, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	conds := []store.Condition{{Field: "id", Operator: "IN", Value: strSliceToInterface(ids)}}
	records, err := r.pipeline.DocStore.Query(ctx, store.Query{
		Table:      "rag_chunks",
		Conditions: conds,
	})
	if err != nil {
		return nil, err
	}

	result := make(map[string]chunkInfo, len(records))
	docIDs := make(map[string]bool)
	for _, rec := range records {
		id := getStr(rec, "id")
		docID := getStr(rec, "doc_id")
		result[id] = chunkInfo{
			DocID:     docID,
			Content:   getStr(rec, "content"),
			Index:     getIntFromRec(rec, "chunk_index"),
			CreatedAt: parseTimeStr(getStr(rec, "created_at")),
		}
		docIDs[docID] = true
	}

	idList := make([]string, 0, len(docIDs))
	for id := range docIDs {
		idList = append(idList, id)
	}
	if len(idList) > 0 {
		docRecords, err := r.pipeline.DocStore.Query(ctx, store.Query{
			Table:      "rag_documents",
			Conditions: []store.Condition{{Field: "id", Operator: "IN", Value: strSliceToInterface(idList)}},
		})
		if err != nil {
			slog.Warn("rag search: failed to enrich doc names", "error", err)
		} else {
			docNames := make(map[string]string, len(docRecords))
			for _, rec := range docRecords {
				docNames[getStr(rec, "id")] = getStr(rec, "name")
			}
			for id, ci := range result {
				if name, ok := docNames[ci.DocID]; ok {
					ci.DocName = name
					result[id] = ci
				}
			}
		}
	}
	return result, nil
}

func (r *RAGTool) runList(params map[string]interface{}) (string, error) {
	limit := getInt(params, "limit", 20)

	ctx := context.Background()
	records, err := r.pipeline.DocStore.Query(ctx, store.Query{
		Table:   "rag_documents",
		OrderBy: []store.OrderBy{{Field: "created_at", Dir: "DESC"}},
		Limit:   int64(limit),
	})
	if err != nil {
		return "", fmt.Errorf("list: %w", err)
	}

	type docInfo struct {
		DocID      string `json:"doc_id"`
		DocName    string `json:"doc_name"`
		Format     string `json:"format"`
		ChunkCount int    `json:"chunk_count"`
		Status     string `json:"status"`
		CreatedAt  string `json:"created_at"`
	}
	docs := make([]docInfo, 0, len(records))
	for _, rec := range records {
		docs = append(docs, docInfo{
			DocID:      getStr(rec, "id"),
			DocName:    getStr(rec, "name"),
			Format:     getStr(rec, "format"),
			ChunkCount: getIntFromRec(rec, "chunk_count"),
			Status:     getStr(rec, "status"),
			CreatedAt:  getStr(rec, "created_at"),
		})
	}

	return jsonResult("listed", map[string]interface{}{
		"count": len(docs),
		"docs":  docs,
	})
}

func (r *RAGTool) runDelete(params map[string]interface{}) (string, error) {
	docID, _ := params["doc_id"].(string)
	if docID == "" {
		return "", fmt.Errorf("doc_id is required for delete")
	}

	ctx := context.Background()

	records, err := r.pipeline.DocStore.Query(ctx, store.Query{
		Table: "rag_chunks",
		Conditions: []store.Condition{
			{Field: "doc_id", Operator: "=", Value: docID},
		},
	})
	if err != nil {
		return "", fmt.Errorf("query chunks: %w", err)
	}

	chunkIDs := make([]string, 0, len(records))
	for _, rec := range records {
		chunkIDs = append(chunkIDs, getStr(rec, "id"))
	}

	if err := r.pipeline.DocStore.Delete(ctx, "rag_documents", docID); err != nil {
		return "", fmt.Errorf("delete document: %w", err)
	}

	if len(chunkIDs) > 0 {
		_ = r.pipeline.VectorStore.Delete(ctx, r.collection(), chunkIDs)
	}

	return jsonResult("deleted", map[string]interface{}{
		"doc_id":         docID,
		"chunks_removed": len(chunkIDs),
	})
}

func (r *RAGTool) runStatus(params map[string]interface{}) (string, error) {
	docID, _ := params["doc_id"].(string)
	if docID == "" {
		return "", fmt.Errorf("doc_id is required for status")
	}

	ctx := context.Background()
	rec, err := r.pipeline.DocStore.Get(ctx, "rag_documents", docID)
	if err != nil {
		return "", fmt.Errorf("get document: %w", err)
	}
	if rec == nil {
		return "", fmt.Errorf("document not found: %s", docID)
	}

	return jsonResult("status", map[string]interface{}{
		"doc_id":      docID,
		"doc_name":    getStr(rec, "name"),
		"format":      getStr(rec, "format"),
		"status":      getStr(rec, "status"),
		"chunk_count": getIntFromRec(rec, "chunk_count"),
		"created_at":  getStr(rec, "created_at"),
	})
}

func (r *RAGTool) collection() string {
	c := r.config.RAGConfig.Collection
	if c == "" {
		return "rag_chunks"
	}
	return c
}

func getInt(params map[string]interface{}, key string, defaultVal int) int {
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return defaultVal
}

func getStr(rec store.Record, key string) string {
	if v, ok := rec[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getIntFromRec(rec store.Record, key string) int {
	if v, ok := rec[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		case string:
			var i int
			fmt.Sscanf(n, "%d", &i)
			return i
		}
	}
	return 0
}

func strSliceToInterface(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}

func jsonResult(action string, data map[string]interface{}) (string, error) {
	result := map[string]interface{}{"action": action}
	for k, v := range data {
		result[k] = v
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseTimeStr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
