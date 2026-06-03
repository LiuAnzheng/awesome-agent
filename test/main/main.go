package main

import (
	"context"
	"log/slog"

	"github.com/LiuAnzheng/memoria/agents"
	"github.com/LiuAnzheng/memoria/core"
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

	// terminal tool
	tool, err := builtins.NewTerminalTool(core.TerminalConfig{})
	if err != nil {
		panic(err)
	}

	registry.Register(tool)

	// 创建agent
	agent, _ := agents.NewReActAgent("demo", llm, core.ContextConfig{}, registry, 1024, "", sessionID)
	_, err = agent.Run(context.Background(), "你先了解以下当前代码库")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(context.Background(), "1~10分，为该项目打分")
	if err != nil {
		panic(err)
	}
}
