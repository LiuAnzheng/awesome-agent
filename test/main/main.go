package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"context"
	"log/slog"
)

func main() {
	testReact()
}

func testReact() {
	// 日志级别
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// 加载配置文件
	e := core.LoadConfig("app-config.yaml")
	if e != nil {
		panic(e)
	}

	// 创建LLM客户端
	llm, err := core.NewAwesomeLLM(core.AppCfg.LLMConfig, core.AppCfg.AgentConfig)
	if err != nil {
		panic(err)
	}

	// 创建工具注册器
	registry := tools.NewToolRegistry()

	// 创建memory tool
	mt, e := builtins.NewMemoryTool(core.AppCfg, types.AvailableMemoryTypes,
		nil, nil, nil, nil)
	if e != nil {
		panic(e)
	}

	// 开启会话
	memoryTool := mt.(*builtins.MemoryTool)
	e = memoryTool.AddSession("1b4db7eb-4057-5ddf-91e0-36dec72071f5")
	if e != nil {
		panic(e)
	}

	// 注册工具
	registry.Register(mt)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	ctx := context.Background()

	// 运行
	_, err = agent.Run(ctx, "你好，我叫什么名字？")
	if err != nil {
		panic(err)
	}
}
