package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"context"
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
	llm, err := core.NewAwesomeLLM(core.AppCfg.LLMConfig)
	if err != nil {
		panic(err)
	}

	// 创建工具注册器
	registry := tools.NewToolRegistry()

	// 创建rag tool
	tool, e := builtins.NewRAGTool(nil, nil, nil, core.AppCfg)
	if e != nil {
		panic(e)
	}
	ragTool, ok := tool.(*builtins.RAGTool)
	if !ok {
		panic("not ragTool")
	}

	// 摄入文档
	ingestResult, e := ragTool.Ingest(context.Background(), "./knowledge_base/demo_OpenAIAPI规范.md", "openai.md")
	if e != nil {
		panic(e)
	}
	fmt.Printf("文档摄入结果：%#v \n", ingestResult)

	// 注册工具
	registry.Register(ragTool)

	// 创建ReAct智能体
	agent := agents.NewReActAgent("react-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

	ctx := context.Background()

	// 运行
	_, err = agent.Run(ctx, "OpenAI API的认证方式是怎样的？")
	if err != nil {
		panic(err)
	}
}
