package impl

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"memoria/memory/store"
	"net/http"
	"time"
)

type QdrantStore struct {
	apiKey  string
	client  *http.Client
	baseURL string
}

func NewQdrantStore(options map[string]interface{}) *QdrantStore {
	host := "127.0.0.1"
	port := 6333
	apiKey := ""

	if v, ok := options["host"]; ok {
		if s, ok := v.(string); ok && s != "" {
			host = s
		}
	}
	if v, ok := options["port"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				port = n
			}
		case float64:
			if n > 0 {
				port = int(n)
			}
		}
	}
	if v, ok := options["api_key"]; ok {
		if s, ok := v.(string); ok {
			apiKey = s
		}
	}

	return &QdrantStore{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
	}
}

func (q *QdrantStore) Init(ctx context.Context, collection string, dimension uint64) error {
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, collection)

	resp, err := q.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body := qdrantCreateCollectionRequest{
		Vectors: qdrantVectorConfig{
			Size:     dimension,
			Distance: "Cosine",
		},
	}

	resp, err = q.doRequest(ctx, http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create collection failed (status %d): %s", resp.StatusCode, string(errBody))
	}

	return nil
}

func (q *QdrantStore) Upsert(ctx context.Context, collection string, point store.VectorPoint) error {
	return q.BatchUpsert(ctx, collection, []store.VectorPoint{point})
}

func (q *QdrantStore) BatchUpsert(ctx context.Context, collection string, points []store.VectorPoint) error {
	qdrantPoints := make([]qdrantPoint, 0, len(points))
	for _, p := range points {
		payload := map[string]interface{}{"_id": p.ID}
		for k, v := range p.Payload {
			payload[k] = v
		}

		qdrantPoints = append(qdrantPoints, qdrantPoint{
			ID:      toPointUUID(p.ID),
			Vector:  p.Vector,
			Payload: payload,
		})
	}

	url := fmt.Sprintf("%s/collections/%s/points", q.baseURL, collection)
	resp, err := q.doRequest(ctx, http.MethodPut, url, qdrantUpsertRequest{Points: qdrantPoints})
	if err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert failed (status %d): %s", resp.StatusCode, string(errBody))
	}

	return nil
}

func (q *QdrantStore) Search(ctx context.Context, search store.VectorSearch) ([]store.VectorSearchResult, error) {
	body := qdrantSearchRequest{
		Vector:         search.Vector,
		Limit:          search.Limit,
		ScoreThreshold: search.MinScore,
		WithPayload:    true,
	}

	if len(search.Filters) > 0 {
		filter, err := buildQdrantFilter(search.Filters)
		if err != nil {
			return nil, fmt.Errorf("build filter: %w", err)
		}
		body.Filter = &filter
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, search.Collection)
	resp, err := q.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result qdrantSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	results := make([]store.VectorSearchResult, 0, len(result.Result))
	for _, r := range result.Result {
		originalID := ""
		if r.Payload != nil {
			if id, ok := r.Payload["_id"]; ok {
				originalID = fmt.Sprintf("%v", id)
			}
		}
		if originalID == "" {
			originalID = r.ID
		}

		payload := make(map[string]interface{})
		for k, v := range r.Payload {
			if k != "_id" {
				payload[k] = v
			}
		}

		results = append(results, store.VectorSearchResult{
			ID:      originalID,
			Score:   r.Score,
			Payload: payload,
		})
	}

	return results, nil
}

func (q *QdrantStore) Delete(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	pointIDs := make([]string, len(ids))
	for i, id := range ids {
		pointIDs[i] = toPointUUID(id)
	}

	url := fmt.Sprintf("%s/collections/%s/points/delete", q.baseURL, collection)
	resp, err := q.doRequest(ctx, http.MethodPost, url, qdrantDeleteRequest{Points: pointIDs})
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed (status %d): %s", resp.StatusCode, string(errBody))
	}

	return nil
}

func (q *QdrantStore) Close() error {
	return nil
}

