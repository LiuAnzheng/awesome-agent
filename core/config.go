package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type LLMConfig struct {
	ModelID         string            `mapstructure:"model_id"`
	Provider        string            `mapstructure:"provider"`
	APIKey          string            `mapstructure:"api_key"`
	BaseURL         string            `mapstructure:"base_url"`
	MaxTokens       int64             `mapstructure:"max_tokens"`
	Temperature     float64           `mapstructure:"temperature"`
	TopP            float64           `mapstructure:"top_p"`
	OpenAIExtraInfo map[string]string `mapstructure:"open_ai_extra_info"`
}

// DriverConfig 通用驱动配置：driver 声明实现，options 为驱动专用参数
type DriverConfig struct {
	Driver  string                 `mapstructure:"driver"`
	Options map[string]interface{} `mapstructure:"options"`
}

type MemoryConfig struct {
	Structured  DriverConfig `mapstructure:"structure"`
	Embedding   DriverConfig `mapstructure:"embedding"`
	VectorStore DriverConfig `mapstructure:"vector_store"`
	Graph       DriverConfig `mapstructure:"graph"`
}

type RAGConfig struct {
	MaxDocSize int64  `mapstructure:"max_doc_size"`
	Collection string `mapstructure:"collection"`
}

type ContextConfig struct {
	MaxTokens         int64   `mapstructure:"max_tokens"`
	ReserveRatio      float64 `mapstructure:"reserve_ratio"`
	MinRelevance      float64 `mapstructure:"min_relevance"`
	EnableCompression bool    `mapstructure:"enable_compression"`
	RecencyWeight     float64 `mapstructure:"recency_weight"`
	RelevanceWeight   float64 `mapstructure:"relevance_weight"`
}

type AppConfig struct {
	LLMConfig     LLMConfig     `mapstructure:"llm"`
	Memory        MemoryConfig  `mapstructure:"memory"`
	RAGConfig     RAGConfig     `mapstructure:"rag"`
	ContextConfig ContextConfig `mapstructure:"context"`
}

var AppCfg = AppConfig{
	LLMConfig: LLMConfig{
		ModelID:         "gpt-5.4",
		Provider:        "openai",
		APIKey:          "",
		BaseURL:         "https://api.openai.com/",
		MaxTokens:       102400,
		Temperature:     0.7,
		TopP:            1.0,
		OpenAIExtraInfo: make(map[string]string),
	},
	Memory: MemoryConfig{
		Structured: DriverConfig{
			Driver: "sqlite",
			Options: map[string]interface{}{
				"db_path": "./data/memory.db",
			},
		},
		Embedding: DriverConfig{
			Driver: "openai",
			Options: map[string]interface{}{
				"model_id":   "text-embedding-3-small",
				"api_key":    "",
				"base_url":   "https://api.openai.com/",
				"dimension":  1024,
				"batch_size": 32,
			},
		},
		VectorStore: DriverConfig{
			Driver: "qdrant",
			Options: map[string]interface{}{
				"host":    "127.0.0.1",
				"port":    6333,
				"api_key": "",
			},
		},
		Graph: DriverConfig{
			Driver: "neo4j",
			Options: map[string]interface{}{
				"url":      "http://127.0.0.1:7474",
				"db":       "neo4j",
				"username": "neo4j",
				"password": "neo4j",
			},
		},
	},
	RAGConfig: RAGConfig{
		MaxDocSize: 50 * 1024 * 1024,
		Collection: "rag",
	},
	ContextConfig: ContextConfig{
		MaxTokens:         102400,
		ReserveRatio:      0.2,
		MinRelevance:      0.1,
		EnableCompression: true,
		RecencyWeight:     0.3,
		RelevanceWeight:   0.7,
	},
}

