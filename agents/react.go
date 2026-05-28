package agents

import (
	"awesome-agent/core"
	"awesome-agent/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
)

type ReActAgent struct {
	core.BaseAgent
	ToolRegistry *tools.ToolRegistry
	Executor     *tools.ToolExecutor
	MaxSteps     int64
	SessionID    string
}

const DefaultReActSystemPrompt = `
You are an AI assistant equipped with reasoning and action capabilities.
Please solve problems through the cycle of "thinking → acting → observing".

## Content Rules
- When you call tools: your content MUST be your thinking process (analysis, plan, reasoning) — NOT the answer
- When information is sufficient to answer: your content MUST be the final answer — no more tool calls

## Workflow
1. Think about the problem and write your reasoning in content
2. If you need more information, call tools to gather it
3. Observe tool results and continue thinking
4. When confident you can fully answer, output the answer directly in content without calling tools

## Rules
- Always start with thinking in content
- When information is needed, tools must be invoked, do not guess without basis
- Never put the final answer in content while still calling tools — content is for thinking only when tools are used

## Memory Rules (MANDATORY when memory tool is available)
- If the memory tool exists in your tool list: you MUST call memory.search before reasoning, and MUST call memory.add after every response
- If no memory tool is available: ignore these rules
- You have no built-in memory across turns — the memory tool is your ONLY way to persist knowledge
`

func (ra *ReActAgent) Run(ctx context.Context, inputText string) (string, error) {
	if ra.MaxSteps <= 0 {
		return "", errors.New("MaxSteps must be positive")
	}

	userMsg := core.Message{Role: "user", Content: inputText}
	ra.AddMessage(userMsg)

	messages := ra.buildMessages()
	schemas := ra.toolSchemas()

	for step := int64(0); step < ra.MaxSteps; step++ {
		resp, finishReason, err := ra.LLM.ChatComplete(ctx, messages, schemas, nil)
		if err != nil {
			return "", errors.New("LLM error: " + err.Error())
		}
		if finishReason == core.ContentFilter {
			return "", errors.New("LLM content filter exception")
		}

		if finishReason == core.ToolCalls && len(resp.ToolCalls) > 0 {
			slog.Debug("agent step", "step", step, "content", resp.Content,
				"calling", toolCallNames(resp.ToolCalls))
			ra.injectSessionID(resp.ToolCalls)
			messages = append(messages, resp)
			toolResults, err := ra.Executor.Execute(resp.ToolCalls)
			if err != nil {
				return "", errors.New("executor error: " + err.Error())
			}
			for _, tr := range toolResults {
				messages = append(messages, tr)
			}
			continue
		}

		// 无工具调用 = 最终答案
		finalAnswer := extractString(resp.Content)
		slog.Debug("agent final answer", "content", finalAnswer)
		ra.AddMessage(resp)
		return finalAnswer, nil
	}

	return "unable to complete the task within the maximum steps", nil
}

func (ra *ReActAgent) buildMessages() []core.Message {
	messages := make([]core.Message, 0, len(ra.History())+1)
	messages = append(messages, core.Message{Role: "system", Content: ra.SystemPrompt})
	for _, msg := range ra.History() {
		messages = append(messages, msg)
	}
	return messages
}

func (ra *ReActAgent) toolSchemas() []map[string]interface{} {
	if ra.ToolRegistry == nil {
		return nil
	}
	return ra.ToolRegistry.ToOpenAISchemas()
}

func extractString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toolCallNames(calls []core.ToolCall) []string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Function.Name
	}
	return names
}

func (ra *ReActAgent) injectSessionID(toolCalls []core.ToolCall) {
	if ra.SessionID == "" {
		return
	}
	for i := range toolCalls {
		var args map[string]interface{}
		if toolCalls[i].Function.Arguments != "" {
			json.Unmarshal([]byte(toolCalls[i].Function.Arguments), &args)
		}
		if args == nil {
			args = make(map[string]interface{})
		}
		args["_session_id"] = ra.SessionID
		b, _ := json.Marshal(args)
		toolCalls[i].Function.Arguments = string(b)
	}
}

func NewReActAgent(name string,
	llm core.LLMInterface,
	config core.AppConfig,
	toolRegistry *tools.ToolRegistry,
	maxSteps int64,
	systemPrompt string) *ReActAgent {

	if systemPrompt == "" {
		systemPrompt = DefaultReActSystemPrompt
	}
	ra := &ReActAgent{
		BaseAgent: core.BaseAgent{
			Name:         name,
			LLM:          llm,
			Config:       config,
			SystemPrompt: systemPrompt,
		},
		ToolRegistry: toolRegistry,
		MaxSteps:     maxSteps,
	}
	if toolRegistry != nil {
		ra.Executor = tools.NewToolExecutor(toolRegistry)
	} else {
		slog.Warn("no tool registry found")
	}
	return ra
}
