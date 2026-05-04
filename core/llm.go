package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (c *openAIHTTPClient) chatComplete(messages []Message, config *Config, tools []map[string]interface{}) (Message, error) {
	if messages == nil || len(messages) == 0 {
		return Message{}, errors.New("no messages")
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

	if len(tools) > 0 {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = "auto"
	}

	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("request failed: %w", err)
	}

	defer func(closer io.Closer) {
		_ = closer.Close()
	}(resp.Body)

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return Message{}, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return Message{}, fmt.Errorf("unmarshal failed: %w", err)
	}
	if len(result.Choices) == 0 {
		return Message{}, errors.New("no choices returned")
	}
	return result.Choices[0].Message, nil
}

func (c *openAIHTTPClient) chatStream(ctx context.Context, messages []Message, config *Config, tools []map[string]interface{}) <-chan StreamChunk {
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

		if len(tools) > 0 {
			reqBody["tools"] = tools
			reqBody["tool_choice"] = "auto"
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
			data, _ := io.ReadAll(resp.Body)
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
			if !c.sendToChan(ctx, ch, StreamChunk{
				Delta:        raw.Choices[0].Delta,
				FinishReason: raw.Choices[0].FinishReason,
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

type AwesomeLLMClient struct {
	httpClient openAIHTTPClient

	ModelID  string
	Provider string
	APIKey   string
	BaseURL  string
}

type LLMConfig struct {
	ModelID  string
	Provider string
	APIKey   string
	BaseURL  string
}

func (llmClient *AwesomeLLMClient) ChatComplete(messages []Message, config *Config, tools []map[string]interface{}) (Message, error) {
	return llmClient.httpClient.chatComplete(messages, config, tools)
}

func (llmClient *AwesomeLLMClient) ChatStream(ctx context.Context, messages []Message, config *Config, tools []map[string]interface{}) <-chan StreamChunk {
	return llmClient.httpClient.chatStream(ctx, messages, config, tools)
}

type StreamChunk struct {
	Delta        Message
	FinishReason string
	Err          error
}

func NewAwesomeLLMClient(config *LLMConfig) (*AwesomeLLMClient, error) {
	// TODO 模型供应商智能检测
	llmClient := &AwesomeLLMClient{}
	if config.ModelID != "" {
		llmClient.ModelID = config.ModelID
	} else {
		llmClient.ModelID = ""
	}

	if config.Provider != "" {
		llmClient.Provider = config.Provider
	} else {
		llmClient.Provider = ""
	}

	if config.APIKey != "" {
		llmClient.APIKey = config.APIKey
	} else {
		return nil, errors.New("API key is required")
	}

	if config.BaseURL != "" {
		llmClient.BaseURL = config.BaseURL
	} else {
		return nil, errors.New("base URL is required")
	}

	llmClient.httpClient = *newOpenAIHTTPClient(llmClient.BaseURL, llmClient.APIKey, llmClient.ModelID)
	return llmClient, nil
}
