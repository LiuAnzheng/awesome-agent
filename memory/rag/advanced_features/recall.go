package advanced_features

import (
	"awesome-agent/core"
	"awesome-agent/memory/store"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

const rrfK = 60

func Recall(ctx context.Context, llm core.LLMInterface,
	query string,
	topK int,
	collectionName string,
	embed store.EmbeddingService,
	vectorStore store.VectorStore,
	enableMQE bool,
	enableHyDE bool) ([]store.VectorSearchResult, error) {
	if vectorStore == nil {
		return nil, errors.New("vectorStore is nil")
	}

	allQueries := []string{query}
	if enableMQE {
		mqeStrs, err := buildMQE(ctx, query, llm)
		if err != nil {
			return nil, err
		}
		allQueries = append(allQueries, mqeStrs...)
		slog.Debug("mqe query strings", "variants", mqeStrs)
	}

	if enableHyDE {
		hydeDoc, err := buildHyDE(ctx, query, llm)
		if err != nil {
			return nil, err
		}
		allQueries = append(allQueries, hydeDoc)
		slog.Debug("hyde doc strings", "variants", hydeDoc)
	}

	allResults, failed, err := parallelSearch(ctx, allQueries, topK*2, collectionName, embed, vectorStore)
	if err != nil {
		return nil, err
	}
	if failed > 0 {
		slog.Warn("parallelSearch: some queries failed", "failed", failed, "total", len(allQueries))
	}

	return rrfMerge(allResults, topK), nil
}

func buildMQE(ctx context.Context, query string, llm core.LLMInterface) ([]string, error) {
	if llm == nil {
		return nil, errors.New("llm is nil")
	}

	prompt := []core.Message{
		{
			Role: "system",
			Content: "You are a retrieval query expansion assistant. Generate diversified " +
				"queries that are semantically equivalent or complementary. " +
				"Use the original language (maintain Chinese as Chinese and English as English), " +
				"keep it concise, and avoid punctuation.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf("Original query: %s\nPlease provide 3 queries with different expressions, "+
				"one per line. Output only the queries, no numbering or prefixes.", query),
		},
	}

	complete, _, err := llm.ChatComplete(ctx, prompt, nil, nil)
	if err != nil {
		return nil, err
	}
	if complete.Content == nil {
		return nil, fmt.Errorf("mqe llm error: empty content")
	}

	str, ok := complete.Content.(string)
	if !ok {
		return nil, fmt.Errorf("mqe llm error, unexpected content type: %T", complete.Content)
	}

	lines := strings.Split(strings.TrimSpace(str), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "0123456789. )-、")
		line = strings.TrimSpace(line)
		if line != "" && line != query {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mqe llm returned no valid queries: %s", str)
	}
	return out, nil
}

func buildHyDE(ctx context.Context, query string, llm core.LLMInterface) (string, error) {
	if llm == nil {
		return "", errors.New("llm is nil")
	}

	prompt := []core.Message{
		{
			Role: "system",
			Content: "You are a document generator. Given a question, write a hypothetical " +
				"document passage that answers it. Write in a factual, encyclopedic style, " +
				"as if extracted from a textbook or technical manual. " +
				"2-5 sentences. Use the same language as the question. " +
				"Output only the passage, no preamble or meta-commentary.",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Question: %s\n\nHypothetical document passage:", query),
		},
	}

	complete, _, err := llm.ChatComplete(ctx, prompt, nil, nil)
	if err != nil {
		return "", err
	}
	if complete.Content == nil {
		return "", fmt.Errorf("hyde llm error: empty content")
	}

	str, ok := complete.Content.(string)
	if !ok {
		return "", fmt.Errorf("hyde llm error, unexpected content type: %T", complete.Content)
	}

	str = strings.TrimSpace(str)
	if str == "" {
		return "", fmt.Errorf("hyde llm returned empty string")
	}
	return str, nil
}

func parallelSearch(ctx context.Context, queries []string, topK int,
	collectionName string, embed store.EmbeddingService,
	vectorStore store.VectorStore) ([][]store.VectorSearchResult, int, error) {

	vectors, err := embed.EmbedBatch(ctx, queries)
	if err != nil {
		return nil, 0, fmt.Errorf("embed batch: %w", err)
	}
	if len(vectors) != len(queries) {
		return nil, 0, fmt.Errorf("embed batch: got %d vectors for %d queries", len(vectors), len(queries))
	}

	allResults := make([][]store.VectorSearchResult, len(vectors))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failedCount int

	for i, vec := range vectors {
		wg.Add(1)
		go func(idx int, vec []float64) {
			defer wg.Done()
			results, err := vectorStore.Search(ctx, store.VectorSearch{
				Collection: collectionName,
				Vector:     vec,
				Limit:      int64(topK),
				MinScore:   0.3,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failedCount++
				slog.Warn("parallelSearch: query failed", "queryIndex", idx, "error", err)
				return
			}
			allResults[idx] = results
		}(i, vec)
	}
	wg.Wait()

	if failedCount == len(vectors) {
		return nil, failedCount, errors.New("parallelSearch: all queries failed")
	}
	return allResults, failedCount, nil
}

func rrfMerge(allResults [][]store.VectorSearchResult, topK int) []store.VectorSearchResult {
	rrfScores := make(map[string]float64)
	bestVectorScores := make(map[string]float64)
	payloads := make(map[string]map[string]interface{})

	for _, results := range allResults {
		for rank, r := range results {
			rrfScores[r.ID] += 1.0 / float64(rrfK+rank+1)
			if r.Score > bestVectorScores[r.ID] {
				bestVectorScores[r.ID] = r.Score
			}
			if payloads[r.ID] == nil {
				payloads[r.ID] = r.Payload
			}
		}
	}

	merged := make([]store.VectorSearchResult, 0, len(rrfScores))
	for id := range rrfScores {
		merged = append(merged, store.VectorSearchResult{
			ID:      id,
			Score:   bestVectorScores[id],
			Payload: payloads[id],
		})
	}

	sort.Slice(merged, func(i, j int) bool {
		return rrfScores[merged[i].ID] > rrfScores[merged[j].ID]
	})

	if len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}