// qdrantNamespace 确保本项目的 UUID v5 不与其他系统冲突
var qdrantNamespace = func() []byte {
	h := sha1.Sum([]byte("memoria.qdrant.points"))
	return h[:16]
}()

// toPointUUID 将 string ID 转为 Qdrant 支持的 UUID point ID
// 零碰撞：数字 ID 直接使用，非数字 ID 用 SHA-1 确定性 UUID v5
func toPointUUID(id string) string {
	h := sha1.New()
	h.Write(qdrantNamespace)
	h.Write([]byte(id))
	hash := h.Sum(nil)

	hash[6] = (hash[6] & 0x0f) | 0x50
	hash[8] = (hash[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

func buildQdrantFilter(filters []store.VectorFilter) (qdrantFilter, error) {
	var must, mustNot []qdrantCondition

	for _, f := range filters {
		cond, err := toQdrantCondition(f)
		if err != nil {
			return qdrantFilter{}, err
		}

		if f.Operator == "!=" {
			mustNot = append(mustNot, cond)
		} else {
			must = append(must, cond)
		}
	}

	return qdrantFilter{Must: must, MustNot: mustNot}, nil
}

func toQdrantCondition(f store.VectorFilter) (qdrantCondition, error) {
	switch f.Operator {
	case "=", "==":
		return qdrantCondition{Key: f.Field, Match: &qdrantMatch{Value: f.Value}}, nil
	case "!=":
		return qdrantCondition{Key: f.Field, Match: &qdrantMatch{Value: f.Value}}, nil
	case ">":
		return qdrantCondition{Key: f.Field, Range: &qdrantRange{Gt: f.Value}}, nil
	case ">=":
		return qdrantCondition{Key: f.Field, Range: &qdrantRange{Gte: f.Value}}, nil
	case "<":
		return qdrantCondition{Key: f.Field, Range: &qdrantRange{Lt: f.Value}}, nil
	case "IN":
		return qdrantCondition{Key: f.Field, Match: &qdrantMatch{Any: f.Value}}, nil
	default:
		return qdrantCondition{}, fmt.Errorf("unsupported filter operator: %s", f.Operator)
	}
}

func (q *QdrantStore) doRequest(ctx context.Context, method string, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	return q.client.Do(req)
}

type qdrantCreateCollectionRequest struct {
	Vectors qdrantVectorConfig `json:"vectors"`
}

type qdrantVectorConfig struct {
	Size     uint64 `json:"size"`
	Distance string `json:"distance"`
}

type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float64              `json:"vector"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type qdrantSearchRequest struct {
	Vector         []float64     `json:"vector"`
	Limit          int64         `json:"limit"`
	ScoreThreshold float64       `json:"score_threshold,omitempty"`
	Filter         *qdrantFilter `json:"filter,omitempty"`
	WithPayload    bool          `json:"with_payload"`
}

type qdrantFilter struct {
	Must    []qdrantCondition `json:"must,omitempty"`
	MustNot []qdrantCondition `json:"must_not,omitempty"`
}

type qdrantCondition struct {
	Key   string       `json:"key"`
	Match *qdrantMatch `json:"match,omitempty"`
	Range *qdrantRange `json:"range,omitempty"`
}

type qdrantMatch struct {
	Value interface{} `json:"value,omitempty"`
	Any   interface{} `json:"any,omitempty"`
}

type qdrantRange struct {
	Gt  interface{} `json:"gt,omitempty"`
	Gte interface{} `json:"gte,omitempty"`
	Lt  interface{} `json:"lt,omitempty"`
	Lte interface{} `json:"lte,omitempty"`
}

type qdrantSearchResponse struct {
	Result []qdrantSearchResult `json:"result"`
	Status string               `json:"status"`
	Time   float64              `json:"time"`
}

type qdrantSearchResult struct {
	ID      string                 `json:"id"`
	Version int64                  `json:"version"`
	Score   float64                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
	Vector  interface{}            `json:"vector"`
}

type qdrantDeleteRequest struct {
	Points []string `json:"points"`
}
