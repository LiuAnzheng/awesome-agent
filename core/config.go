package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type LLMConfig struct {
	ModelID  string `mapstructure:"model_id"`
	Provider string `mapstructure:"provider"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"`
}

type AgentConfig struct {
	Temperature     float64           `mapstructure:"temperature"`
	MaxTokens       int64             `mapstructure:"max_tokens"`
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

type AppConfig struct {
	LLMConfig   LLMConfig    `mapstructure:"llm"`
	AgentConfig AgentConfig  `mapstructure:"agent"`
	Memory      MemoryConfig `mapstructure:"memory"`
	Debug       bool         `mapstructure:"debug"`
}

var AppCfg = AppConfig{
	LLMConfig: LLMConfig{
		ModelID:  "gpt-5.4",
		Provider: "openai",
		APIKey:   "",
		BaseURL:  "https://api.openai.com/",
	},
	AgentConfig: AgentConfig{
		Temperature:     0.7,
		MaxTokens:       1024,
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
	Debug: true,
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

	// Agent
	v.SetDefault("awesome-agent.agent.temperature", AppCfg.AgentConfig.Temperature)
	v.SetDefault("awesome-agent.agent.max_tokens", AppCfg.AgentConfig.MaxTokens)
	v.SetDefault("awesome-agent.agent.top_p", AppCfg.AgentConfig.TopP)
	v.SetDefault("awesome-agent.agent.open_ai_extra_info", AppCfg.AgentConfig.OpenAIExtraInfo)

	// Memory
	v.SetDefault("awesome-agent.memory.structure.driver", AppCfg.Memory.Structured.Driver)
	v.SetDefault("awesome-agent.memory.structure.options", AppCfg.Memory.Structured.Options)

	v.SetDefault("awesome-agent.memory.embedding.driver", AppCfg.Memory.Embedding.Driver)
	v.SetDefault("awesome-agent.memory.embedding.options", AppCfg.Memory.Embedding.Options)

	v.SetDefault("awesome-agent.memory.vector_store.driver", AppCfg.Memory.VectorStore.Driver)
	v.SetDefault("awesome-agent.memory.vector_store.options", AppCfg.Memory.VectorStore.Options)

	v.SetDefault("awesome-agent.memory.graph.driver", AppCfg.Memory.Graph.Driver)
	v.SetDefault("awesome-agent.memory.graph.options", AppCfg.Memory.Graph.Options)

	// Debug
	v.SetDefault("awesome-agent.debug", AppCfg.Debug)

	err = v.ReadConfig(strings.NewReader(expanded))
	if err != nil {
		return err
	}

	sub := v.Sub("awesome-agent")
	if sub == nil {
		return fmt.Errorf("config not found in %s", path)
	}

	return sub.Unmarshal(&AppCfg)
}
