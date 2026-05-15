package core

import (
	"log"
	"os"
)

type FinishReasonType string

const (
	Stop          FinishReasonType = "stop"
	Length        FinishReasonType = "length"
	ToolCalls     FinishReasonType = "tool_calls"
	ContentFilter FinishReasonType = "content_filter"
)

func ParseFinishReason(value string) FinishReasonType {
	switch value {
	case "stop":
		return Stop
	case "length":
		return Length
	case "tool_calls":
		return ToolCalls
	case "content_filter":
		return ContentFilter
	default:
		return Stop
	}
}

var logger = log.New(os.Stderr, "[core] ", log.LstdFlags|log.Lshortfile)

type BaseAgent struct {
	Name         string
	LLM          LLMInterface
	SystemPrompt string
	history      []Message
	Config       AgentConfig
}

func (agent *BaseAgent) AddMessage(message Message) {
	agent.history = append(agent.history, message)
}
func (agent *BaseAgent) ClearHistory() {
	agent.history = []Message{}
}
func (agent *BaseAgent) History() []Message {
	history := make([]Message, len(agent.history))
	copy(history, agent.history)
	return history
}
func (agent *BaseAgent) String() string {
	return "Agent name:" + agent.Name + " Provider:" + agent.LLM.Provider()
}
