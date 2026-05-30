package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"memoria/core"
	"memoria/ctx/gssc"
	"memoria/tools"
	"memoria/tools/builtins"
)

type ReActAgent struct {
	core.BaseAgent
	ToolRegistry *tools.ToolRegistry
	Executor     *tools.ToolExecutor
	MaxSteps     int64
	SessionID    string
	ctxBuilder   *gssc.ContextBuilder
}

const DefaultReActSystemPrompt = `You are an AI assistant. Follow the PERCEIVE → THINK → ACT loop strictly.

=== THE LOOP ===

Each turn, execute these steps in order:

  PERCEIVE
    - Read the user's message and all tool results carefully.
    - If the memory tool is registered, call memory.search NOW — before anything else.
      You have zero built-in memory. Skipping search = operating blind.

  THINK
    - Analyze what you know and what you still need.
    - Write your reasoning in content: what's the goal, what's missing,
      what tool to call and why. Be brief but concrete.

  ACT
    - If you need information: call tools. Content stays as thinking — NOT the answer.
    - If you can answer fully: output the final answer in content. No more tool calls.

=== RULES ===

1. Never guess facts. If you don't know, call a tool.
2. Content = thinking when tools are used. Content = answer only at the end.
3. If the memory tool is registered:
   a. BEFORE reasoning: call memory.search(query="keywords").
   b. AFTER your response: call memory.add() for everything you learned
      (facts, user info, preferences, decisions, errors, feedback).
      Skipping add = permanent amnesia — this rule is non-negotiable.
`

func (ra *ReActAgent) Run(ctx context.Context, inputText string) (string, error) {
	if ra.MaxSteps <= 0 {
		return "", errors.New("MaxSteps must be positive")
	}

	userMsg := core.Message{Role: "user", Content: inputText, Timestamp: core.Now()}
	ra.AddMessage(userMsg)

	for step := int64(0); step < ra.MaxSteps; step++ {
		messages := ra.buildMessages(inputText)
		schemas := ra.toolSchemas()
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
			ra.AddMessage(resp)
			toolResults, err := ra.Executor.Execute(resp.ToolCalls)
			if err != nil {
				return "", errors.New("executor error: " + err.Error())
			}
			for _, tr := range toolResults {
				ra.AddMessage(tr)
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

func (ra *ReActAgent) buildMessages(userQuery string) []core.Message {
	cbContent := ra.ctxBuilder.Build(
		userQuery,
		ra.History(),
		ra.SystemPrompt,
		nil,
	)
	messages := make([]core.Message, 0)
	messages = append(messages, core.Message{Role: "system", Content: cbContent})
	messages = append(messages, core.Message{Role: "user", Content: userQuery})
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
	systemPrompt string,
	sessionID string) (*ReActAgent, error) {

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
		SessionID:    sessionID,
	}
	if toolRegistry != nil {
		ra.Executor = tools.NewToolExecutor(toolRegistry)
	} else {
		slog.Warn("no tool registry found")
	}

	var mt *builtins.MemoryTool
	var rt *builtins.RAGTool
	if toolRegistry != nil {
		if t, ok := toolRegistry.Tool("memory_tool"); ok {
			mt, _ = t.(*builtins.MemoryTool)
		}
		if t, ok := toolRegistry.Tool("rag_tool"); ok {
			rt, _ = t.(*builtins.RAGTool)
		}
	}
	ra.ctxBuilder = gssc.NewContextBuilder(
		config.ContextConfig,
		mt,
		rt,
		ra.SessionID,
	)

	// 初始化memory tool session
	if toolRegistry != nil {
		if t, ok := toolRegistry.Tool("memory_tool"); ok {
			mt, _ = t.(*builtins.MemoryTool)
			if !mt.HasSessionID(ra.SessionID) {
				err := mt.AddSession(ra.SessionID)
				slog.Info("react agent set default memory session", "sessionID", ra.SessionID)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return ra, nil
}
