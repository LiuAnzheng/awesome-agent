package gssc

import (
	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/ctx"
	"github.com/LiuAnzheng/memoria/tools/builtins"
	"log/slog"
	"strconv"
	"time"
)

type GathererImpl struct {
	SessionID string
	mt        *builtins.MemoryTool
	rt        *builtins.RAGTool
}

func NewGatherer(sessionID string, mt *builtins.MemoryTool, rt *builtins.RAGTool) *GathererImpl {
	return &GathererImpl{
		SessionID: sessionID,
		mt:        mt,
		rt:        rt,
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
