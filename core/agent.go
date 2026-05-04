package core

import (
	"log"
	"os"
)

var logger = log.New(os.Stderr, "[core] ", log.LstdFlags|log.Lshortfile)

type BaseAgent struct {
	Name         string
	LLM          AwesomeLLMClient
	SystemPrompt string
	history      []Message
	Config       Config
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
	return "Agent name:" + agent.Name + " Provider:" + agent.LLM.Provider
}
