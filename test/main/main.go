package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
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

	// 创建 rag tool
	rt, e := builtins.NewRAGTool(nil, nil, nil,
		core.AppCfg, true, true)
	if e != nil {
		panic(e)
	}

	// 摄入文档
	ragTool := rt.(*builtins.RAGTool)
	e = ragTool.Ingest(context.Background(), "./knowledge_base/demo_OpenAIAPI规范.md", "openai.md")
	if e != nil {
		panic(e)
	}

	// 注册工具
	registry.Register(rt)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	ctx := context.Background()

	// 运行
	_, err = agent.Run(ctx, "openai api的鉴权规范应该是怎样的？")
	if err != nil {
		panic(err)
	}
}