func LoadConfig(path string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	expanded := os.ExpandEnv(string(file))
	v := viper.New()
	v.SetConfigType("yaml")

	// LLM
	v.SetDefault("awesome-agent.llm.model_id", AppCfg.LLMConfig.ModelID)
	v.SetDefault("awesome-agent.llm.provider", AppCfg.LLMConfig.Provider)
	v.SetDefault("awesome-agent.llm.api_key", AppCfg.LLMConfig.APIKey)
	v.SetDefault("awesome-agent.llm.base_url", AppCfg.LLMConfig.BaseURL)
	v.SetDefault("awesome-agent.llm.max_tokens", AppCfg.LLMConfig.MaxTokens)
	v.SetDefault("awesome-agent.llm.temperature", AppCfg.LLMConfig.Temperature)
	v.SetDefault("awesome-agent.llm.top_p", AppCfg.LLMConfig.TopP)
	v.SetDefault("awesome-agent.llm.open_ai_extra_info", AppCfg.LLMConfig.OpenAIExtraInfo)

	// Memory
	v.SetDefault("awesome-agent.memory.structure.driver", AppCfg.Memory.Structured.Driver)
	v.SetDefault("awesome-agent.memory.structure.options", AppCfg.Memory.Structured.Options)

	v.SetDefault("awesome-agent.memory.embedding.driver", AppCfg.Memory.Embedding.Driver)
	v.SetDefault("awesome-agent.memory.embedding.options", AppCfg.Memory.Embedding.Options)

	v.SetDefault("awesome-agent.memory.vector_store.driver", AppCfg.Memory.VectorStore.Driver)
	v.SetDefault("awesome-agent.memory.vector_store.options", AppCfg.Memory.VectorStore.Options)

	v.SetDefault("awesome-agent.memory.graph.driver", AppCfg.Memory.Graph.Driver)
	v.SetDefault("awesome-agent.memory.graph.options", AppCfg.Memory.Graph.Options)

	// RAG
	v.SetDefault("awesome-agent.rag.max_doc_size", AppCfg.RAGConfig.MaxDocSize)
	v.SetDefault("awesome-agent.rag.collection", AppCfg.RAGConfig.Collection)

	// Context
	v.SetDefault("awesome-agent.context.max_tokens", AppCfg.ContextConfig.MaxTokens)
	v.SetDefault("awesome-agent.context.reserve_ratio", AppCfg.ContextConfig.ReserveRatio)
	v.SetDefault("awesome-agent.context.min_relevance", AppCfg.ContextConfig.MinRelevance)
	v.SetDefault("awesome-agent.context.enable_compression", AppCfg.ContextConfig.EnableCompression)
	v.SetDefault("awesome-agent.context.recency_weight", AppCfg.ContextConfig.RecencyWeight)
	v.SetDefault("awesome-agent.context.relevance_weight", AppCfg.ContextConfig.RelevanceWeight)

	err = v.ReadConfig(strings.NewReader(expanded))
	if err != nil {
		return err
	}

	sub := v.Sub("awesome-agent")
	if sub == nil {
		return fmt.Errorf("config not found in %s", path)
	}

	if err := sub.Unmarshal(&AppCfg); err != nil {
		return err
	}
	return validateContextConfig(AppCfg.ContextConfig, AppCfg.LLMConfig)
}

func validateContextConfig(c ContextConfig, l LLMConfig) error {
	if c.MaxTokens <= 0 {
		return fmt.Errorf("context.max_tokens must be positive, got: %v", c.MaxTokens)
	}
	if c.ReserveRatio < 0.0 || c.ReserveRatio > 1.0 {
		return fmt.Errorf("reserve_ratio must be in [0, 1], got: %v", c.ReserveRatio)
	}
	if c.MinRelevance < 0.0 || c.MinRelevance > 1.0 {
		return fmt.Errorf("min_relevance must be in [0, 1], got: %v", c.MinRelevance)
	}
	if c.RecencyWeight < 0.0 || c.RecencyWeight > 1.0 {
		return fmt.Errorf("recency_weight must be in [0, 1], got: %v", c.RecencyWeight)
	}
	if c.RelevanceWeight < 0.0 || c.RelevanceWeight > 1.0 {
		return fmt.Errorf("relevance_weight must be in [0, 1], got: %v", c.RelevanceWeight)
	}
	sum := c.RecencyWeight + c.RelevanceWeight
	if sum < 0.999999 || sum > 1.000001 {
		return fmt.Errorf("recency_weight + relevance_weight must equal 1.0, got sum=%v (recency=%v, relevance=%v)",
			sum, c.RecencyWeight, c.RelevanceWeight)
	}
	return nil
}
