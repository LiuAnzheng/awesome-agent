package builtins

import (
	"awesome-agent/core"
	"awesome-agent/memory"
	"awesome-agent/memory/store"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"time"
)

var logger = log.New(os.Stderr, "[builtins] ", log.LstdFlags|log.Lshortfile)

var memoryToolDescription = `Memory tool for storing, searching, managing memories across 3 types: episodic (events with time decay), semantic (knowledge with graph relations), working (short-term context).

== Actions & Parameters ==

[add] — Store new memory. params: memory_type*, content*, importance, summary, event_type, tags, relations.
  - memory_type*: "working" | "episodic" | "semantic"
  - content*: full text to store
  - importance: 0.0~1.0 (default 0.5). 0.8+ for key findings, 0.5 normal, 0.3- trivial
  - summary: short summary for embedding generation (default=content)
  - event_type: "observation" | "thought" | "action" | "result" (episodic only)
  - tags: comma-separated tags (semantic only, e.g. "Go,error_handling,concurrency")
  - relations: JSON array for semantic graph edges or episodic event chains (optional)

[search] — Semantic vector search. params: query*, memory_types, limit, min_score, min_importance, session_id, tags.
  - query*: search text, translates to embedding for vector similarity
  - memory_types: []string, default=[episodic,semantic]. Searches both for broad recall
  - limit: max results (default 10)
  - min_score: composite score threshold to filter low-quality hits (score range [0,1.2], recommend 0.3)
  - min_importance: importance floor, only matches with importance >= N
  - session_id: restrict to a specific conversation session (episodic only)
  - tags: filter by tag (semantic only, match any tag in the comma-separated list)

[retrieve] — Structured field-based query, no vector search. params: memory_type*, query, limit, session_id, event_type, min_importance, tags.
  - memory_type*: "episodic" | "semantic"
  - query: keyword filter on content field (no embedding needed)
  - limit: max results (default 10)
  - session_id: session filter (episodic)
  - event_type: "observation" | "thought" | "action" | "result" (episodic)
  - min_importance: importance floor (episodic & semantic)
  - tags: filter by Neo4j node label (semantic only)
  - NOTE: For content-based search, prefer [search] over [retrieve]. Use [retrieve] for exact field filters.

[forget] — Apply forget strategy to remove low-value memories. params: memory_type*, strategy*, threshold, max_age_days.
  - memory_type*: "episodic" | "semantic"
  - strategy*: "importance_based" (memories with importance < threshold) | "time_based" (older than max_age_days, episodic only)
  - threshold: 0.0~1.0 for importance_based, or capacity for time_based
  - max_age_days: days for time_based strategy

[delete] — Delete a specific memory by ID. params: memory_type*, id*.

[status] — Get statistics (count, time range). params: memory_type*.

* = required parameter

== Guidelines ==

Memory type selection:
  - episodic: conversations, observations, tool calls, actions, results — events tied to a point in time, memory decays with age
  - semantic: reusable knowledge, rules, patterns, technical facts — no time dimension, supports graph relationships via tags
  - working: short-term context for the current session, both writable and searchable

Session ID: System auto-sets SessionId to current conversation. Omit session_id to search across all sessions; include it to scope to current session only.

Importance grading: 0.8+ critical (root cause, key decision), 0.5 normal (observation), 0.3- trivial (intermediate step). Items < 0.3 are candidates for future forget.

Search before answer: Always call [search] with a descriptive query before reasoning about a problem. Include both episodic and semantic for maximum recall. Use min_importance=0.3 to filter noise.

Rich summaries: When adding to semantic memory, provide a clear summary parameter for better vector embedding quality. Keep content complete; make summary concise and search-friendly.

Composite score range: [0, 1.2], combining vector similarity + time recency (episodic) or graph similarity (semantic) x importance boost. Higher score = more relevant.

MANDATORY save: Every task MUST end with [add]. You are the agent — you must persist your own analysis results.
  Golden flow: [search] for context → reason → [add] your conclusion → final answer.
  Save BOTH: (a) what the user told you AND (b) what you discovered through reasoning/tools.
  If you skip [add], the knowledge from this turn is lost forever.

== Few-Shot (pay attention to the mandatory save after analysis) ==
- User says "my name is Smith" → add(working, content="User name is Smith", importance=0.9)
- You search memory, analyze logs, find root cause → add(episodic, content="Root cause: nil pointer at handler.go L42. Fix: add nil guard before dereference", event_type="result", importance=0.9)
- You discover a reusable pattern through debugging → add(semantic, content="Go HTTP handler must validate req.Body != nil before json.Decode", tags="Go,HTTP,error", importance=0.8, summary="Go HTTP body nil check pattern")
- Start any task → search(query="relevant past context keywords", memory_types=["episodic","semantic"], limit=5, min_importance=0.3)
- You finished answering user's question → add(episodic, content="Answered question about X. Key finding: Y", event_type="result", importance=0.7)
- Cleanup after task → forget(memory_type="episodic", strategy="importance_based", threshold=0.2)
`

