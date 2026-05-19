package agents

import (
	"awesome-agent/core"
	"awesome-agent/tools"
	"context"
	"errors"
	"fmt"
	"log/slog"
)

type ReActAgent struct {
	core.BaseAgent
	ToolRegistry *tools.ToolRegistry
	Executor     *tools.ToolExecutor
	MaxSteps     int64
}

const DefaultReActSystemPrompt = `
You are an AI assistant equipped with reasoning and action capabilities. 
Please solve problems through the cycle of "thinking → acting → observing".

## Workflow 1. First, output your thought analysis. 2. If information is needed, call the tool function. 
3. Continue to think based on the results returned by the tool. 
4. When sufficient information is available, directly provide the final answer

## Rules - Start every response with thinking 
- When information is needed, tools must be invoked, do not guess without basis 
- When confident in being able to answer completely, directly output the answer and do not invoke tools again
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
		resp, finishReason, err := ra.LLM.ChatComplete(ctx, messages, schemas)
		if err != nil {
			return "", errors.New("LLM error: " + err.Error())
		}
		if finishReason == core.ContentFilter {
			return "", errors.New("LLM content filter exception")
		}

		if finishReason == core.ToolCalls && len(resp.ToolCalls) > 0 {
			slog.Debug("agent step", "step", step, "content", resp.Content,
				"calling", toolCallNames(resp.ToolCalls))
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

func NewReActAgent(name string,
	llm core.LLMInterface,
	config core.AgentConfig,
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
