package gssc

import (
	"awesome-agent/core"
	"awesome-agent/ctx"
	"awesome-agent/tools/builtins"
	"log/slog"
)

type ContextBuilder struct {
	gatherer   ctx.Gatherer
	selector   ctx.Selector
	structurer ctx.Structurer
	config     core.ContextConfig
}

func NewContextBuilder(
	cfg core.ContextConfig,
	mt *builtins.MemoryTool,
	rt *builtins.RAGTool,
	sessionID string,
) *ContextBuilder {
	cb := &ContextBuilder{
		config: cfg,
	}
	cb.gatherer = NewGatherer(sessionID, mt, rt)
	cb.selector = NewSelector(WithRecencyWeight(cfg.RecencyWeight),
		WithRelevanceWeight(cfg.RelevanceWeight),
		WithMinRelevance(cfg.MinRelevance))
	cb.structurer = NewStructurer()
	return cb
}

func (cb *ContextBuilder) Build(
	query string,
	history []core.Message,
	systemInstructions string,
	customPackets []ctx.ContextPacket,
) string {
	packets := cb.gatherer.Gather(query, history, systemInstructions, customPackets)
	slog.Debug("gssc: gathered", "count", len(packets))

	budget := float64(cb.config.MaxTokens) * (1 - cb.config.ReserveRatio)
	packets = cb.selector.Select(packets, query, int64(budget))
	slog.Debug("gssc: selected", "count", len(packets))

	structured := cb.structurer.Structure(packets, query)

	slog.Debug("gssc: structured", "content", structured)

	return structured
}