type MemoryTool struct {
	SessionId   string
	Types       []types.MemoryType
	Manager     *memory.Manager
	description string
}

func NewMemoryTool(
	sessionId string,
	config core.MemoryConfig,
	memoryTypes []types.MemoryType,
	vectorStore store.VectorStore,
	structuredStore store.StructuredStore,
	embeddingService store.EmbeddingService,
	graphStore store.GraphStore,
) (tools.Tool, error) {
	mt := &MemoryTool{
		SessionId: sessionId,
		Types:     memoryTypes,
	}
	if sessionId == "" {
		mt.SessionId = strconv.Itoa(int(time.Now().UnixNano()))
	}
	if len(memoryTypes) == 0 {
		mt.Types = []types.MemoryType{types.Working, types.Episodic, types.Semantic, types.Perceptual}
	}
	mt.description = memoryToolDescription + `Currently available memory types include: ` + fmt.Sprintf(" %v ", memoryTypes)
	manager, err := memory.NewManager(
		config,
		mt.SessionId,
		slices.Contains(mt.Types, types.Working),
		slices.Contains(mt.Types, types.Episodic),
		slices.Contains(mt.Types, types.Semantic),
		slices.Contains(mt.Types, types.Perceptual),
		vectorStore,
		structuredStore,
		embeddingService,
		graphStore,
	)
	if err != nil {
		return nil, err
	}
	mt.Manager = manager
	return mt, nil
}

func (m *MemoryTool) Name() string {
	return "Memory"
}

func (m *MemoryTool) Description() string {
	return m.description
}

func (m *MemoryTool) Parameters() []tools.ToolParameter {
	return []tools.ToolParameter{
		{Name: "action", Type: tools.ParamString, Description: "Action to perform: add, search, retrieve, forget, delete, status", Required: true},
		{Name: "memory_type", Type: tools.ParamString, Description: "Target memory type: working, episodic, semantic. System has dynamically informed you which types are currently available.", Required: false},
		{Name: "content", Type: tools.ParamString, Description: "Content to store (for add)", Required: false},
		{Name: "importance", Type: tools.ParamNumber, Description: "Importance 0..1, default 0.5 (for add/search)", Required: false},
		{Name: "summary", Type: tools.ParamString, Description: "Summary for embedding (for add), defaults to content", Required: false},
		{Name: "event_type", Type: tools.ParamString, Description: "Event type: observation, thought, action, result (for episodic add)", Required: false},
		{Name: "tags", Type: tools.ParamString, Description: "Comma-separated tags (for semantic add/retrieve/search) MUST be one of: Concept, Rule, Tool", Required: false},
		{Name: "query", Type: tools.ParamString, Description: "Search or retrieve query text", Required: false},
		{Name: "memory_types", Type: tools.ParamArray, Description: "Memory types to search across (for search)", Required: false, ItemsType: tools.ParamString},
		{Name: "limit", Type: tools.ParamInteger, Description: "Max results, default 10", Required: false},
		{Name: "min_score", Type: tools.ParamNumber, Description: "Minimum composite score threshold (for search)", Required: false},
		{Name: "min_importance", Type: tools.ParamNumber, Description: "Minimum importance filter (for search/retrieve)", Required: false},
		{Name: "session_id", Type: tools.ParamString, Description: "Filter by session ID (for search/retrieve)", Required: false},
		{Name: "strategy", Type: tools.ParamString, Description: "Forget strategy: importance_based, time_based", Required: false},
		{Name: "threshold", Type: tools.ParamNumber, Description: "Forget threshold (for importance_based: 0..1, for time_based: capacity)", Required: false},
		{Name: "max_age_days", Type: tools.ParamInteger, Description: "Max age in days for time_based forget", Required: false},
		{Name: "id", Type: tools.ParamString, Description: "Memory item ID (for delete)", Required: false},
		{Name: "relations", Type: tools.ParamString, Description: "JSON array (for add). episodic relation_type MUST be one of: before, after, caused_by, related_to. semantic: any string. Example episodic: [{\"target_id\":\"<id>\",\"relation_type\":\"caused_by\"}]", Required: false},
	}
}

