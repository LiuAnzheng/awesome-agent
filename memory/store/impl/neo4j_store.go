package impl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"awesome-agent/memory/store"
)

type Neo4jStore struct {
	client *http.Client
	txURL  string
	auth   string
}

func NewNeo4jStore(options map[string]interface{}) *Neo4jStore {
	url := "http://127.0.0.1:7474"
	dbName := "neo4j"
	username := "neo4j"
	password := "neo4j"

	if v, ok := options["url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			url = s
		}
	}
	if v, ok := options["db"]; ok {
		if s, ok := v.(string); ok && s != "" {
			dbName = s
		}
	}
	if v, ok := options["username"]; ok {
		if s, ok := v.(string); ok && s != "" {
			username = s
		}
	}
	if v, ok := options["password"]; ok {
		if s1, ok := v.(string); ok {
			password = s1
		}
		if s2, ok := v.(int); ok {
			password = strconv.Itoa(s2)
		}
	}

	return &Neo4jStore{
		client: &http.Client{Timeout: 30 * time.Second},
		txURL:  fmt.Sprintf("%s/db/%s/tx/commit", strings.TrimRight(url, "/"), dbName),
		auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)),
	}
}

// Init 检查 Neo4j 是否可达
func (n *Neo4jStore) Init(ctx context.Context) error {
	_, err := n.runCypher(ctx, []neo4jStatement{
		{Statement: "RETURN 1 AS ok"},
	})
	return err
}

// CreateNode 创建或更新节点（MERGE 实现幂等）
func (n *Neo4jStore) CreateNode(ctx context.Context, node store.GraphNode) error {
	if node.ID == "" {
		return fmt.Errorf("node id is required")
	}
	labels := make([]string, len(node.Tags))
	for i, t := range node.Tags {
		if err := validateToken(t); err != nil {
			return err
		}
		labels[i] = ":" + t
	}

	props := make(map[string]interface{})
	for k, v := range node.Properties {
		props[k] = v
	}
	props["id"] = node.ID

	cypher := fmt.Sprintf("MERGE (n%s {id: $id}) SET n += $props", strings.Join(labels, ""))

	_, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement: cypher,
			Parameters: map[string]interface{}{
				"id":    node.ID,
				"props": props,
			},
		},
	})
	return err
}

// UpdateNode 更新节点属性
func (n *Neo4jStore) UpdateNode(ctx context.Context, id string, props map[string]interface{}) error {
	_, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement:  "MATCH (n {id: $id}) SET n += $props",
			Parameters: map[string]interface{}{"id": id, "props": props},
		},
	})
	return err
}

// GetNode 按 ID 获取节点
func (n *Neo4jStore) GetNode(ctx context.Context, id string) (store.GraphNode, error) {
	resp, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement:  "MATCH (n {id: $id}) RETURN n",
			Parameters: map[string]interface{}{"id": id},
		},
	})
	if err != nil {
		return store.GraphNode{}, err
	}
	if len(resp.Results) == 0 || len(resp.Results[0].Data) == 0 {
		return store.GraphNode{}, nil
	}
	return parseNode(resp.Results[0].Data[0].Row, 0)
}

// DeleteNode 删除节点（DETACH 同时删除关联关系）
func (n *Neo4jStore) DeleteNode(ctx context.Context, id string) error {
	_, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement:  "MATCH (n {id: $id}) DETACH DELETE n",
			Parameters: map[string]interface{}{"id": id},
		},
	})
	return err
}

// CreateRelation 创建节点间关系
func (n *Neo4jStore) CreateRelation(ctx context.Context, rel store.GraphRelation) error {
	if err := validateToken(rel.RelationType); err != nil {
		return err
	}
	cypher := fmt.Sprintf(
		"MATCH (a {id: $source}), (b {id: $target}) MERGE (a)-[r:%s]->(b)",
		rel.RelationType,
	)
	_, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement: cypher,
			Parameters: map[string]interface{}{
				"source": rel.SourceID,
				"target": rel.TargetID,
			},
		},
	})
	return err
}

// GetNeighbors 获取节点的所有邻居
func (n *Neo4jStore) GetNeighbors(ctx context.Context, id string) ([]store.GraphNode, error) {
	resp, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement:  "MATCH (n {id: $id})-[r]-(m) RETURN DISTINCT m",
			Parameters: map[string]interface{}{"id": id},
		},
	})
	if err != nil {
		return nil, err
	}
	return parseNodeList(resp, 0), nil
}

