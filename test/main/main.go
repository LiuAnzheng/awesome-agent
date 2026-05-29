package main

import (
	"awesome-agent/agents"
	"awesome-agent/core"
	"awesome-agent/memory/rag/ingestion/chunker"
	"awesome-agent/memory/types"
	"awesome-agent/tools"
	"awesome-agent/tools/builtins"
	"context"
)

var sessionID string = "1b4db7eb-4057-5ddf-91e0-36dec72071f5"

func main() {
	err := core.LoadConfig("./app-config.yaml")
	if err != nil {
		panic(err)
	}

	llm, err := core.NewLLM(core.AppCfg.LLMConfig)
	if err != nil {
		panic(err)
	}

	registry := tools.NewToolRegistry()

	mt, err := builtins.NewMemoryTool(core.AppCfg, types.AvailableMemoryTypes,
		nil, nil, nil, nil)
	memoryTool := mt.(*builtins.MemoryTool)
	err = memoryTool.AddSession(sessionID)
	if err != nil {
		panic(err)
	}

	rt, err := builtins.NewRAGTool(nil, nil, nil, core.AppCfg, chunker.Semantic,
		true, true)
	if err != nil {
		panic(err)
	}

	registry.Register(mt)
	registry.Register(rt)

	agent := agents.NewReActAgent("demo", llm, core.AppCfg, registry, 1024, "", sessionID)
	_, err = agent.Run(context.Background(), "")
	if err != nil {
		panic(err)
	}

	_, err = agent.Run(context.Background(), "")
	if err != nil {
		panic(err)
	}
}
