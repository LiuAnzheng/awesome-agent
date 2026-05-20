package types

import (
	"awesome-agent/memory/store"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type SemanticRelation struct {
	TargetID     string       `json:"target_id"`
	RelationType RelationType `json:"relation_type"`
}

type SemanticItem struct {
	MemoryItem
	Summary   string
	Tags      []string
	Relations []SemanticRelation
}

func (s *SemanticItem) ToMemoryItem() MemoryItem {
	meta := make(map[string]string)
	for k, v := range s.Metadata {
		meta[k] = v
	}
	meta["summary"] = s.Summary
	if len(s.Tags) > 0 {
		meta["tags"] = strings.Join(s.Tags, ",")
	}
	if len(s.Relations) > 0 {
		relations, _ := json.Marshal(s.Relations)
		meta["relations"] = string(relations)
	}
	return MemoryItem{
		ID:         s.ID,
		SessionID:  s.SessionID,
		Content:    s.Content,
		CreatedAt:  s.CreatedAt,
		Importance: s.Importance,
		Metadata:   meta,
	}
}

type SemanticMemory struct {
	sessionID    string
	collection   string
	graphStore   store.GraphStore
	vectorStore  store.VectorStore
	embeddingSvc store.EmbeddingService
}

func NewSemanticMemory(
	sessionID string,
	collection string,
	graphStore store.GraphStore,
	vectorStore store.VectorStore,
	embeddingSvc store.EmbeddingService,
) *SemanticMemory {
	return &SemanticMemory{
		sessionID:    sessionID,
		collection:   collection,
		graphStore:   graphStore,
		vectorStore:  vectorStore,
		embeddingSvc: embeddingSvc,
	}
}

func (s *SemanticMemory) Add(item MemoryItem) (string, error) {
	ctx := context.Background()

	if item.Importance == 0 {
		item.Importance = 0.5
	}

	// 还原为 SemanticItem
	semantic := memoryItemToSemantic(item)

	// 1. 生成 embedding
	vec, err := s.embeddingSvc.Embed(ctx, semantic.Summary)
	if err != nil {
		return "", fmt.Errorf("embed: %w", err)
	}

	// 2. 去重检查
	existing, err := s.vectorStore.Search(ctx, store.VectorSearch{
		Collection: s.collection,
		Vector:     vec,
		Limit:      3,
		MinScore:   0.85,
	})
	if err == nil && len(existing) > 0 && existing[0].Score > 0.9 {
		existingID := existing[0].ID
		oldNode, err := s.graphStore.GetNode(ctx, existingID)
		if err == nil && oldNode.ID != "" {
			oldImp, _ := strconv.ParseFloat(fmt.Sprintf("%v", oldNode.Properties["importance"]), 64)
			_ = s.graphStore.UpdateNode(ctx, existingID, map[string]interface{}{
				"importance": math.Max(oldImp, semantic.Importance),
			})
		}
		for _, rel := range semantic.Relations {
			_ = s.graphStore.CreateRelation(ctx, store.GraphRelation{
				SourceID: existingID, TargetID: rel.TargetID, RelationType: string(rel.RelationType),
			})
		}
		return existingID, nil
	}

	// 3. 创建 Neo4j 节点
	props := map[string]interface{}{
		"id":         semantic.ID,
		"session_id": semantic.SessionID,
		"content":    semantic.Content,
		"summary":    semantic.Summary,
		"importance": semantic.Importance,
		"labels":     semantic.Tags,
	}
	if len(semantic.Metadata) > 0 {
		bytes, _ := json.Marshal(semantic.Metadata)
		props["metadata"] = string(bytes)
	}

	node := store.GraphNode{
		ID:         semantic.ID,
		Tags:       semantic.Tags,
		Properties: props,
	}
	if err := s.graphStore.CreateNode(ctx, node); err != nil {
		return "", fmt.Errorf("create graph node: %w", err)
	}

	// 4. 创建图关系
	for _, rel := range semantic.Relations {
		_ = s.graphStore.CreateRelation(ctx, store.GraphRelation{
			SourceID: semantic.ID, TargetID: rel.TargetID, RelationType: string(rel.RelationType),
		})
	}

	// 5. 写 Qdrant
	payload := map[string]interface{}{
		"session_id": semantic.SessionID,
		"importance": semantic.Importance,
		"labels":     semantic.Tags,
	}
	if err := s.vectorStore.Upsert(ctx, s.collection, store.VectorPoint{
		ID: semantic.ID, Vector: vec, Payload: payload,
	}); err != nil {
		return "", fmt.Errorf("upsert qdrant: %w", err)
	}

	return semantic.ID, nil
}

func (s *SemanticMemory) Delete(id string) error {
	ctx := context.Background()
	if err := s.graphStore.DeleteNode(ctx, id); err != nil {
		return fmt.Errorf("delete graph node: %w", err)
	}
	_ = s.vectorStore.Delete(ctx, s.collection, []string{id})
	return nil
}

func (s *SemanticMemory) Status() MemoryStatus {
	ctx := context.Background()
	status := MemoryStatus{Type: Semantic}

	rows, err := s.graphStore.Query(ctx, "MATCH (n) RETURN count(n) AS count", nil)
	if err == nil && len(rows) > 0 {
		if c, ok := rows[0]["count"]; ok {
			status.Count, _ = strconv.ParseInt(fmt.Sprintf("%v", c), 10, 64)
		}
	}
	return status
}

func (s *SemanticMemory) Search(query string, opts SearchOptions) ([]MemoryItem, error) {
	ctx := context.Background()

	vec, err := s.embeddingSvc.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	searchLimit := opts.Limit * 3
	if searchLimit < 20 {
		searchLimit = 20
	}

	results, err := s.vectorStore.Search(ctx, store.VectorSearch{
		Collection: s.collection,
		Vector:     vec,
		Limit:      searchLimit,
		MinScore:   0.3,
		Filters:    s.buildVectorFilters(opts),
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	candidateIDs := make([]string, len(results))
	for i, r := range results {
		candidateIDs[i] = r.ID
	}
	candidateSet := make(map[string]bool, len(candidateIDs))
	for _, id := range candidateIDs {
		candidateSet[id] = true
	}

	neighborMap, err := s.batchGetNeighborOverlap(ctx, candidateIDs)
	if err != nil {
		neighborMap = make(map[string][]string)
	}

	type scoredItem struct {
		id    string
		score float64
	}
	scored := make([]scoredItem, 0, len(results))
	for _, r := range results {
		importance := 0.5
		if v, ok := r.Payload["importance"]; ok {
			importance, _ = strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
		}

		cs := semanticScore(r.Score, calcGraphSim(r.ID, neighborMap, candidateSet), importance)
		if opts.MinScore > 0 && cs < opts.MinScore {
			continue
		}
		scored = append(scored, scoredItem{id: r.ID, score: cs})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	if opts.Limit > 0 && int64(len(scored)) > opts.Limit {
		scored = scored[:opts.Limit]
	}

	ids := make([]string, len(scored))
	for i, s := range scored {
		ids[i] = s.id
	}
	if len(ids) == 0 {
		return nil, nil
	}

	idList := make([]string, len(ids))
	for i, id := range ids {
		idList[i] = "'" + id + "'"
	}
	cypher := fmt.Sprintf("MATCH (n) WHERE n.id IN [%s] RETURN n", strings.Join(idList, ","))
	rows, err := s.graphStore.Query(ctx, cypher, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch graph nodes: %w", err)
	}

	nodeMap := make(map[string]SemanticItem)
	var semanticItems []SemanticItem
	for _, row := range rows {
		if node, ok := row["n"].(map[string]interface{}); ok {
			item := nodeToSemanticItem(node)
			nodeMap[item.ID] = item
			semanticItems = append(semanticItems, item)
		}
	}
	_ = s.populateRelations(ctx, semanticItems)
	for i := range semanticItems {
		nodeMap[semanticItems[i].ID] = semanticItems[i]
	}

	items := make([]MemoryItem, 0, len(scored))
	for _, s := range scored {
		if sem, ok := nodeMap[s.id]; ok {
			item := sem.ToMemoryItem()
			item.Metadata["score"] = strconv.FormatFloat(s.score, 'f', 4, 64)
			items = append(items, item)
		}
	}
	return items, nil
}

func semanticScore(vectorSim, graphSim, importance float64) float64 {
	return (vectorSim*0.7 + graphSim*0.3) * (0.8 + importance*0.4)
}

func calcGraphSim(nodeID string, neighborMap map[string][]string, candidateSet map[string]bool) float64 {
	neighbors, ok := neighborMap[nodeID]
	if !ok || len(neighbors) == 0 {
		return 0
	}
	coOccur := 0
	for _, nid := range neighbors {
		if candidateSet[nid] {
			coOccur++
		}
	}
	return float64(coOccur) / float64(len(neighbors))
}

func (s *SemanticMemory) batchGetNeighborOverlap(ctx context.Context, ids []string) (map[string][]string, error) {
	idList := make([]string, len(ids))
	for i, id := range ids {
		idList[i] = "'" + id + "'"
	}
	cypher := fmt.Sprintf(
		"MATCH (n)-[r]-(m) WHERE n.id IN [%s] AND m.id IN [%s] RETURN n.id AS node_id, COLLECT(DISTINCT m.id) AS neighbors",
		strings.Join(idList, ","),
		strings.Join(idList, ","),
	)

	rows, err := s.graphStore.Query(ctx, cypher, nil)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string, len(rows))
	for _, row := range rows {
		nodeID, _ := row["node_id"].(string)
		var neighbors []string
		if nList, ok := row["neighbors"].([]interface{}); ok {
			for _, n := range nList {
				if s, ok := n.(string); ok {
					neighbors = append(neighbors, s)
				}
			}
		}
		result[nodeID] = neighbors
	}
	return result, nil
}

func (s *SemanticMemory) buildVectorFilters(opts SearchOptions) []store.VectorFilter {
	var filters []store.VectorFilter
	if opts.MinImportance > 0 {
		filters = append(filters, store.VectorFilter{Field: "importance", Operator: ">=", Value: opts.MinImportance})
	}
	if sid, ok := opts.Filter["session_id"]; ok && sid != "" {
		filters = append(filters, store.VectorFilter{Field: "session_id", Operator: "=", Value: sid})
	}
	if label, ok := opts.Filter["tags"]; ok && label != "" {
		filters = append(filters, store.VectorFilter{Field: "labels", Operator: "IN", Value: []string{label}})
	}
	return filters
}

// populateRelations 批量查询 Neo4j 关系并填充 SemanticItem.Relations
func (s *SemanticMemory) populateRelations(ctx context.Context, items []SemanticItem) error {
	if len(items) == 0 {
		return nil
	}
	idList := make([]string, len(items))
	for i, item := range items {
		idList[i] = "'" + item.ID + "'"
	}

	cypher := fmt.Sprintf(
		"MATCH (n)-[r]-(m) WHERE n.id IN [%s] RETURN n.id AS node_id, type(r) AS rel_type, m.id AS target_id",
		strings.Join(idList, ","),
	)
	rows, err := s.graphStore.Query(ctx, cypher, nil)
	if err != nil {
		return err
	}

	relMap := make(map[string][]SemanticRelation)
	for _, row := range rows {
		nodeID, _ := row["node_id"].(string)
		targetID, _ := row["target_id"].(string)
		relType, _ := row["rel_type"].(string)
		relMap[nodeID] = append(relMap[nodeID], SemanticRelation{
			TargetID:     targetID,
			RelationType: RelationType(relType),
		})
	}

	for i := range items {
		if rels, ok := relMap[items[i].ID]; ok {
			items[i].Relations = rels
		}
	}
	return nil
}

// memoryItemToSemantic 从 MemoryItem 还原 SemanticItem
func memoryItemToSemantic(item MemoryItem) SemanticItem {
	s := SemanticItem{
		MemoryItem: item,
		Summary:    item.Content,
		Tags:       []string{"Concept"},
	}
	if item.Metadata["summary"] != "" {
		s.Summary = item.Metadata["summary"]
	}
	if item.Metadata["tags"] != "" {
		s.Tags = parseTags(item.Metadata["tags"])
	}
	// 解析 Relations
	s.Relations = parseSemanticRelations(item.Metadata)
	return s
}

// nodeToSemanticItem 将 Neo4j 节点转为 SemanticItem
func nodeToSemanticItem(nodeMap map[string]interface{}) SemanticItem {
	props, _ := nodeMap["properties"].(map[string]interface{})
	var labels []string
	if l, ok := nodeMap["labels"].([]interface{}); ok {
		for _, ll := range l {
			if s, ok := ll.(string); ok {
				labels = append(labels, s)
			}
		}
	}

	item := SemanticItem{
		MemoryItem: MemoryItem{
			Metadata: make(map[string]string),
		},
		Tags: labels,
	}
	if props != nil {
		item.ID = fmt.Sprintf("%v", props["id"])
		item.Content = fmt.Sprintf("%v", props["content"])
		item.SessionID = fmt.Sprintf("%v", props["session_id"])
		item.Summary = fmt.Sprintf("%v", props["summary"])

		if v := props["importance"]; v != nil {
			item.Importance, _ = strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
		}
	}
	return item
}

func semanticsToMemoryItems(items []SemanticItem) []MemoryItem {
	result := make([]MemoryItem, 0, len(items))
	for i := range items {
		result = append(result, items[i].ToMemoryItem())
	}
	return result
}

func parseTags(labelStr string) []string {
	if labelStr == "" {
		return nil
	}
	parts := strings.Split(labelStr, ",")
	labels := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			labels = append(labels, p)
		}
	}
	return labels
}

func parseSemanticRelations(metadata map[string]string) []SemanticRelation {
	if metadata == nil {
		return nil
	}
	rel, ok := metadata["relations"]
	if !ok {
		return nil
	}
	var relations []SemanticRelation
	if err := json.Unmarshal([]byte(rel), &relations); err != nil {
		return nil
	}
	return relations
}

func validateTagName(label string) error {
	for _, ch := range label {
		if !(('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') ||
			('0' <= ch && ch <= '9') || ch == '_' || ch == '-') {
			return fmt.Errorf("invalid label: %s", label)
		}
	}
	return nil
}
