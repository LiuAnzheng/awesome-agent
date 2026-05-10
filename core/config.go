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

type StructuredConfig struct {
	DBPath string `mapstructure:"db_path"`
}

type EmbeddingConfig struct {
	ModelID   string `mapstructure:"model_id"`
	APIKey    string `mapstructure:"api_key"`
	BaseURL   string `mapstructure:"base_url"`
	Dimension uint64 `mapstructure:"dimension"`
	BatchSize int    `mapstructure:"batch_size"`
}

type VectorStoreConfig struct {
	Host   string `mapstructure:"host"`
	Port   int    `mapstructure:"port"`
	APIKey string `mapstructure:"api_key"`
}

type GraphConfig struct {
	URL      string `mapstructure:"url"`
	DBName   string `mapstructure:"db"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type MemoryConfig struct {
	Structured  StructuredConfig  `mapstructure:"structure"`
	Embedding   EmbeddingConfig   `mapstructure:"embedding"`
	VectorStore VectorStoreConfig `mapstructure:"vector_store"`
	Graph       GraphConfig       `mapstructure:"graph"`
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
		Structured: StructuredConfig{
			DBPath: "./data/memory.db",
		},
		Embedding: EmbeddingConfig{
			ModelID:   "text-embedding-3-small",
			APIKey:    "",
			BaseURL:   "https://api.openai.com/",
			Dimension: 1024,
			BatchSize: 32,
		},
		VectorStore: VectorStoreConfig{
			Host:   "127.0.0.1",
			Port:   6333,
			APIKey: "",
		},
		Graph: GraphConfig{
			URL:      "http://127.0.0.1:7474",
			DBName:   "neo4j",
			Username: "neo4j",
			Password: "neo4j",
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

	// Memory.Structured
	v.SetDefault("awesome-agent.memory.structure.db_path", AppCfg.Memory.Structured.DBPath)

	// Memory.Embedding
	v.SetDefault("awesome-agent.memory.embedding.model_id", AppCfg.Memory.Embedding.ModelID)
	v.SetDefault("awesome-agent.memory.embedding.api_key", AppCfg.Memory.Embedding.APIKey)
	v.SetDefault("awesome-agent.memory.embedding.base_url", AppCfg.Memory.Embedding.BaseURL)
	v.SetDefault("awesome-agent.memory.embedding.dimension", AppCfg.Memory.Embedding.Dimension)
	v.SetDefault("awesome-agent.memory.embedding.batch_size", AppCfg.Memory.Embedding.BatchSize)

	// Memory.VectorStore
	v.SetDefault("awesome-agent.memory.vector_store.host", AppCfg.Memory.VectorStore.Host)
	v.SetDefault("awesome-agent.memory.vector_store.port", AppCfg.Memory.VectorStore.Port)
	v.SetDefault("awesome-agent.memory.vector_store.api_key", AppCfg.Memory.VectorStore.APIKey)

	// Memory.Graph
	v.SetDefault("awesome-agent.memory.graph.url", AppCfg.Memory.Graph.URL)
	v.SetDefault("awesome-agent.memory.graph.db", AppCfg.Memory.Graph.DBName)
	v.SetDefault("awesome-agent.memory.graph.username", AppCfg.Memory.Graph.Username)
	v.SetDefault("awesome-agent.memory.graph.password", AppCfg.Memory.Graph.Password)

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
