package builtins

import (
	"awesome-agent/core"
	"awesome-agent/memory"
	"awesome-agent/memory/store"
	"awesome-agent/memory/store/impl"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
)

var memoryToolDescription = `CRITICAL: You have NO persistent memory. Every turn starts from zero. Without this tool, all past context is LOST.

ALWAYS use this tool every turn:
1. search(query="keywords") BEFORE answering — recall what you already know
2. add(content="what you learned", importance=0.5~0.9) AFTER answering — save new knowledge

Skipping add = forgetting permanently. Skipping search = ignoring existing knowledge.

Actions:
- add: content* (what to remember), importance (0.0~1.0, default 0.5, 0.8+ for critical facts)
- search: query* (search text), limit (max results, default 10)`

type MemoryTool struct {
	types            []types.MemoryType
	description      string
	vectorStore      store.VectorStore
	structuredStore  store.StructuredStore
	embeddingService store.EmbeddingService
	graphStore       store.GraphStore
	config           core.AppConfig

	managers         map[string]*memory.Manager
	mu               sync.RWMutex
	defaultSessionID string
	llm              core.LLMInterface
}

func (m *MemoryTool) Name() string {
	return "memory_tool"
}

func (m *MemoryTool) Description() string {
	return m.description
}

func (m *MemoryTool) Parameters() []tools.ToolParameter {
	return []tools.ToolParameter{
		{Name: "action", Type: tools.ParamString, Required: true,
			Description: "One of: add (save a memory), search (recall past memories)"},
		{Name: "content", Type: tools.ParamString, Required: false,
			Description: "[add] What to remember"},
		{Name: "importance", Type: tools.ParamNumber, Required: false, Default: 0.5,
			Description: "[add] Importance 0.0~1.0. 0.8+ = critical, 0.5 = normal, 0.3 = trivial"},
		{Name: "query", Type: tools.ParamString, Required: false,
			Description: "[search] Natural language search text"},
		{Name: "limit", Type: tools.ParamInteger, Required: false, Default: 10,
			Description: "[search] Max results"},
	}
}

func (m *MemoryTool) Run(parameters map[string]interface{}) (string, error) {
	mgr, err := m.resolveManager(parameters)
	if err != nil {
		return "", err
	}
	action, _ := parameters["action"].(string)
	switch action {
	case "add":
		return m.runAdd(mgr, parameters)
	case "search":
		items, err := m.RunSearch(mgr, parameters)
		if err != nil {
			return "", err
		}
		return jsonResult("searched", map[string]interface{}{
			"count":   len(items),
			"results": items,
		})
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// GetManager 返回指定 session 的 Manager；sessionID 为空时回退到 defaultSessionID。
func (m *MemoryTool) GetManager(sessionID string) (*memory.Manager, error) {
	if sessionID == "" {
		sessionID = m.getDefaultSessionID()
	}
	if sessionID == "" {
		return nil, fmt.Errorf("no session: sessionID is empty and no default session set")
	}
	m.mu.RLock()
	mgr, ok := m.managers[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	return mgr, nil
}

// resolveManager 从 _session_id 解析 Manager；未注入时回退到 defaultSessionID。
func (m *MemoryTool) resolveManager(params map[string]interface{}) (*memory.Manager, error) {
	sessionID, _ := params["_session_id"].(string)
	return m.GetManager(sessionID)
}

func (m *MemoryTool) HasSessionID(sessionID string) bool {
	_, ok := m.managers[sessionID]
	return ok
}

func (m *MemoryTool) getDefaultSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultSessionID
}

func (m *MemoryTool) runAdd(mgr *memory.Manager, p map[string]interface{}) (string, error) {
	content, _ := p["content"].(string)
	if content == "" {
		return "", fmt.Errorf("content is required for add")
	}
	item := types.MemoryItem{
		Content:    content,
		Importance: toFloat64(p["importance"]),
	}
	id, err := mgr.Add(item)
	if err != nil {
		return "", err
	}
	return jsonResult("added", map[string]interface{}{
		"id": id,
	})
}

func (m *MemoryTool) RunSearch(mgr *memory.Manager, p map[string]interface{}) ([]types.MemoryItem, error) {
	query, _ := p["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required for search")
	}
	limit := toInt64(p["limit"])

	opts := types.SearchOptions{
		Limit:         limit,
		MinScore:      0.1,
		MinImportance: 0.1,
		Filter: map[string]string{
			"session_id": mgr.SessionId,
		},
	}

	all, err := mgr.Search(query, m.types, opts)
	if err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Importance > all[j].Importance
	})
	if int64(len(all)) > limit {
		all = all[:limit]
	}

	return all, nil
}

