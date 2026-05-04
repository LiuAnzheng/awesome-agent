package builtins

import (
	"awesome-agent/tools"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type WebSearchTool struct {
	tavilyApiKey string
	serpApiKey   string
	client       *http.Client
}

func (w *WebSearchTool) Name() string {
	return "WebSearch"
}

func (w *WebSearchTool) Description() string {
	return "搜索引擎工具，当用户需要实时信息或互联网信息时，应使用该工具"
}

func (w *WebSearchTool) Run(parameters map[string]interface{}) (string, error) {
	if w.tavilyApiKey == "" && w.serpApiKey == "" {
		return "", errors.New("no api key configured")
	}

	query, ok := parameters["query"].(string)
	if !ok || query == "" {
		return "", errors.New("query is required")
	}

	if w.tavilyApiKey != "" {
		result, err := w.tavilySearch(query)
		if err == nil {
			return result, nil
		}
		return "", fmt.Errorf("tavily search failed: %w", err)
	}

	if w.serpApiKey != "" {
		result, err := w.serpSearch(query)
		if err == nil {
			return result, nil
		}
		return "", fmt.Errorf("serpapi search failed: %w", err)
	}

	return "", errors.New("all search providers failed")
}

func (w *WebSearchTool) tavilySearch(query string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"api_key":      w.tavilyApiKey,
		"query":        query,
		"search_depth": "basic",
		"max_results":  5,
	})

	resp, err := w.client.Post(
		"https://api.tavily.com/search",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("tavily request failed: %w", err)
	}

	defer func(resp *http.Response) {
		e := resp.Body.Close()
		if e != nil {
			return
		}
	}(resp)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tavily returned status %d", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)
	var result tavilyResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("tavily parse failed: %w", err)
	}

	return result.format(), nil
}

func (w *WebSearchTool) serpSearch(query string) (string, error) {
	url := fmt.Sprintf(
		"https://serpapi.com/search?api_key=%s&q=%s&engine=google&num=5",
		w.serpApiKey, query,
	)

	resp, err := w.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("serpapi request failed: %w", err)
	}

	defer func(resp *http.Response) {
		e := resp.Body.Close()
		if e != nil {
			return
		}
	}(resp)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("serpapi returned status %d", resp.StatusCode)
	}

	data, _ := io.ReadAll(resp.Body)
	var result serpResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("serpapi parse failed: %w", err)
	}

	return result.format(), nil
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type tavilyResponse struct {
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

func (r *tavilyResponse) format() string {
	var sb strings.Builder
	if r.Answer != "" {
		sb.WriteString(r.Answer)
		sb.WriteString("\n\n")
	}
	for i, item := range r.Results {
		if i >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, item.Title, item.Content, item.URL))
	}
	return strings.TrimSpace(sb.String())
}

type serpResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

type serpResponse struct {
	OrganicResults []serpResult `json:"organic_results"`
}

func (r *serpResponse) format() string {
	var sb strings.Builder
	for i, item := range r.OrganicResults {
		if i >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, item.Title, item.Snippet, item.Link))
	}
	return strings.TrimSpace(sb.String())
}

func (w *WebSearchTool) Parameters() []tools.ToolParameter {
	return []tools.ToolParameter{
		{
			Name:        "query",
			Type:        tools.ParamString,
			Description: "搜索关键词",
			Required:    true,
		},
	}
}

func NewWebSearchTool(tavilyApiKey, serpApiKey string) (*WebSearchTool, error) {
	if tavilyApiKey == "" && serpApiKey == "" && os.Getenv("TAVILY_API_KEY") == "" && os.Getenv("SERPAPI_API_KEY") == "" {
		return nil, errors.New(`at least one api key is required
								1. Tavily API: os env TAVILY_API_KEY
								url: https://tavily.com/
								2. SerpAPI: os env SERPAPI_API_KEY
								url: https://serpapi.com/`)
	}
	if tavilyApiKey == "" {
		tavilyApiKey = os.Getenv("TAVILY_API_KEY")
	}
	if serpApiKey == "" {
		serpApiKey = os.Getenv("SERPAPI_API_KEY")
	}
	return &WebSearchTool{
		tavilyApiKey: tavilyApiKey,
		serpApiKey:   serpApiKey,
		client:       &http.Client{Timeout: 5 * time.Second},
	}, nil
}
