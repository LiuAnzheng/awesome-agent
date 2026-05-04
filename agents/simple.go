package agents

import (
	"awesome-agent/core"
	"awesome-agent/tools"
	"errors"
	"log"
	"os"
)

var logger = log.New(os.Stderr, "[agents] ", log.LstdFlags|log.Lshortfile)

type SimpleAgent struct {
	core.BaseAgent
	ToolRegistry      *tools.ToolRegistry
	Executor          *tools.ToolExecutor
	EnableToolCalling bool
}

func (sa *SimpleAgent) Run(inputText string, maxToolIterations int64) (string, error) {
	if sa.EnableToolCalling && sa.ToolRegistry == nil {
		return "", errors.New("tool calling enabled but ToolRegistry is nil")
	}

	userMsg := core.Message{Role: "user", Content: inputText}
	sa.AddMessage(userMsg)

	messages := sa.buildMessages()
	schemas := sa.toolSchemas()

	var finalAnswer string
	for i := int64(0); i <= maxToolIterations; i++ {
		resp, err := sa.LLM.ChatComplete(messages, &sa.Config, schemas)
		if err != nil {
			return "", errors.New("LLM error: " + err.Error())
		}

		if len(resp.ToolCalls) == 0 {
			finalAnswer = sa.extractContent(resp)
			sa.AddMessage(resp)
			return finalAnswer, nil
		}

		// 工具调用轮次
		messages = append(messages, resp)
		if sa.Config.Debug {
			logger.Printf("calling tool: %s", resp.ToolCalls[0].Function.Name)
		}
		toolResults, err := sa.Executor.Execute(resp.ToolCalls)
		if err != nil {
			return "", errors.New("executor error: " + err.Error())
		}
		for _, tr := range toolResults {
			messages = append(messages, tr)
		}
	}

	// 达到最大迭代次数，最后一次不带工具调用要求模型给出答案
	resp, err := sa.LLM.ChatComplete(messages, &sa.Config, nil)
	if err != nil {
		return "", errors.New("LLM error: " + err.Error())
	}
	sa.AddMessage(resp)
	return sa.extractContent(resp), nil
}

func (sa *SimpleAgent) buildMessages() []core.Message {
	messages := make([]core.Message, 0, len(sa.History())+1)
	if sa.SystemPrompt != "" {
		messages = append(messages, core.Message{Role: "system", Content: sa.SystemPrompt})
	}
	for _, msg := range sa.History() {
		messages = append(messages, msg)
	}
	return messages
}

func (sa *SimpleAgent) toolSchemas() []map[string]interface{} {
	if !sa.EnableToolCalling || sa.ToolRegistry == nil {
		return nil
	}
	return sa.ToolRegistry.ToOpenAISchemas()
}

func (sa *SimpleAgent) extractContent(msg core.Message) string {
	if s, ok := msg.Content.(string); ok {
		return s
	}
	return ""
}

func NewSimpleAgent(name string,
	llm *core.AwesomeLLMClient,
	systemPrompt string,
	config core.Config,
	toolRegistry *tools.ToolRegistry,
	enableToolCalling bool) *SimpleAgent {

	sa := &SimpleAgent{
		BaseAgent: core.BaseAgent{
			Name:         name,
			LLM:          *llm,
			SystemPrompt: systemPrompt,
			Config:       config,
		},
		ToolRegistry:      toolRegistry,
		EnableToolCalling: enableToolCalling,
	}
	if toolRegistry != nil {
		sa.Executor = tools.NewToolExecutor(toolRegistry)
	}
	return sa
}
