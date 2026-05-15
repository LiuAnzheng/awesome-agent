package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"context"
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
	llm, err := core.NewAwesomeLLM(core.AppCfg.LLMConfig)
	if err != nil {
		panic(err)
	}

	mt, e := builtins.NewMemoryTool("demo-project",
		core.AppCfg.Memory,
		[]types.MemoryType{types.Working},
		nil,
		nil,
		nil,
		nil)
	if e != nil {
		panic(e)
	}

	// 创建工具注册器
	registry := tools.NewToolRegistry()

	// 注册工具
	registry.Register(mt)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	ctx := context.Background()

	// 运行
	_, err = agent.Run(ctx, "我的名字是Tom, 你是谁？")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(ctx, "我叫什么名字，你都可以干什么？")
	if err != nil {
		panic(err)
	}
}
