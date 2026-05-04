package core

var DefaultConfig = Config{
	Temperature:      0.7,
	MaxTokens:        1024,
	TopP:             1.0,
	OpenAIExtraInfo:  nil,
	Debug:            true,
	LogLevel:         "debug",
	MaxHistoryLength: 1024,
}

type Config struct {
	Temperature     float64
	MaxTokens       int64
	TopP            float64
	OpenAIExtraInfo map[string]interface{}

	Debug            bool
	LogLevel         string
	MaxHistoryLength int64
}
