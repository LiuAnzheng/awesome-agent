package builtins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/note"
	"github.com/LiuAnzheng/memoria/tools"
	"gopkg.in/yaml.v3"
)

type NoteTool struct {
	mu        sync.RWMutex
	workspace string
	index     note.NoteIndex

	noteSequence uint64
}

func NewNoteTool(workspace string) tools.Tool {
	nt := &NoteTool{
		workspace:    workspace,
		index:        note.NoteIndex{},
		noteSequence: 1,
		mu:           sync.RWMutex{},
	}
	if nt.workspace == "" {
		nt.workspace = "./data/notes"
	}
	err := nt.loadIndex()
	if err != nil {
		slog.Warn("load index failed", "err", err)
	}
	return nt
}

func (n *NoteTool) Name() string {
	return "note_tool"
}

func (n *NoteTool) Description() string {
	return `═══ STRUCTURED NOTE-TAKING ═══

Notes are for LONG-FORM, PERSISTENT documents. Unlike memory_tool (quick fact recall),
notes suit tasks with clear structure spanning multiple turns:

  Academic Research — literature review, methodology, experimental results, citations
  Coding Sessions  — architecture decisions, bug tracking, implementation plans, code review
  Deep Analysis    — multi-step reasoning, comparative studies, trade-off analysis, audit trails

DO use note_tool when: the task has phases, you need to track state across turns, you're
  producing a deliverable (report, plan, audit), or you need to reference findings later.
Do NOT use note_tool for: one-off facts, short Q&A, transient state (use memory_tool instead).

═══ NOTE TYPES ═══

  task_state  — Current progress, "done / next", sprint status, experiment phase
  conclusion  — Final decisions, analysis results, architectural verdicts, research findings
  blocker     — Unresolved issues, missing dependencies, unknowns blocking progress
  action      — Concrete TODOs, action items, next steps
  reference   — External links, API docs, code snippets, paper citations, config examples
  general     — Catch-all for notes that don't fit the above

═══ WORKFLOW ═══

  1. search(query, note_type?) BEFORE creating — avoid duplicates, find related notes
  2. create(title, content, note_type, tags) to persist findings
  3. update(note_id, ...) as understanding evolves
  4. summary() periodically to check overall progress and type distribution`
}

func (n *NoteTool) Run(parameters map[string]interface{}) (string, error) {
	action, _ := parameters["action"].(string)
	switch action {
	case "create":
		title, _ := parameters["title"].(string)
		content, _ := parameters["content"].(string)
		noteType := note.NoteType(getString(parameters, "note_type"))
		tags := toStringSlice(parameters["tags"])
		return n.create(title, noteType, tags, content)
	case "read":
		noteID, _ := parameters["note_id"].(string)
		return n.read(noteID)
	case "update":
		noteID, _ := parameters["note_id"].(string)
		title, _ := parameters["title"].(string)
		content, _ := parameters["content"].(string)
		noteType := note.NoteType(getString(parameters, "note_type"))
		tags := toStringSlice(parameters["tags"])
		return n.update(noteID, title, content, noteType, tags)
	case "delete":
		noteID, _ := parameters["note_id"].(string)
		return n.delete(noteID)
	case "search":
		query, _ := parameters["query"].(string)
		limit := int(toInt64(parameters["limit"]))
		noteType := note.NoteType(getString(parameters, "note_type"))
		tags := toStringSlice(parameters["tags"])
		return n.search(query, limit, noteType, tags)
	case "list":
		limit := int(toInt64(parameters["limit"]))
		return n.list(limit)
	case "summary":
		return n.summary(), nil
	default:
		return "", fmt.Errorf("unknown action: %s (valid: create, read, update, delete, search, list, summary)", action)
	}
}

