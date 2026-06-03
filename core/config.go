package core

import (
	"os"
	"time"
)

type LLMConfig struct {
	// required
	ModelID string
	APIKey  string
	BaseURL string
	// no required
	MaxTokens       int64
	Temperature     float64
	TopP            float64
	OpenAIExtraInfo map[string]string
}

func (l LLMConfig) ApplyEnv() LLMConfig {
	if l.ModelID == "" {
		l.ModelID = os.Getenv("MODEL_ID")
	}
	if l.APIKey == "" {
		l.APIKey = os.Getenv("MODEL_API_KEY")
	}
	if l.BaseURL == "" {
		l.BaseURL = os.Getenv("MODEL_BASE_URL")
	}
	return l
}

type DriverConfig struct {
	// no required
	Driver  string
	Options map[string]interface{}
}

type MemoryConfig struct {
	// no required
	Structured  DriverConfig
	Embedding   DriverConfig
	VectorStore DriverConfig
	Graph       DriverConfig
}

type RAGConfig struct {
	// no required
	MaxDocSize int64
	Collection string
}

type TerminalConfig struct {
	// no required
	Timeout         time.Duration
	MaxOutputSize   int64
	InitDir         string
	WorkSpace       string
	AllowCD         *bool
	AllowedCommands []string
}

type ContextConfig struct {
	// no required
	MaxTokens         int64
	ReserveRatio      float64
	MinRelevance      float64
	EnableCompression bool
	RecencyWeight     float64
	RelevanceWeight   float64
}
