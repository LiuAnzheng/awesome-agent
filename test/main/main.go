package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/memory/rag/ingestion/chunker"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"context"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	testRag()
	//testMemory()
	//testLLM()
}

func testLLM() {
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

	// 读取图片
	bytes, e := os.ReadFile("./multimodal_data/test.jpg")
	if e != nil {
		panic(e)
	}

	// 构建content part
	var contentParts []core.ContentPart
	p1 := core.NewTextContentPart("这是什么？")
	contentParts = append(contentParts, p1)
	p2 := core.NewImageContentPart(&core.ImageURL{
		URL:    core.BuildBase64URL(bytes, core.JPEG),
		Detail: "auto",
	})
	contentParts = append(contentParts, p2)

	message, _, e := llm.ChatComplete(context.Background(), []core.Message{
		core.Message{
			Role:    "user",
			Content: contentParts,
		},
	}, nil, nil)
	if e != nil {
		panic(e)
	}

	fmt.Println(message)
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
		core.AppCfg, chunker.Semantic, false, false)
	if e != nil {
		panic(e)
	}

	// 摄入文档
	ragTool := rt.(*builtins.RAGTool)
	e = ragTool.Ingest(context.Background(), "knowledge_base/thoughts_on_the_altar_of_the_earth.txt",
		"thoughts_on_the_altar_of_the_earth.txt")
	if e != nil {
		panic(e)
	}

	// 注册工具
	registry.Register(rt)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	ctx := context.Background()

	// 运行
	question := `作者为什么说 "这园中不单是处处都有过我的车辙，有过我的车辙的地方也都有过母亲的脚印"？请结合文章内容谈谈你对这句话的理解。`
	_, err = agent.Run(ctx, question)
	if err != nil {
		panic(err)
	}
}
