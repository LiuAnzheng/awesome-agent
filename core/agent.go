package core

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

type BaseAgent struct {
	Name         string
	LLM          LLMInterface
	SystemPrompt string
	history      []Message
	Config       AppConfig
}

func (agent *BaseAgent) AddMessage(message Message) {
	if len(agent.history) > 1024 {
		agent.history = agent.history[:512]
	}
	agent.history = append(agent.history, message)
}

func (agent *BaseAgent) History() []Message {
	history := make([]Message, len(agent.history))
	copy(history, agent.history)
	return history
}
func (agent *BaseAgent) String() string {
	return "Agent name:" + agent.Name + " Provider:" + agent.LLM.Provider()
}