func (m *MemoryTool) AddSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.managers[sessionID]; ok {
		return fmt.Errorf("session ID %q already exists", sessionID)
	}
	mm, err := memory.NewManager(m.config,
		sessionID,
		slices.Contains(m.types, types.Working),
		slices.Contains(m.types, types.Episodic),
		slices.Contains(m.types, types.Semantic),
		slices.Contains(m.types, types.Perceptual),
		m.vectorStore,
		m.structuredStore,
		m.embeddingService,
		m.graphStore,
		m.llm)
	if err != nil {
		return err
	}
	m.managers[sessionID] = mm
	if m.defaultSessionID == "" {
		m.defaultSessionID = sessionID
	}
	return nil
}

func (m *MemoryTool) RemoveSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.managers[sessionID]; !ok {
		return fmt.Errorf("session ID %q not found", sessionID)
	}
	delete(m.managers, sessionID)
	if m.defaultSessionID == sessionID {
		m.defaultSessionID = ""
	}
	return nil
}

func (m *MemoryTool) UseSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.managers[sessionID]; !ok {
		return fmt.Errorf("session ID %q not found", sessionID)
	}
	m.defaultSessionID = sessionID
	return nil
}

func NewMemoryTool(
	config core.AppConfig,
	memoryTypes []types.MemoryType,
	vectorStore store.VectorStore,
	structuredStore store.StructuredStore,
	embeddingService store.EmbeddingService,
	graphStore store.GraphStore,
) (tools.Tool, error) {
	mt := &MemoryTool{
		types:            memoryTypes,
		config:           config,
		vectorStore:      vectorStore,
		structuredStore:  structuredStore,
		embeddingService: embeddingService,
		graphStore:       graphStore,

		managers: make(map[string]*memory.Manager),
	}

	if len(memoryTypes) == 0 {
		mt.types = []types.MemoryType{types.Working, types.Episodic, types.Semantic}
	}
	mt.description = memoryToolDescription

	if err := mt.initDefaults(); err != nil {
		return nil, err
	}

	if slices.Contains(mt.types, types.Working) && slices.Contains(mt.types, types.Episodic) {
		llm, err := core.NewLLM(core.LLMConfig{
			ModelID:         config.LLMConfig.ModelID,
			BaseURL:         config.LLMConfig.BaseURL,
			MaxTokens:       config.LLMConfig.MaxTokens,
			Temperature:     0.3, // 记忆整合用低温度
			TopP:            config.LLMConfig.TopP,
			OpenAIExtraInfo: config.LLMConfig.OpenAIExtraInfo,
			Provider:        config.LLMConfig.Provider,
			APIKey:          config.LLMConfig.APIKey,
		})
		if err != nil {
			return nil, err
		}
		mt.llm = llm
	}

	return mt, nil
}

func (m *MemoryTool) initDefaults() error {
	cfg := m.config.Memory

	if m.structuredStore == nil && slices.Contains(m.types, types.Episodic) {
		opts := cfg.Structured.Options
		switch cfg.Structured.Driver {
		case "sqlite":
			m.structuredStore = impl.NewSQLiteStore(opts)
		default:
			return fmt.Errorf("unsupported structure driver: %s", cfg.Structured.Driver)
		}
		if err := m.structuredStore.Init(context.Background()); err != nil {
			return err
		}
	}

	if m.embeddingService == nil && (slices.Contains(m.types, types.Episodic) || slices.Contains(m.types, types.Semantic)) {
		opts := cfg.Embedding.Options
		switch cfg.Embedding.Driver {
		case "openai":
			m.embeddingService = impl.NewOpenAIEmbedding(opts)
		default:
			return fmt.Errorf("unsupported embedding driver: %s", cfg.Embedding.Driver)
		}
	}

	if m.vectorStore == nil && (slices.Contains(m.types, types.Episodic) || slices.Contains(m.types, types.Semantic)) {
		opts := cfg.VectorStore.Options
		switch cfg.VectorStore.Driver {
		case "qdrant":
			m.vectorStore = impl.NewQdrantStore(opts)
		default:
			return fmt.Errorf("unsupported vector_store driver: %s", cfg.VectorStore.Driver)
		}
		if err := m.vectorStore.Init(context.Background(), "episodes", m.embeddingService.Dimension()); err != nil {
			return fmt.Errorf("init vector_store episodic: %w", err)
		}
		if err := m.vectorStore.Init(context.Background(), "semantic", m.embeddingService.Dimension()); err != nil {
			return fmt.Errorf("init vector_store semantic: %w", err)
		}
	}

	if m.graphStore == nil && slices.Contains(m.types, types.Semantic) {
		opts := cfg.Graph.Options
		switch cfg.Graph.Driver {
		case "neo4j":
			m.graphStore = impl.NewNeo4jStore(opts)
		default:
			return fmt.Errorf("unsupported graph driver: %s", cfg.Graph.Driver)
		}
		if err := m.graphStore.Init(context.Background()); err != nil {
			return fmt.Errorf("init graph store neo4j: %w", err)
		}
	}

	return nil
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	default:
		return 0
	}
}
