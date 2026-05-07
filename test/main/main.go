package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"fmt"
)

func main() {
	testReact()
}

func testReact() {
	// 加载配置文件
	e := core.LoadConfig("app-config.yaml")
	if e != nil {
		panic(e)
	}

	// 创建LLM客户端
	llm, err := core.NewAwesomeLLMClient(core.AppCfg.LLMConfig)
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
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	// 运行
	ans, err := agent.Run("mac book最新的型号都有哪些？")
	if err != nil {
		panic(err)
	}
	fmt.Println(ans)
}
