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

type AppConfig struct {
	LLMConfig   LLMConfig   `mapstructure:"llm"`
	AgentConfig AgentConfig `mapstructure:"agent"`
	Debug       bool        `mapstructure:"debug"`
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

	// 设置默认值
	v.SetDefault("awesome-agent.llm.model_id", AppCfg.LLMConfig.ModelID)
	v.SetDefault("awesome-agent.llm.provider", AppCfg.LLMConfig.Provider)
	v.SetDefault("awesome-agent.llm.api_key", AppCfg.LLMConfig.APIKey)
	v.SetDefault("awesome-agent.llm.base_url", AppCfg.LLMConfig.BaseURL)
	v.SetDefault("awesome-agent.agent.temperature", AppCfg.AgentConfig.Temperature)
	v.SetDefault("awesome-agent.agent.max_tokens", AppCfg.AgentConfig.MaxTokens)
	v.SetDefault("awesome-agent.agent.top_p", AppCfg.AgentConfig.TopP)
	v.SetDefault("awesome-agent.agent.open_ai_extra_info", AppCfg.AgentConfig.OpenAIExtraInfo)
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
