package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"fmt"
	"os"
)

func main() {
	testReact()
}

func testReact() {
	// 创建LLM客户端
	llm, err := core.NewAwesomeLLMClient(
		&core.LLMConfig{
			ModelID:  "deepseek-v4-pro",
			Provider: "deepseek",
			APIKey:   os.Getenv("LLM_API_KEY"),
			BaseURL:  os.Getenv("LLM_BASE_URL"),
		},
	)
	if err != nil {
		panic(err)
	}

	// 创建搜索工具
	webSearch, err := builtins.NewWebSearchTool("", "")
	if err != nil {
		panic(err)
	}

	// 创建工具注册器
	registry := tools.NewToolRegistry()

	// 注册工具
	registry.Register(webSearch)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.DefaultConfig, registry, 1024)

	// 运行
	ans, err := agent.Run("mac book最新的型号都有哪些？")
	if err != nil {
		panic(err)
	}
	fmt.Println(ans)
}
