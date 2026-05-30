package ctx

import (
	"github.com/LiuAnzheng/memoria/core"
	"time"
)

type ContextSource string

const (
	Memory             ContextSource = "memory"
	RAG                ContextSource = "rag"
	SystemInstructions ContextSource = "systemInstructions"
	History            ContextSource = "history"
	Custom             ContextSource = "custom"
)

type ContextPacket struct {
	Content        string
	Timestamp      time.Time
	TokenCount     int64
	RelevanceScore float64
	Source         ContextSource
	Metadata       map[string]interface{}
}

type Gatherer interface {
	Gather(userQuery string,
		history []core.Message,
		systemInstructions string,
		customPackets []ContextPacket) []ContextPacket
}

type Selector interface {
	Select(packets []ContextPacket, query string, budget int64) []ContextPacket
}

type Structurer interface {
	Structure(packets []ContextPacket, query string) string
}
