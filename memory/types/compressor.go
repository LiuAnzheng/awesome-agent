package types

import (
	"awesome-agent/core"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Compressor interface {
	Summarize(ctx context.Context, items []MemoryItem) (*MemoryItem, error)

	SourceType() MemoryType

	TargetType() MemoryType
}

type Working2EpisodicCompressor struct {
	llm core.LLMInterface
}

func NewWorking2EpisodicCompressor(llm core.LLMInterface) Compressor {
	return &Working2EpisodicCompressor{
		llm: llm,
	}
}

func (w *Working2EpisodicCompressor) Summarize(ctx context.Context, items []MemoryItem) (*MemoryItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to compress")
	}

	prompt := w.buildPrompt(items)
	messages := []core.Message{
		{Role: "user", Content: prompt},
	}

	output, err := w.callLLM(ctx, messages)
	if err != nil {
		return nil, err
	}

	return w.buildEpisodicItem(items, output), nil
}

func (w *Working2EpisodicCompressor) buildPrompt(items []MemoryItem) string {
	sorted := make([]MemoryItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Importance > sorted[j].Importance
	})

	var itemList strings.Builder
	for i, it := range sorted {
		itemList.WriteString(fmt.Sprintf("[%d] importance=%.1f %s\n", i+1, it.Importance, it.Content))
	}

	return fmt.Sprintf(`You are a memory compressor. Call the compress_memory function to merge
the following %d working memory items from a single session into ONE episodic memory.
## Memory Items (sorted by importance, highest first)
%s
## Compression Rules
1. Extract the core thread: "user intent → key findings/actions → result/conclusion"
2. Filter out process noise: tool call logs, intermediate reasoning steps, duplicate observations
3. Preserve critical details: code paths, error messages, parameter values, concrete data
4. If multiple items describe different stages of the same event, merge them chronologically
5. Write "content" in 2-5 sentences — stay specific, avoid over-generalizing
6. Write "summary" as a single sentence with key nouns, for embedding-based retrieval
7. Set "importance" to (max importance among source items) × 0.9`, len(items), itemList.String())
}

func (w *Working2EpisodicCompressor) callLLM(ctx context.Context, messages []core.Message) (*compressOutput, error) {
	resp, finishReason, err := w.llm.ChatComplete(ctx, messages, w2eCompressFunctionSchema(), "required")
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}
	if finishReason != core.ToolCalls {
		return nil, fmt.Errorf("llm call: got %q, want %q", finishReason, core.ToolCalls)
	}
	if resp.ToolCalls == nil || len(resp.ToolCalls) == 0 {
		return nil, fmt.Errorf("llm call: no tool calls")
	}
	f := resp.ToolCalls[0].Function
	if f.Name != "compress_memory" {
		return nil, fmt.Errorf("llm call: got %q, want %q", f.Name, "compress_memory")
	}
	output := &compressOutput{}
	err = json.Unmarshal([]byte(f.Arguments), &output)
	if err != nil {
		return nil, fmt.Errorf("llm call: unmarshal: %w", err)
	}
	return output, nil
}

// buildEpisodicItem 用 LLM 输出构建 Episodic MemoryItem
func (w *Working2EpisodicCompressor) buildEpisodicItem(items []MemoryItem, output *compressOutput) *MemoryItem {
	maxImp := 0.0
	sourceIDs := make([]string, len(items))
	for i, it := range items {
		sourceIDs[i] = it.ID
		if it.Importance > maxImp {
			maxImp = it.Importance
		}
	}

	imp := output.Importance
	if imp <= 0 || imp > 1.0 {
		imp = maxImp * 0.9
	}

	return &MemoryItem{
		Content:        output.Content,
		Importance:     imp,
		SourceIDs:      sourceIDs,
		CompressedFrom: Working,
		Status:         "active",
		Metadata: map[string]string{
			"summary":    output.Summary,
			"event_type": string(output.EventType),
		},
	}
}

type compressOutput struct {
	Summary    string    `json:"summary"`
	Content    string    `json:"content"`
	Importance float64   `json:"importance"`
	EventType  EventType `json:"event_type"`
}

func w2eCompressFunctionSchema() []map[string]interface{} {
	return []map[string]interface{}{
		map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "compress_memory",
				"description": "Output the compressed episodic memory in structured format",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"summary": map[string]interface{}{
							"type":        "string",
							"description": "One-sentence summary with key nouns, used for embedding retrieval",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "2-5 sentence narrative of the complete event, preserving concrete details",
						},
						"importance": map[string]interface{}{
							"type":        "number",
							"description": "Score 0.0-1.0, derived from max source importance × 0.9",
						},
						"event_type": map[string]interface{}{
							"type":        "string",
							"description": "Event type",
							"enum":        []string{"observation", "thought", "action", "result"},
						},
					},
					"required": []string{"summary", "content", "importance", "event_type"},
				},
			},
		}}
}

func (w *Working2EpisodicCompressor) SourceType() MemoryType {
	return Working
}

func (w *Working2EpisodicCompressor) TargetType() MemoryType {
	return Episodic
}

type Episodic2SemanticCompressor struct{}

func (e *Episodic2SemanticCompressor) Summarize(ctx context.Context, items []MemoryItem) (*MemoryItem, error) {
	//TODO implement me
	panic("implement me")
}

func (e *Episodic2SemanticCompressor) SourceType() MemoryType {
	return Episodic
}

func (e *Episodic2SemanticCompressor) TargetType() MemoryType {
	return Semantic
}
