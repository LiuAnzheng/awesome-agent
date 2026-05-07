package agents

import (
	"awesome-agent/core"
	"awesome-agent/tools"
	"errors"
	"fmt"
	"log"
	"os"
)

var logger = log.New(os.Stderr, "[agents] ", log.LstdFlags|log.Lshortfile)

type ReActAgent struct {
	core.BaseAgent
	ToolRegistry *tools.ToolRegistry
	Executor     *tools.ToolExecutor
	MaxSteps     int64
}

const reactSystemPrompt = `你是一个具备推理和行动能力的AI助手。请通过"思考→行动→观察"的循环来解决问题。

## 工作流程
1. 先输出你的思考分析
2. 如果需要获取信息，调用工具函数
3. 根据工具返回的结果继续思考
4. 当有足够信息时，直接给出最终答案

## 规则
- 每次回应先从思考开始
- 需要信息时必须调用工具，不要凭空猜测
- 确信可以完整回答时直接输出答案，不要再调用工具`

func (ra *ReActAgent) Run(inputText string) (string, error) {
	if ra.MaxSteps <= 0 {
		return "", errors.New("MaxSteps must be positive")
	}

	userMsg := core.Message{Role: "user", Content: inputText}
	ra.AddMessage(userMsg)

	messages := ra.buildMessages()
	schemas := ra.toolSchemas()

	for step := int64(0); step < ra.MaxSteps; step++ {
		resp, finishReason, err := ra.LLM.ChatComplete(messages, &ra.Config, schemas)
		if err != nil {
			return "", errors.New("LLM error: " + err.Error())
		}
		if finishReason == core.ContentFilter {
			return "", errors.New("LLM content filter exception")
		}

		if finishReason == core.ToolCalls && len(resp.ToolCalls) > 0 {
			if ra.Config.Debug {
				logger.Printf("step %d ---- %s, calling: %s", step+1,
					truncate(extractString(resp.Content), 80),
					resp.ToolCalls[0].Function.Name)
			}
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
		if ra.Config.Debug {
			logger.Printf("step %d — final answer: %s", step, truncate(finalAnswer, 80))
		}
		ra.AddMessage(resp)
		return finalAnswer, nil
	}

	return "unable to complete the task within the maximum steps", nil
}

func (ra *ReActAgent) buildMessages() []core.Message {
	messages := make([]core.Message, 0, len(ra.History())+1)
	if ra.SystemPrompt == "" {
		ra.SystemPrompt = reactSystemPrompt
	}
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func NewReActAgent(name string,
	llm *core.AwesomeLLMClient,
	config core.Config,
	toolRegistry *tools.ToolRegistry,
	maxSteps int64,
	systemPrompt string) *ReActAgent {

	if systemPrompt == "" {
		systemPrompt = reactSystemPrompt
	}
	ra := &ReActAgent{
		BaseAgent: core.BaseAgent{
			Name:         name,
			LLM:          *llm,
			Config:       config,
			SystemPrompt: systemPrompt,
		},
		ToolRegistry: toolRegistry,
		MaxSteps:     maxSteps,
	}
	if toolRegistry != nil {
		ra.Executor = tools.NewToolExecutor(toolRegistry)
	}
	return ra
}
