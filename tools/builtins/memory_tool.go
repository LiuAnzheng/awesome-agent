package builtins

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/memory"
	"github.com/LiuAnzheng/memoria/memory/store"
	"github.com/LiuAnzheng/memoria/memory/store/impl"
	"github.com/LiuAnzheng/memoria/memory/types"
	"github.com/LiuAnzheng/memoria/tools"
)

var memoryToolDescription = `═══ CRITICAL — READ THIS FIRST ═══

You have NO persistent memory. Every turn starts BLANK. This tool is your ONLY way to persist knowledge across turns. Without it, everything you learn is LOST FOREVER.

═══ MANDATORY WORKFLOW (EVERY turn, no exceptions) ═══

  1. search(query="...") BEFORE answering —— recall what you already know
  2. add(content="...", importance=...) AFTER answering —— save what you learned

  Skipping add  = permanent amnesia.
  Skipping search = answering blind.

═══ WHAT MUST BE REMEMBERED ═══

You MUST record the following categories of information. Nothing is too small — the system will compress and organize it automatically.

  ┌─────────────────────┬──────────────────────────────────────────────────┐
  │ Category            │ Examples (add EVERY time you encounter these)    │
  ├─────────────────────┼──────────────────────────────────────────────────┤
  │ User Identity       │ name, role, team, department, seniority, contact │
  │ User Preferences    │ "prefers short answers", "uses Python", "hates   │
  │                     │  verbose error messages", timezone, language      │
  │ Key Decisions       │ "decided to use PostgreSQL over MySQL",          │
  │                     │  "chose monorepo strategy", "picked port 8080"    │
  │ Facts & Findings    │ API timeout values, config defaults, known bugs,  │
  │                     │  system limits, dependency versions               │
  │ Task Progress       │ what was done, what remains, what was attempted   │
  │                     │  and failed, current milestone                    │
  │ Action Results      │ tool outputs, error messages, search findings,    │
  │                     │  confirmed hypotheses                             │
  │ Reasoning Traces    │ WHY you made a choice, what alternatives you      │
  │                     │  considered, what assumptions you made            │
  │ User Feedback       │ corrections, "that's wrong", "good answer",       │
  │                     │  style complaints, accuracy judgments             │
  │ Conversation Context│ the user's original goal, topic shifts,           │
  │                     │  unresolved questions                             │
  └─────────────────────┴──────────────────────────────────────────────────┘

═══ IMPORTANCE GUIDE ═══

  Choose importance based on how critical the information is for FUTURE turns:

  0.9 - 1.0  ║ CRITICAL ║ User identity, decisions that change everything,
             ║          ║ irreversible actions, core preferences.
  0.7 - 0.8  ║ HIGH     ║ Key facts, findings, task progress, error messages,
             ║          ║ user corrections and feedback.
  0.5 - 0.6  ║ MEDIUM   ║ General observations, context details, intermediate
             ║          ║ reasoning steps, exploration results.
  0.3 - 0.4  ║ LOW      ║ Minor details, failed attempts that proved
             ║          ║ uninformative, background context.
  0.1 - 0.2  ║ TRIVIAL  ║ Barely relevant — use rarely.

  ⚠ If unsure, use 0.5. Err on the HIGH side for anything that might matter later.

═══ search GUIDANCE ═══

  - Searches across ALL memory types (working + episodic + semantic).
  - Use specific keywords: "user role Python" not "what does the user do".
  - Start with 2-3 core keywords; broaden if no results.
  - Always search even if you "think" you know — you might be wrong.
  - If search returns nothing, note it and move on — don't retry with minor variations.

═══ add GUIDANCE ═══

  - Write content as self-contained facts, NOT conversational replies.
    ✅ GOOD: "User Li Wei is a senior engineer in the Tech R&D Center, team lead for the API Gateway project."
    ❌ BAD:  "The user told me that he works as a senior engineer."
  - One piece of information per add() call for precise retrieval.
  - Include concrete values: version numbers, file paths, error codes, config keys.
  - After a multi-step task, add a summary with the full outcome chain.
  - If you learned 5 things, call add() 5 times — don't cram them into one call.

═══ FEW-SHOT EXAMPLES ═══

  User: "我是技术研发中心的李伟，负责API网关项目"
  → add(content="User Li Wei (李伟) works in Tech R&D Center, leads API Gateway project", importance=0.95)
  → search(query="李伟 API网关")

  User: "我不喜欢太啰嗦的回答，代码示例用Python"
  → add(content="User prefers concise answers, code examples in Python", importance=0.85)

  User: "把那个超时的bug修一下，之前排查到是连接池满了"
  → search(query="timeout bug connection pool fix")
  → add(content="Bug: timeout caused by connection pool exhaustion, currently investigating fix", importance=0.8)

  User: "上次你说的Redis配置要改成cluster模式"
  → search(query="Redis config cluster mode")
  → add(content="Decision: Redis should be configured as cluster mode (per user instruction)", importance=0.9)

═══ HOW MEMORY WORKS (so you understand the lifecycle) ═══

  Your add() calls enter Working Memory (in-memory, BM25 keyword search).
  When Working Memory fills up (~90%), the system auto-compresses the oldest
  items into Episodic Memory (SQLite + vector search) — a concise narrative
  summary is generated. Episodic memories can later consolidate into
  Semantic Memory (Neo4j knowledge graph) for cross-session reasoning.

  → High-importance items are more likely to survive compression.
  → Self-contained, fact-dense content compresses better than chatty notes.
  → What you write NOW determines what you can recall LATER.`

type MemoryTool struct {
	types            []types.MemoryType
	description      string
	vectorStore      store.VectorStore
	structuredStore  store.StructuredStore
	embeddingService store.EmbeddingService
	graphStore       store.GraphStore

	config core.MemoryConfig
	llm    core.LLMInterface

	managers         map[string]*memory.Manager
	mu               sync.RWMutex
	defaultSessionID string
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
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	config core.MemoryConfig,
	llm core.LLMInterface,

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

	if slices.Contains(mt.types, types.Working) && slices.Contains(mt.types, types.Episodic) && llm == nil {
		return nil, fmt.Errorf("memory compress need to llm client")
	}
	mt.llm = llm

	return mt, nil
}

func (m *MemoryTool) initDefaults() error {
	cfg := m.config

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
