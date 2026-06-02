package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/LiuAnzheng/memoria/agents"
	"github.com/LiuAnzheng/memoria/core"
	"github.com/LiuAnzheng/memoria/memory/rag/ingestion/chunker"
	"github.com/LiuAnzheng/memoria/memory/types"
	"github.com/LiuAnzheng/memoria/tools"
	"github.com/LiuAnzheng/memoria/tools/builtins"
)

var sessionID string = "1b4db7eb-4057-5ddf-91e0-36dec72071f5"

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// 实例化LLM客户端
	llm, err := core.NewLLM(core.LLMConfig{
		ModelID: "deepseek-v4-pro",
	}.ApplyEnv())
	if err != nil {
		panic(err)
	}

	// 工具注册器
	registry := tools.NewToolRegistry()

	// 存储后端配置:
	// working记忆无依赖
	// episodic记忆依赖Structured和Embedding和VectorStore
	// semantic记忆依赖Embedding和VectorStore和Graph, 只有开启episodic后semantic才会生效
	memoryConfig := core.MemoryConfig{
		Structured: core.DriverConfig{Driver: "sqlite", Options: map[string]any{"db_path": "./data/memory.db"}},
		Embedding: core.DriverConfig{Driver: "openai",
			Options: map[string]any{"model_id": "text-embedding-v4", "api_key": os.Getenv("EMBEDDING_API_KEY"),
				"base_url": os.Getenv("EMBEDDING_BASE_URL"), "dimension": 1024, "batch_size": 10}},
		VectorStore: core.DriverConfig{Driver: "qdrant", Options: map[string]any{"host": "127.0.0.1", "port": 6333}},
		Graph: core.DriverConfig{Driver: "neo4j", Options: map[string]any{"url": "http://192.168.187.100:7474",
			"db": "neo4j", "username": "neo4j", "password": "123456789"}},
	}
	// memory tool
	mt, err := builtins.NewMemoryTool(memoryConfig, llm, types.AvailableMemoryTypes,
		nil, nil, nil, nil)
	if err != nil {
		panic(err)
	}

	// rag tool
	rt, err := builtins.NewRAGTool(nil, nil, nil,
		core.RAGConfig{
			MaxDocSize: 50 * 1024 * 1024,
			Collection: "rag",
		},
		memoryConfig, chunker.Semantic,
		true, true, llm)
	if err != nil {
		panic(err)
	}

	// note tool
	nt := builtins.NewNoteTool("./data/notes")

	// 摄入文档
	ragTool := rt.(*builtins.RAGTool)
	err = ragTool.Ingest(context.Background(), "./knowledge_base/people.txt", "test.txt")
	if err != nil {
		panic(err)
	}

	// 注册工具
	registry.Register(mt)
	registry.Register(rt)
	registry.Register(nt)

	// 创建agent
	agent, _ := agents.NewReActAgent("demo", llm, core.ContextConfig{}, registry, 1024, "", sessionID)
	_, err = agent.Run(context.Background(), "技术研发中心里的高级工程师有哪几位？")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(context.Background(), "如果想要对接软件功能测试相关工作，应该联系哪个部门的哪位负责人？")
	if err != nil {
		panic(err)
	}
}