// GetNeighborIDs 获取邻居 ID 集合（只返回 id 属性，无需取全量节点）
func (n *Neo4jStore) GetNeighborIDs(ctx context.Context, id string) ([]string, error) {
	resp, err := n.runCypher(ctx, []neo4jStatement{
		{
			Statement:  "MATCH (n {id: $id})-[r]-(m) RETURN DISTINCT m.id AS neighbor_id",
			Parameters: map[string]interface{}{"id": id},
		},
	})
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, data := range extractDataList(resp) {
		for _, item := range data.Row {
			if s, ok := item.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
	}
	return ids, nil
}

// Query 执行任意 Cypher 查询
func (n *Neo4jStore) Query(ctx context.Context, cypher string, params map[string]interface{}) ([]map[string]interface{}, error) {
	resp, err := n.runCypher(ctx, []neo4jStatement{
		{Statement: cypher, Parameters: params},
	})
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]interface{}, 0)
	for _, result := range resp.Results {
		columns := result.Columns
		for _, data := range result.Data {
			row := make(map[string]interface{}, len(columns))
			for i, col := range columns {
				if i < len(data.Row) {
					row[col] = data.Row[i]
				}
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// Close 关闭连接池
func (n *Neo4jStore) Close() error {
	n.client.CloseIdleConnections()
	return nil
}

// ============================================================
// 内部工具
// ============================================================

func validateToken(token string) error {
	if token == "" {
		return fmt.Errorf("token must not be empty")
	}
	for _, ch := range token {
		if !(('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') ||
			('0' <= ch && ch <= '9') || ch == '_' || ch == '-') {
			return fmt.Errorf("invalid token %q: only alphanumeric, underscore, and dash allowed", token)
		}
	}
	return nil
}

func (n *Neo4jStore) runCypher(ctx context.Context, statements []neo4jStatement) (*neo4jResponse, error) {
	reqBody := neo4jRequest{Statements: statements}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.txURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.Header.Set("Authorization", n.auth)

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result neo4jResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Errors) > 0 {
		errMsgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			errMsgs[i] = fmt.Sprintf("%s: %s", e.Code, e.Message)
		}
		return nil, fmt.Errorf("neo4j: %s", strings.Join(errMsgs, "; "))
	}

	return &result, nil
}

func parseNode(row []interface{}, col int) (store.GraphNode, error) {
	if col >= len(row) || row[col] == nil {
		return store.GraphNode{}, nil
	}
	nodeMap, ok := row[col].(map[string]interface{})
	if !ok {
		return store.GraphNode{}, fmt.Errorf("unexpected node type: %T", row[col])
	}
	return mapToNode(nodeMap), nil
}

func parseNodeList(resp *neo4jResponse, col int) []store.GraphNode {
	var nodes []store.GraphNode
	for _, data := range extractDataList(resp) {
		if n, err := parseNode(data.Row, col); err == nil && n.ID != "" {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func extractDataList(resp *neo4jResponse) []neo4jData {
	var all []neo4jData
	for _, result := range resp.Results {
		all = append(all, result.Data...)
	}
	return all
}

func mapToNode(m map[string]interface{}) store.GraphNode {
	node := store.GraphNode{Properties: make(map[string]interface{})}

	if labels, ok := m["labels"].([]interface{}); ok {
		node.Tags = make([]string, 0, len(labels))
		for _, l := range labels {
			if s, ok := l.(string); ok {
				node.Tags = append(node.Tags, s)
			}
		}
	}
	if props, ok := m["properties"].(map[string]interface{}); ok {
		node.Properties = props
		if id, ok := props["id"].(string); ok {
			node.ID = id
		}
	}
	return node
}

// ============================================================
// Neo4j REST API 数据结构
// ============================================================

type neo4jRequest struct {
	Statements []neo4jStatement `json:"statements"`
}

type neo4jStatement struct {
	Statement  string                 `json:"statement"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type neo4jResponse struct {
	Results []neo4jResult `json:"results"`
	Errors  []neo4jError  `json:"errors"`
}

type neo4jResult struct {
	Columns []string    `json:"columns"`
	Data    []neo4jData `json:"data"`
}

type neo4jData struct {
	Row  []interface{} `json:"row"`
	Meta []neo4jMeta   `json:"meta"`
}

type neo4jMeta struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	Deleted bool   `json:"deleted"`
}

type neo4jError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
