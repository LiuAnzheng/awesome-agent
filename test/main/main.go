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
	//testRag()
	testMemory()
}

func testMemory() {
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
	tool, e := builtins.NewMemoryTool(core.AppCfg, types.AvailableMemoryTypes,
		nil, nil, nil, nil)
	if e != nil {
		panic(e)
	}
	mt := tool.(*builtins.MemoryTool)
	// 开启会话
	e = mt.AddSession("4ebd0208-8328-5d69-8c44-ec50939c0967")
	if e != nil {
		panic(e)
	}

	// 注册工具
	registry.Register(mt)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	ctx := context.Background()

	// 运行
	_, err = agent.Run(ctx, "我的MySQL昨天发生了死锁问题，我怎么办？")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(ctx, "MySQL的死锁如何解决")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(ctx, "我的MySQL刚刚发生了死锁问题，我怎么办？")
	if err != nil {
		panic(err)
	}
}

func testRag() {
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