func (m *MemoryTool) Run(parameters map[string]interface{}) (string, error) {
	action, ok := parameters["action"].(string)
	if !ok {
		return "", fmt.Errorf("action is required")
	}

	logger.Printf("memory action [%s]", action)
	logger.Printf("memory params [%v]", parameters)

	switch action {
	case "add":
		return m.runAdd(parameters)
	case "search":
		return m.runSearch(parameters)
	case "retrieve":
		return m.runRetrieve(parameters)
	case "forget":
		return m.runForget(parameters)
	case "delete":
		return m.runDelete(parameters)
	case "status":
		return m.runStatus(parameters)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (m *MemoryTool) runAdd(p map[string]interface{}) (string, error) {
	mt, err := parseMemoryType(p, "memory_type")
	if err != nil {
		return "", err
	}
	content, ok := p["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("content is required for add")
	}

	item := types.MemoryItem{
		Content:    content,
		Importance: parseFloat(p, "importance", 0.5),
		SessionID:  m.SessionId,
		Metadata:   make(map[string]string),
	}

	if v, ok := p["summary"].(string); ok && v != "" {
		item.Metadata["summary"] = v
	}
	if v, ok := p["event_type"].(string); ok && v != "" {
		item.Metadata["event_type"] = v
	}
	if v, ok := p["tags"].(string); ok && v != "" {
		item.Metadata["tags"] = v
	}
	if v, ok := p["relations"].(string); ok && v != "" {
		item.Metadata["relations"] = v
	}

	id, err := m.Manager.Add(mt, item)
	if err != nil {
		return "", err
	}
	return m.jsonResult("added", map[string]interface{}{"id": id, "memory_type": string(mt)})
}

func (m *MemoryTool) runSearch(p map[string]interface{}) (string, error) {
	memoryTypes := parseMemoryTypes(p, "memory_types")
	if len(memoryTypes) == 0 {
		memoryTypes = m.Types
	}

	query, _ := p["query"].(string)

	opts := types.SearchOptions{
		Limit:         parseInt(p, "limit", 10),
		MinScore:      parseFloat(p, "min_score", 0),
		MinImportance: parseFloat(p, "min_importance", 0),
		Filter:        parseFilter(p),
	}

	items, err := m.Manager.Search(query, memoryTypes, opts)
	if err != nil {
		return "", err
	}
	return m.jsonResult("searched", map[string]interface{}{
		"count":   len(items),
		"results": items,
	})
}

func (m *MemoryTool) runRetrieve(p map[string]interface{}) (string, error) {
	mt, err := parseMemoryType(p, "memory_type")
	if err != nil {
		return "", err
	}
	query, _ := p["query"].(string)
	limit := parseInt(p, "limit", 10)

	metadata := parseFilter(p)
	items, err := m.Manager.Retrieve(mt, query, limit, metadata)
	if err != nil {
		return "", err
	}
	return m.jsonResult("retrieved", map[string]interface{}{
		"count":   len(items),
		"results": items,
	})
}

func (m *MemoryTool) runForget(p map[string]interface{}) (string, error) {
	mt, err := parseMemoryType(p, "memory_type")
	if err != nil {
		return "", err
	}
	strategyStr, _ := p["strategy"].(string)
	strategy := types.ForgotStrategy(strategyStr)
	threshold := parseFloat(p, "threshold", 0.5)
	maxAgeDays := parseInt(p, "max_age_days", 30)

	count, err := m.Manager.Forget(mt, strategy, threshold, int64(maxAgeDays))
	if err != nil {
		return "", err
	}
	return m.jsonResult("forgotten", map[string]interface{}{"count": count, "memory_type": string(mt)})
}

func (m *MemoryTool) runDelete(p map[string]interface{}) (string, error) {
	mt, err := parseMemoryType(p, "memory_type")
	if err != nil {
		return "", err
	}
	id, ok := p["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("id is required for delete")
	}
	if err := m.Manager.Delete(mt, id); err != nil {
		return "", err
	}
	return m.jsonResult("deleted", map[string]interface{}{"id": id, "memory_type": string(mt)})
}

func (m *MemoryTool) runStatus(p map[string]interface{}) (string, error) {
	mt, err := parseMemoryType(p, "memory_type")
	if err != nil {
		return "", err
	}
	status, err := m.Manager.Status(mt)
	if err != nil {
		return "", err
	}
	return m.jsonResult("status", map[string]interface{}{
		"memory_type": string(mt),
		"count":       status.Count,
		"oldest_item": status.OldestItem,
		"newest_item": status.NewestItem,
	})
}

func parseMemoryType(p map[string]interface{}, key string) (types.MemoryType, error) {
	v, ok := p[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return types.MemoryType(v), nil
}

func parseMemoryTypes(p map[string]interface{}, key string) []types.MemoryType {
	arr, ok := p[key].([]interface{})
	if !ok {
		return nil
	}
	result := make([]types.MemoryType, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			result = append(result, types.MemoryType(s))
		}
	}
	return result
}

func parseFloat(p map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := p[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return f
			}
		}
	}
	return defaultVal
}

func parseInt(p map[string]interface{}, key string, defaultVal int) int64 {
	if v, ok := p[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int:
			return int64(n)
		case string:
			if i, err := strconv.ParseInt(n, 10, 64); err == nil {
				return i
			}
		}
	}
	return int64(defaultVal)
}

func parseFilter(p map[string]interface{}) map[string]string {
	filter := make(map[string]string)
	for _, k := range []string{"session_id", "event_type", "tags", "min_importance"} {
		if v, ok := p[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				filter[k] = s
			}
		}
	}
	return filter
}

func (m *MemoryTool) jsonResult(action string, data map[string]interface{}) (string, error) {
	result := map[string]interface{}{
		"action": action,
	}
	for k, v := range data {
		result[k] = v
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
