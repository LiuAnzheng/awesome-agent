package core

var DefaultAgentConfig = AgentConfig{
	Temperature:      0.7,
	MaxTokens:        1024,
	TopP:             1.0,
	OpenAIExtraInfo:  nil,
	Debug:            true,
	MaxHistoryLength: 1024,
}

type AgentConfig struct {
	Temperature     float64
	MaxTokens       int64
	TopP            float64
	OpenAIExtraInfo map[string]interface{}

	Debug            bool
	MaxHistoryLength int64
}
