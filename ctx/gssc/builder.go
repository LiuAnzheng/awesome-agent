package gssc

import (
	"log/slog"

	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/ctx"
	"github.com/LiuAnzheng/memoria/tools/builtins"
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
	nt *builtins.NoteTool,
	sessionID string,
) *ContextBuilder {
	cb := &ContextBuilder{
		config: cfg,
	}
	cb.gatherer = NewGatherer(sessionID, mt, rt, nt)
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

	budget := float64(cb.config.MaxTokens) * (1 - cb.config.ReserveRatio)
	packets = cb.selector.Select(packets, query, int64(budget))

	slog.Info("gssc recall packets ", "count", len(packets))

	structured := cb.structurer.Structure(packets, query)

	return structured
}
