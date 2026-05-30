package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/memory/rag/ingestion/chunker"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"context"
	"log/slog"
)

var sessionID string = "1b4db7eb-4057-5ddf-91e0-36dec72071f5"

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// 加载配置文件
	err := core.LoadConfig("./app-config.yaml")
	if err != nil {
		panic(err)
	}

	// 实例化LLM客户端
	llm, err := core.NewLLM(core.AppCfg.LLMConfig)
	if err != nil {
		panic(err)
	}

	// 工具注册器
	registry := tools.NewToolRegistry()

	// memory tool
	mt, err := builtins.NewMemoryTool(core.AppCfg, types.AvailableMemoryTypes,
		nil, nil, nil, nil)
	if err != nil {
		panic(err)
	}

	// rag tool
	rt, err := builtins.NewRAGTool(nil, nil, nil, core.AppCfg, chunker.Semantic,
		true, true)
	if err != nil {
		panic(err)
	}

	// 摄入文档
	ragTool := rt.(*builtins.RAGTool)
	err = ragTool.Ingest(context.Background(), "./knowledge_base/people.txt", "test.txt")
	if err != nil {
		panic(err)
	}

	// 注册工具
	registry.Register(mt)
	registry.Register(rt)

	// 创建agent
	agent, _ := agents.NewReActAgent("demo", llm, core.AppCfg, registry, 1024, "", sessionID)
	_, err = agent.Run(context.Background(), "技术研发中心里的高级工程师有哪几位？")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(context.Background(), "如果想要对接软件功能测试相关工作，应该联系哪个部门的哪位负责人？")
	if err != nil {
		panic(err)
	}
}