func (n *NoteTool) Parameters() []tools.ToolParameter {
	return []tools.ToolParameter{
		{
			Name: "action", Type: tools.ParamString, Required: true,
			Description: "Operation: create, read, update, delete, search, list, summary",
		},
		{
			Name: "title", Type: tools.ParamString, Required: false,
			Description: "[create/update] Note title",
		},
		{
			Name: "content", Type: tools.ParamString, Required: false,
			Description: "[create/update] Note content (Markdown)",
		},
		{
			Name: "note_type", Type: tools.ParamString, Required: false, Default: "general",
			Description: "[create/update/search] task_state, conclusion, blocker, action, reference, general",
		},
		{
			Name: "tags", Type: tools.ParamArray, Required: false, ItemsType: tools.ParamString,
			Description: "[create/update/search] Tag list for filtering",
		},
		{
			Name: "note_id", Type: tools.ParamString, Required: false,
			Description: "[read/update/delete] Target note ID",
		},
		{
			Name: "query", Type: tools.ParamString, Required: false,
			Description: "[search] Keyword to match in title and content",
		},
		{
			Name: "limit", Type: tools.ParamInteger, Required: false, Default: 10,
			Description: "[search/list] Max results returned",
		},
	}
}

func getString(params map[string]interface{}, key string) string {
	s, _ := params[key].(string)
	return s
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func (n *NoteTool) create(title string, noteType note.NoteType, tags []string, content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("content can not be empty")
	}
	if title == "" {
		return "", fmt.Errorf("title can not be empty")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	now := core.Now()
	ts := now.Format("20060102_150405")
	noteID := fmt.Sprintf("note_%s_%03d", ts, n.noteSequence)

	metadata := note.NoteMetadata{
		ID:        noteID,
		Title:     title,
		Type:      noteType,
		Tags:      tags,
		CreatedAt: now.Format("2006-01-02 15:04:05"),
		UpdatedAt: now.Format("2006-01-02 15:04:05"),
	}
	filePath := filepath.Join(n.workspace, noteID+".md")
	metadata.FilePath = filePath

	yamlHeader, _ := yaml.Marshal(metadata)
	fileContent := fmt.Sprintf("---\n%s---\n\n%s", string(yamlHeader), content)

	if err := os.MkdirAll(n.workspace, 0755); err != nil {
		return "", fmt.Errorf("create workspace dir failed: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return "", fmt.Errorf("write note file failed: %w", err)
	}

	n.index[noteID] = metadata
	n.noteSequence++

	if err := n.saveIndex(); err != nil {
		return "", fmt.Errorf("save index failed: %w", err)
	}

	return noteID, nil
}

func (n *NoteTool) read(noteID string) (string, error) {
	if noteID == "" {
		return "", fmt.Errorf("noteID can not be empty")
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	metadata, ok := n.index[noteID]
	if !ok {
		return "", fmt.Errorf("note id %s not exists", noteID)
	}
	bytes, err := os.ReadFile(metadata.FilePath)
	if err != nil {
		return "", fmt.Errorf("read note file failed: %w", err)
	}
	return string(bytes), nil
}

func (n *NoteTool) update(noteID string, title string, content string, noteType note.NoteType, tags []string) (string, error) {
	if noteID == "" {
		return "", fmt.Errorf("noteID can not be empty")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	metadata, ok := n.index[noteID]
	if !ok {
		return "", fmt.Errorf("note id %s not exists", noteID)
	}

	// 鍘熷鍐呭
	oldContent, e := n.getNoteContent(metadata)

	if e != nil {
		return "", fmt.Errorf("get old content failed: %w", e)
	}

	if title != "" {
		metadata.Title = title
	}
	if noteType != "" {
		metadata.Type = noteType
	}
	if tags != nil {
		metadata.Tags = tags
	}
	if content != "" {
		oldContent = content
	}
	metadata.UpdatedAt = core.Now().Format("2006-01-02 15:04:05")

	yamlHeader, _ := yaml.Marshal(metadata)
	fileContent := fmt.Sprintf("---\n%s---\n\n%s", string(yamlHeader), oldContent)
	filePath := filepath.Join(n.workspace, noteID+".md")
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return "", fmt.Errorf("write note file failed: %w", err)
	}

	n.index[noteID] = metadata
	err := n.saveIndex()
	if err != nil {
		return "", fmt.Errorf("save index failed: %w", err)
	}
	return "SUCCESS", nil
}

func (n *NoteTool) delete(noteID string) (string, error) {
	if noteID == "" {
		return "", fmt.Errorf("noteID can not be empty")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	metadata, ok := n.index[noteID]
	if !ok {
		return "", fmt.Errorf("note id %s not exists", noteID)
	}

	if metadata.FilePath != "" {
		if err := os.Remove(metadata.FilePath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("delete note file failed: %w", err)
		}
	}

	title := metadata.Title
	delete(n.index, noteID)

	if err := n.saveIndex(); err != nil {
		return "", fmt.Errorf("save index failed: %w", err)
	}

	return fmt.Sprintf("note deleted: %s", title), nil
}

func (n *NoteTool) search(query string, limit int, noteType note.NoteType, tags []string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query can not be empty")
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	queryLower := strings.ToLower(query)
	res := make([]note.NoteMetadata, 0)
	for _, metadata := range n.index {
		if noteType != "" && noteType != metadata.Type {
			continue
		}
		if tags != nil && len(tags) > 0 {
			set := make(map[string]struct{})
			for _, tag := range metadata.Tags {
				set[tag] = struct{}{}
			}
			contains := false
			for _, tag := range tags {
				if _, ok := set[tag]; ok {
					contains = true
					break
				}
			}
			if !contains {
				continue
			}
		}
		noteContent, err := n.getNoteContent(metadata)
		if err != nil {
			slog.Warn("get note content fail ", "err", err)
			continue
		}
		noteContentLower := strings.ToLower(noteContent)
		titleLower := strings.ToLower(metadata.Title)
		if !strings.Contains(noteContentLower, queryLower) && !strings.Contains(titleLower, queryLower) {
			continue
		}
		res = append(res, metadata)
	}
	slices.SortFunc(res, func(m1, m2 note.NoteMetadata) int {
		return strings.Compare(m2.UpdatedAt, m1.UpdatedAt)
	})
	res = res[:min(limit, len(res))]
	bytes, _ := json.Marshal(res)
	return string(bytes), nil
}

func (n *NoteTool) list(limit int) (string, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	res := make([]note.NoteMetadata, 0)
	for _, metadata := range n.index {
		res = append(res, metadata)
	}
	slices.SortFunc(res, func(m1, m2 note.NoteMetadata) int {
		return strings.Compare(m2.UpdatedAt, m1.UpdatedAt)
	})
	bytes, _ := json.Marshal(res[:min(limit, len(res))])
	return string(bytes), nil
}

type summaryResult struct {
	TotalNotes       int                   `json:"total_notes"`
	TypeDistribution map[note.NoteType]int `json:"type_distribution"`
	RecentNotes      []note.NoteMetadata   `json:"recent_notes"`
}

func (n *NoteTool) summary() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	cnt := len(n.index)
	metaSlice := make([]note.NoteMetadata, 0)
	typeCnt := make(map[note.NoteType]int)
	for _, metadata := range n.index {
		typeCnt[metadata.Type]++
		metaSlice = append(metaSlice, metadata)
	}
	slices.SortFunc(metaSlice, func(m1, m2 note.NoteMetadata) int {
		return strings.Compare(m2.UpdatedAt, m1.UpdatedAt)
	})
	metaSlice = metaSlice[:min(cnt, 5)]
	summary := summaryResult{
		TotalNotes:       cnt,
		TypeDistribution: typeCnt,
		RecentNotes:      metaSlice,
	}
	bytes, _ := json.Marshal(summary)
	return string(bytes)
}

func (n *NoteTool) saveIndex() error {
	indexPath := filepath.Join(n.workspace, "index.json")
	data, err := json.Marshal(n.index)
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}

func (n *NoteTool) getNoteContent(metadata note.NoteMetadata) (string, error) {
	bytes, err := os.ReadFile(metadata.FilePath)
	if err != nil {
		return "", fmt.Errorf("read note file failed: %w", err)
	}
	yamlHeader, _ := yaml.Marshal(metadata)
	split := fmt.Sprintf("---\n%s---\n\n", string(yamlHeader))
	strs := strings.Split(string(bytes), split)
	if len(strs) < 2 {
		return "", fmt.Errorf("note content is empty")
	}
	return strs[1], nil
}

func (n *NoteTool) loadIndex() error {
	indexPath := filepath.Join(n.workspace, "index.json")
	bytes, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read index file failed: %w", err)
	}
	index := make(map[string]note.NoteMetadata)
	err = json.Unmarshal(bytes, &index)
	if err != nil {
		return fmt.Errorf("unmarshal index file failed: %w", err)
	}
	n.index = index
	return nil
}
