package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type openAIHTTPClient struct {
	baseURL string
	apiKey  string
	model   string
}

func newOpenAIHTTPClient(baseURL, apiKey, model string) *openAIHTTPClient {
	return &openAIHTTPClient{baseURL: baseURL, apiKey: apiKey, model: model}
}

func (c *openAIHTTPClient) chatComplete(ctx context.Context, messages []Message, config LLMConfig, tools []map[string]interface{}, toolChoice interface{}) (Message, FinishReasonType, error) {
	if messages == nil || len(messages) == 0 {
		return Message{}, "", errors.New("no messages")
	}
	reqBody := map[string]interface{}{
		"model":       c.model,
		"messages":    messages,
		"temperature": config.Temperature,
		"max_tokens":  config.MaxTokens,
		"top_p":       config.TopP,
		"thinking":    map[string]string{"type": "disabled"},
	}

	if config.OpenAIExtraInfo != nil && len(config.OpenAIExtraInfo) > 0 {
		for k, v := range config.OpenAIExtraInfo {
			reqBody[k] = v
		}
	}

	if toolChoice == nil {
		toolChoice = "auto"
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = toolChoice
	}

	slog.Debug("llm request", "body", fmt.Sprintf("%#v", reqBody))

	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Message{}, "", fmt.Errorf("request failed: %w", err)
	}

	defer func(closer io.Closer) {
		_ = closer.Close()
	}(resp.Body)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return Message{}, "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return Message{}, "", fmt.Errorf("unmarshal failed: %w", err)
	}
	if len(result.Choices) == 0 {
		return Message{}, "", errors.New("no choices returned")
	}
	finishReason := ParseFinishReason(result.Choices[0].FinishReason)
	result.Choices[0].Message.Timestamp = Now()
	return result.Choices[0].Message, finishReason, nil
}

func (c *openAIHTTPClient) chatStream(ctx context.Context, messages []Message, config LLMConfig, tools []map[string]interface{}, toolChoice interface{}) <-chan StreamChunk {
	ch := make(chan StreamChunk)
	if messages == nil || len(messages) == 0 {
		c.sendToChan(ctx, ch, StreamChunk{Err: fmt.Errorf("no messages")})
		return ch
	}
	go func() {
		defer close(ch)

		select {
		case <-ctx.Done():
			return
		default:
		}

		reqBody := map[string]interface{}{
			"model":       c.model,
			"messages":    messages,
			"stream":      true,
			"temperature": config.Temperature,
			"max_tokens":  config.MaxTokens,
			"top_p":       config.TopP,
			"thinking":    map[string]string{"type": "disabled"},
		}

		if config.OpenAIExtraInfo != nil && len(config.OpenAIExtraInfo) > 0 {
			for k, v := range config.OpenAIExtraInfo {
				reqBody[k] = v
			}
		}

		if toolChoice == nil {
			toolChoice = "auto"
		}
		if len(tools) > 0 {
			reqBody["tools"] = tools
			reqBody["tool_choice"] = toolChoice
		}

		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			c.sendToChan(ctx, ch, StreamChunk{Err: fmt.Errorf("request failed: %w", err)})
			return
		}

		defer func(closer io.Closer) {
			_ = closer.Close()
		}(resp.Body)

		if resp.StatusCode >= 400 {
			data, e := io.ReadAll(resp.Body)
			if e != nil {
				c.sendToChan(ctx, ch, StreamChunk{Err: fmt.Errorf("failed to read response body: %w", e)})
			}
			c.sendToChan(ctx, ch, StreamChunk{Err: fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))})
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var raw struct {
				Choices []struct {
					Delta        Message `json:"delta"`
					FinishReason string  `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &raw); err != nil {
				continue
			}
			if len(raw.Choices) == 0 {
				continue
			}
			raw.Choices[0].Delta.Timestamp = Now()
			if !c.sendToChan(ctx, ch, StreamChunk{
				Delta:        raw.Choices[0].Delta,
				FinishReason: ParseFinishReason(raw.Choices[0].FinishReason),
			}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			c.sendToChan(ctx, ch, StreamChunk{Err: err})
		}
	}()
	return ch
}

func (c *openAIHTTPClient) sendToChan(ctx context.Context, ch chan<- StreamChunk, chunk StreamChunk) bool {
	select {
	case ch <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}

type LLMInterface interface {
	ChatComplete(ctx context.Context, messages []Message, tools []map[string]interface{}, toolChoice interface{}) (Message, FinishReasonType, error)

	ChatStream(ctx context.Context, messages []Message, tools []map[string]interface{}, toolChoice interface{}) <-chan StreamChunk

	Provider() string
}

type AwesomeLLM struct {
	httpClient openAIHTTPClient
	config     LLMConfig

	ModelID  string
	provider string
	APIKey   string
	BaseURL  string
}

func (llmClient *AwesomeLLM) Provider() string {
	return llmClient.provider
}

func (llmClient *AwesomeLLM) ChatComplete(ctx context.Context, messages []Message,
	tools []map[string]interface{}, tc interface{}) (Message, FinishReasonType, error) {
	return llmClient.httpClient.chatComplete(ctx, messages, llmClient.config, tools, tc)
}

func (llmClient *AwesomeLLM) ChatStream(ctx context.Context, messages []Message,
	tools []map[string]interface{}, tc interface{}) <-chan StreamChunk {
	return llmClient.httpClient.chatStream(ctx, messages, llmClient.config, tools, tc)
}

type StreamChunk struct {
	Delta        Message
	FinishReason FinishReasonType
	Err          error
}

func NewAwesomeLLM(llmConfig LLMConfig) (LLMInterface, error) {
	llmClient := &AwesomeLLM{}

	llmClient.provider = llmConfig.Provider
	llmClient.ModelID = llmConfig.ModelID
	llmClient.APIKey = llmConfig.APIKey
	llmClient.BaseURL = llmConfig.BaseURL
	llmClient.config = llmConfig

	llmClient.httpClient = *newOpenAIHTTPClient(llmClient.BaseURL, llmClient.APIKey, llmClient.ModelID)
	return llmClient, nil
}
