package gssc

import (
	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/ctx"
	"github.com/LiuAnzheng/memoria/memory/retrieval"
	"log/slog"
	"math"
	"sort"
	"time"
)

type RelevanceFunc func(content, query string) float64

type RecencyFunc func(t time.Time) float64

type SelectorImpl struct {
	relevanceWeight float64
	recencyWeight   float64
	minRelevance    float64
	relevanceFn     RelevanceFunc
	recencyFn       RecencyFunc
}

type SelectorOption func(*SelectorImpl)

func NewSelector(opts ...SelectorOption) *SelectorImpl {
	s := &SelectorImpl{
		relevanceWeight: 0.7,
		recencyWeight:   0.3,
		minRelevance:    0.1,
		relevanceFn:     jaccardRelevance,
		recencyFn:       expDecayRecency,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func WithRelevanceWeight(w float64) SelectorOption {
	return func(s *SelectorImpl) { s.relevanceWeight = w }
}

func WithRecencyWeight(w float64) SelectorOption {
	return func(s *SelectorImpl) { s.recencyWeight = w }
}

func WithMinRelevance(r float64) SelectorOption {
	return func(s *SelectorImpl) { s.minRelevance = r }
}

func WithRelevanceFunc(fn RelevanceFunc) SelectorOption {
	return func(s *SelectorImpl) { s.relevanceFn = fn }
}

func WithRecencyFunc(fn RecencyFunc) SelectorOption {
	return func(s *SelectorImpl) { s.recencyFn = fn }
}

// FromConfig 从 ContextConfig 一键配置 selectorImpl。
func FromConfig(cfg core.ContextConfig) SelectorOption {
	return func(s *SelectorImpl) {
		if cfg.RelevanceWeight > 0 {
			s.relevanceWeight = cfg.RelevanceWeight
		}
		if cfg.RecencyWeight > 0 {
			s.recencyWeight = cfg.RecencyWeight
		}
		if cfg.MinRelevance > 0 {
			s.minRelevance = cfg.MinRelevance
		}
	}
}

func (s *SelectorImpl) Select(packets []ctx.ContextPacket, query string, budget int64) []ctx.ContextPacket {
	systemPackets, otherPackets := s.splitBySystem(packets)

	systemTokens := sumTokens(systemPackets)
	remaining := budget - systemTokens
	if remaining <= 0 {
		slog.Warn("gatherer: system instructions exhausted token budget",
			"system_tokens", systemTokens, "budget", budget)
		return systemPackets
	}

	type scoredPacket struct {
		packet ctx.ContextPacket
		score  float64
	}
	scored := make([]scoredPacket, 0, len(otherPackets))
	for _, p := range otherPackets {
		relevance := p.RelevanceScore
		if relevance < 0 {
			relevance = s.relevanceFn(p.Content, query)
		}
		if relevance < s.minRelevance {
			continue
		}

		combined := s.relevanceWeight * relevance
		if s.skipRecency(p) {
			combined += s.recencyWeight * relevance
		} else {
			combined += s.recencyWeight * s.recencyFn(p.Timestamp)
		}

		scored = append(scored, scoredPacket{packet: p, score: combined})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	selected := make([]ctx.ContextPacket, 0, len(systemPackets)+len(scored))
	selected = append(selected, systemPackets...)
	currentTokens := systemTokens
	for _, sc := range scored {
		if currentTokens+sc.packet.TokenCount > budget {
			continue
		}
		selected = append(selected, sc.packet)
		currentTokens += sc.packet.TokenCount
	}

	return selected
}

func (s *SelectorImpl) splitBySystem(packets []ctx.ContextPacket) (system, other []ctx.ContextPacket) {
	system = make([]ctx.ContextPacket, 0)
	other = make([]ctx.ContextPacket, 0, len(packets))
	for _, p := range packets {
		if p.Source == ctx.SystemInstructions {
			system = append(system, p)
		} else {
			other = append(other, p)
		}
	}
	return
}

func (s *SelectorImpl) skipRecency(p ctx.ContextPacket) bool {
	if p.Source != ctx.Memory {
		return false
	}
	t, ok := p.Metadata["memory_type"]
	return ok && t == "semantic"
}

func sumTokens(packets []ctx.ContextPacket) int64 {
	var total int64
	for _, p := range packets {
		total += p.TokenCount
	}
	return total
}

func jaccardRelevance(content, query string) float64 {
	tokenizer := retrieval.GetTokenizer()
	cw := tokenSet(tokenizer, content)
	qw := tokenSet(tokenizer, query)
	if len(qw) == 0 {
		return 0
	}
	var common int
	for w := range qw {
		if cw[w] {
			common++
		}
	}
	union := len(cw) + len(qw) - common
	if union == 0 {
		return 0
	}
	return float64(common) / float64(union)
}

func tokenSet(t *retrieval.Tokenizer, text string) map[string]bool {
	tokens := t.CutForQuery(text)
	set := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		set[tok] = true
	}
	return set
}

func expDecayRecency(t time.Time) float64 {
	if t.IsZero() {
		return 0.1
	}
	ageHours := time.Since(t).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	score := math.Exp(-0.01 * ageHours / 24)
	return max(0.1, min(1.0, score))
}
