package gssc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/ctx"
	"github.com/LiuAnzheng/memoria/note"
	"github.com/LiuAnzheng/memoria/tools/builtins"
)

type GathererImpl struct {
	SessionID string
	mt        *builtins.MemoryTool
	rt        *builtins.RAGTool
	nt        *builtins.NoteTool
}

func NewGatherer(sessionID string, mt *builtins.MemoryTool, rt *builtins.RAGTool,
	nt *builtins.NoteTool) *GathererImpl {
	return &GathererImpl{
		SessionID: sessionID,
		mt:        mt,
		rt:        rt,
		nt:        nt,
	}
}

func (g *GathererImpl) Gather(
	userQuery string,
	history []core.Message,
	systemInstructions string,
	customPackets []ctx.ContextPacket,
) []ctx.ContextPacket {
	packets := make([]ctx.ContextPacket, 0)

	// 系统核心指令
	if systemInstructions != "" {
		packets = append(packets, ctx.ContextPacket{
			Content:        systemInstructions,
			Timestamp:      core.Now(),
			TokenCount:     EstimateTokens(systemInstructions),
			Source:         ctx.SystemInstructions,
			RelevanceScore: 1.0,
		})
	}

	// 记忆
	g.collectMemory(userQuery, &packets)

	// RAG
	g.collectRAG(userQuery, &packets)

	// 历史消息
	g.collectHistory(history, &packets)

	// 笔记
	g.collectNotes(userQuery, &packets)

	// 自定义上下文
	for _, p := range customPackets {
		packets = append(packets, p)
	}

	return packets
}

func (g *GathererImpl) collectMemory(query string, packets *[]ctx.ContextPacket) {
	if g.mt == nil {
		return
	}

	manager, err := g.mt.GetManager(g.SessionID)
	if err != nil {
		slog.Warn("gatherer: failed to get memory manager", "session_id", g.SessionID, "error", err)
		return
	}

	items, err := g.mt.RunSearch(manager, map[string]interface{}{
		"query": query,
	})
	if err != nil {
		slog.Warn("gatherer: memory search failed", "session_id", g.SessionID, "error", err)
		return
	}

	for _, item := range items {
		score := parseScore(item.Metadata["score"], item.Importance)
		ts := itemTime(item.CreatedAt)

		packet := ctx.ContextPacket{
			Content:        item.Content,
			Timestamp:      ts,
			TokenCount:     EstimateTokens(item.Content),
			RelevanceScore: score,
			Source:         ctx.Memory,
			Metadata:       copyMap(item.Metadata),
		}
		packet.Metadata["memory_type"] = string(item.Type)
		*packets = append(*packets, packet)
	}
}

func (g *GathererImpl) collectRAG(query string, packets *[]ctx.ContextPacket) {
	if g.rt == nil {
		return
	}

	items, err := g.rt.RunSearch(map[string]interface{}{
		"query": query,
	})
	if err != nil {
		slog.Warn("gatherer: RAG search failed", "error", err)
		return
	}

	for _, item := range items {
		*packets = append(*packets, ctx.ContextPacket{
			Content:        item.Content,
			Timestamp:      itemTime(item.CreatedAt),
			TokenCount:     EstimateTokens(item.Content),
			Source:         ctx.RAG,
			RelevanceScore: item.Score,
		})
	}
}

func (g *GathererImpl) collectHistory(history []core.Message, packets *[]ctx.ContextPacket) {
	if len(history) == 0 {
		return
	}

	limit := min(len(history), 32)
	for _, msg := range history[len(history)-limit:] {
		content := msg.String()

		*packets = append(*packets, ctx.ContextPacket{
			Content:        content,
			Timestamp:      msg.Timestamp,
			TokenCount:     EstimateTokens(content),
			Source:         ctx.History,
			RelevanceScore: 0.6,
		})
	}
}

func (g *GathererImpl) collectNotes(query string, packets *[]ctx.ContextPacket) {
	if g.nt == nil {
		return
	}
	noteSlice := make([]note.NoteMetadata, 0)
	recallNotes := make(map[string]struct{})

	blockerNote, err := g.nt.List(5, note.NoteTypeBlocker, nil)
	if err != nil {
		slog.Warn("gatherer: failed to list notes", "session_id", g.SessionID, "error", err)
		return
	}
	blockerSlice := make([]note.NoteMetadata, 0)
	_ = json.Unmarshal([]byte(blockerNote), &blockerSlice)
	for _, blocker := range blockerSlice {
		if _, ok := recallNotes[blocker.ID]; !ok {
			noteSlice = append(noteSlice, blocker)
			recallNotes[blocker.ID] = struct{}{}
		}
	}

	semanticNote, err := g.nt.Search(query, 5, "", nil)
	if err != nil {
		slog.Warn("gatherer: failed to search notes", "session_id", g.SessionID, "error", err)
		return
	}
	semanticSlice := make([]note.NoteMetadata, 0)
	_ = json.Unmarshal([]byte(semanticNote), &semanticSlice)
	for _, semantic := range semanticSlice {
		if _, ok := recallNotes[semantic.ID]; !ok {
			noteSlice = append(noteSlice, semantic)
			recallNotes[semantic.ID] = struct{}{}
		}
	}

	noteSlice = noteSlice[:min(5, len(noteSlice))]

	var resMap sync.Map

	wg := sync.WaitGroup{}
	for _, metadata := range noteSlice {
		wg.Add(1)
		go g.getNoteContent(&wg, metadata, &resMap)
	}
	wg.Wait()
	for _, metadata := range noteSlice {
		content, ok := resMap.Load(metadata.ID)
		if !ok {
			continue
		}
		updateTime, err := time.Parse("2006-01-02 15:04:05", metadata.UpdatedAt)
		if err != nil {
			slog.Warn("gatherer: failed to parse updated time", "session_id", g.SessionID, "error", err)
			updateTime = core.Now()
		}
		strContent := content.(string)
		packet := ctx.ContextPacket{
			Content:        strContent,
			Timestamp:      updateTime,
			TokenCount:     EstimateTokens(strContent),
			RelevanceScore: noteRelevanceScore(metadata.Type),
			Source:         ctx.Note,
		}
		*packets = append(*packets, packet)
	}
}

func noteRelevanceScore(t note.NoteType) float64 {
	switch t {
	case note.NoteTypeBlocker:
		return 0.85
	case note.NoteTypeAction:
		return 0.80
	default:
		return 0.75
	}
}

func (g *GathererImpl) getNoteContent(wg *sync.WaitGroup, metadata note.NoteMetadata, resMap *sync.Map) {
	defer wg.Done()
	content, err := g.nt.GetNoteContent(metadata)
	if err != nil {
		slog.Warn("gatherer: failed to get note content", "session_id", g.SessionID, "error", err)
		return
	}
	content = fmt.Sprintf("[Note: %s] (%s)\n%s", metadata.Title, metadata.Type, content)
	resMap.Store(metadata.ID, content)
}

func parseScore(raw string, fallback float64) float64 {
	if raw == "" {
		return fallback
	}
	score, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return score
}

func itemTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

func copyMap(m map[string]string) map[string]interface{} {
	res := make(map[string]interface{}, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}
