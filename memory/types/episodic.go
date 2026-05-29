package types

import (
	"awesome-agent/memory/store"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

type EventType string

const (
	EventObservation EventType = "observation"
	EventThought     EventType = "thought"
	EventAction      EventType = "action"
	EventResult      EventType = "result"
)

type RelationType string

const (
	RelBefore    RelationType = "before"
	RelAfter     RelationType = "after"
	RelCausedBy  RelationType = "caused_by"
	RelRelatedTo RelationType = "related_to"
)

type EpisodeRelation struct {
	TargetID     string       `json:"target_id"`
	RelationType RelationType `json:"relation_type"`
}

type EpisodeItem struct {
	MemoryItem
	Summary   string
	EventType EventType
	Relations []EpisodeRelation
}

func (e *EpisodeItem) ToMemoryItem() MemoryItem {
	meta := make(map[string]string)
	for k, v := range e.Metadata {
		meta[k] = v
	}
	meta["summary"] = e.Summary
	meta["event_type"] = string(e.EventType)
	if len(e.Relations) > 0 {
		relations, _ := json.Marshal(e.Relations)
		meta["relations"] = string(relations)
	}
	return MemoryItem{
		ID:         e.ID,
		SessionID:  e.SessionID,
		Type:       Episodic,
		Content:    e.Content,
		CreatedAt:  e.CreatedAt,
		Importance: e.Importance,
		Metadata:   meta,
	}
}

type EpisodicMemory struct {
	sessionID       string
	collection      string
	structuredStore store.StructuredStore
	vectorStore     store.VectorStore
	embeddingSvc    store.EmbeddingService
}

func NewEpisodicMemory(
	sessionID string,
	collection string,
	structuredStore store.StructuredStore,
	vectorStore store.VectorStore,
	embeddingSvc store.EmbeddingService,
) *EpisodicMemory {
	return &EpisodicMemory{
		sessionID:       sessionID,
		collection:      collection,
		structuredStore: structuredStore,
		vectorStore:     vectorStore,
		embeddingSvc:    embeddingSvc,
	}
}

func (e *EpisodicMemory) Init(ctx context.Context) error {
	schema := `
  CREATE TABLE IF NOT EXISTS episodes (
      id          TEXT PRIMARY KEY,
      session_id  TEXT NOT NULL,
      content     TEXT NOT NULL,
      summary     TEXT,
      importance  REAL DEFAULT 0.5,
      event_type  TEXT NOT NULL,
      created_at  TEXT NOT NULL,
      metadata    TEXT DEFAULT '{}'
  );
  CREATE INDEX IF NOT EXISTS idx_episodes_session    ON episodes(session_id);
  CREATE INDEX IF NOT EXISTS idx_episodes_time       ON episodes(created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_episodes_importance ON episodes(importance);
  CREATE INDEX IF NOT EXISTS idx_episodes_type       ON episodes(event_type);

  CREATE TABLE IF NOT EXISTS episode_relations (
      source_id     TEXT NOT NULL,
      target_id     TEXT NOT NULL,
      relation_type TEXT NOT NULL,
      PRIMARY KEY (source_id, target_id, relation_type),
      FOREIGN KEY (source_id) REFERENCES episodes(id) ON DELETE CASCADE,
      FOREIGN KEY (target_id) REFERENCES episodes(id) ON DELETE CASCADE
  );
  CREATE INDEX IF NOT EXISTS idx_relations_source ON episode_relations(source_id);
  CREATE INDEX IF NOT EXISTS idx_relations_target ON episode_relations(target_id);`

	if err := e.structuredStore.Exec(ctx, schema); err != nil {
		return fmt.Errorf("create episodes tables: %w", err)
	}
	return nil
}

func (e *EpisodicMemory) Add(item MemoryItem) (string, error) {
	ctx := context.Background()

	if item.Importance == 0 {
		item.Importance = 0.5
	}

	// 还原为 EpisodeItem
	episode := memoryItemToEpisode(item)

	// 1. 生成 embedding
	vec, err := e.embeddingSvc.Embed(ctx, episode.Summary)
	if err != nil {
		return "", fmt.Errorf("embed: %w", err)
	}

	// 2. 写 SQLite
	record := store.Record{
		"id":         episode.ID,
		"session_id": episode.SessionID,
		"content":    episode.Content,
		"summary":    episode.Summary,
		"importance": episode.Importance,
		"event_type": string(episode.EventType),
		"created_at": episode.CreatedAt.Format(time.RFC3339),
		"metadata":   episode.Metadata,
	}
	if err := e.structuredStore.Save(ctx, "episodes", record); err != nil {
		return "", fmt.Errorf("save sqlite: %w", err)
	}

	// 3. 存储关系
	for _, rel := range episode.Relations {
		_ = e.structuredStore.Exec(ctx,
			"INSERT OR IGNORE INTO episode_relations (source_id, target_id, relation_type) VALUES (?, ?, ?)",
			episode.ID, rel.TargetID, string(rel.RelationType))
	}

	// 4. 写 Qdrant
	payload := map[string]interface{}{
		"session_id": episode.SessionID,
		"event_type": string(episode.EventType),
		"importance": episode.Importance,
		"created_at": episode.CreatedAt.Unix(),
	}
	if err := e.vectorStore.Upsert(ctx, e.collection, store.VectorPoint{
		ID: episode.ID, Vector: vec, Payload: payload,
	}); err != nil {
		return "", fmt.Errorf("upsert qdrant: %w", err)
	}

	return episode.ID, nil
}

func (e *EpisodicMemory) Delete(id string) error {
	ctx := context.Background()
	if err := e.structuredStore.Delete(ctx, "episodes", id); err != nil {
		return fmt.Errorf("delete sqlite: %w", err)
	}
	_ = e.vectorStore.Delete(ctx, e.collection, []string{id})
	return nil
}

func (e *EpisodicMemory) Status() MemoryStatus {
	ctx := context.Background()
	status := MemoryStatus{Type: Episodic}

	all, err := e.structuredStore.Query(ctx, store.Query{Table: "episodes"})
	if err != nil {
		return status
	}
	status.Count = int64(len(all))

	if status.Count > 0 {
		if oldest, err := e.structuredStore.Query(ctx, store.Query{
			Table: "episodes", OrderBy: []store.OrderBy{{Field: "created_at", Dir: "ASC"}}, Limit: 1,
		}); err == nil && len(oldest) > 0 {
			status.OldestItem = parseTime(oldest[0]["created_at"])
		}
		if newest, err := e.structuredStore.Query(ctx, store.Query{
			Table: "episodes", OrderBy: []store.OrderBy{{Field: "created_at", Dir: "DESC"}}, Limit: 1,
		}); err == nil && len(newest) > 0 {
			status.NewestItem = parseTime(newest[0]["created_at"])
		}
	}
	return status
}

func (e *EpisodicMemory) Search(query string, opts SearchOptions) ([]MemoryItem, error) {
	ctx := context.Background()

	vec, err := e.embeddingSvc.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	searchLimit := opts.Limit * 3
	if searchLimit < 20 {
		searchLimit = 20
	}

	filters := e.buildVectorFilters(opts)
	results, err := e.vectorStore.Search(ctx, store.VectorSearch{
		Collection: e.collection,
		Vector:     vec,
		Limit:      searchLimit,
		MinScore:   0.3,
		Filters:    filters,
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
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
		var createdAtUnix float64
		if v, ok := r.Payload["created_at"]; ok {
			createdAtUnix, _ = strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
		}

		cs := episodicScore(r.Score, calcTimeRecency(createdAtUnix), importance)
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

	records, err := e.structuredStore.Query(ctx, store.Query{
		Table: "episodes",
		Conditions: []store.Condition{
			{Field: "id", Operator: "IN", Value: ids},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch records: %w", err)
	}

	episodes := recordsToEpisodes(records)
	// 批量填充关系
	_ = e.populateRelations(ctx, episodes)
	// 转为MemoryItem并填充分数
	episodesMap := make(map[string]EpisodeItem, len(episodes))
	for _, episode := range episodes {
		episodesMap[episode.ID] = episode
	}
	items := make([]MemoryItem, 0, len(scored))
	for _, s := range scored {
		if episodic, ok := episodesMap[s.id]; ok {
			item := episodic.ToMemoryItem()
			item.Metadata["score"] = strconv.FormatFloat(s.score, 'f', 4, 64)
			items = append(items, item)
		}
	}

	return items, nil
}

func episodicScore(vectorSim, timeRecency, importance float64) float64 {
	return (vectorSim*0.8 + timeRecency*0.2) * (0.8 + importance*0.4)
}

func calcTimeRecency(createdAtUnix float64) float64 {
	if createdAtUnix <= 0 {
		return 0
	}
	hoursAgo := time.Since(time.Unix(int64(createdAtUnix), 0)).Hours()
	if hoursAgo < 0 {
		hoursAgo = 0
	}
	tr := math.Exp(-0.05 * hoursAgo)
	tr = max(0.1, tr)
	return tr
}

func (e *EpisodicMemory) buildVectorFilters(opts SearchOptions) []store.VectorFilter {
	var filters []store.VectorFilter
	if opts.MinImportance > 0 {
		filters = append(filters, store.VectorFilter{Field: "importance", Operator: ">=", Value: opts.MinImportance})
	}
	if sid, ok := opts.Filter["session_id"]; ok && sid != "" {
		filters = append(filters, store.VectorFilter{Field: "session_id", Operator: "=", Value: sid})
	}
	if et, ok := opts.Filter["event_type"]; ok {
		filters = append(filters, store.VectorFilter{Field: "event_type", Operator: "=", Value: et})
	}
	return filters
}

func (e *EpisodicMemory) batchDeleteRecords(ctx context.Context, records []store.Record) (int, error) {
	ids := extractIDs(records)
	if len(ids) == 0 {
		return 0, nil
	}
	if err := e.structuredStore.BatchDelete(ctx, "episodes", ids); err != nil {
		return 0, fmt.Errorf("batch delete sqlite: %w", err)
	}
	_ = e.vectorStore.Delete(ctx, e.collection, ids)
	return len(ids), nil
}

// populateRelations 批量查询 episode_relations 并填充 EpisodeItem.Relations
func (e *EpisodicMemory) populateRelations(ctx context.Context, episodes []EpisodeItem) error {
	if len(episodes) == 0 {
		return nil
	}
	ids := make([]string, len(episodes))
	for i, ep := range episodes {
		ids[i] = ep.ID
	}

	records, err := e.structuredStore.Query(ctx, store.Query{
		Table: "episode_relations",
		Conditions: []store.Condition{
			{Field: "source_id", Operator: "IN", Value: ids},
		},
	})
	if err != nil {
		return err
	}

	relMap := make(map[string][]EpisodeRelation)
	for _, r := range records {
		sid := strVal(r["source_id"])
		relMap[sid] = append(relMap[sid], EpisodeRelation{
			TargetID:     strVal(r["target_id"]),
			RelationType: RelationType(strVal(r["relation_type"])),
		})
	}

	for i := range episodes {
		if rels, ok := relMap[episodes[i].ID]; ok {
			episodes[i].Relations = rels
		}
	}
	return nil
}

// memoryItemToEpisode 从 MemoryItem 还原 EpisodeItem
func memoryItemToEpisode(item MemoryItem) EpisodeItem {
	episode := EpisodeItem{
		MemoryItem: item,
		Summary:    item.Content,
		EventType:  EventObservation,
	}
	if s, ok := item.Metadata["summary"]; ok && s != "" {
		episode.Summary = s
	}
	if et, ok := item.Metadata["event_type"]; ok && et != "" {
		episode.EventType = EventType(et)
	}
	if relationStr, ok := item.Metadata["relations"]; ok {
		relations := make([]EpisodeRelation, 0)
		if err := json.Unmarshal([]byte(relationStr), &relations); err == nil {
			episode.Relations = relations
		}
	}
	return episode
}

// recordToEpisodeItem 从 SQLite 行还原 EpisodeItem
func recordToEpisodeItem(r store.Record) EpisodeItem {
	episode := EpisodeItem{
		MemoryItem: MemoryItem{
			ID:        strVal(r["id"]),
			Type:      Episodic,
			Content:   strVal(r["content"]),
			SessionID: strVal(r["session_id"]),
			Metadata:  make(map[string]string),
		},
		Summary:   strVal(r["summary"]),
		EventType: EventType(strVal(r["event_type"])),
	}
	if v := r["importance"]; v != nil {
		episode.Importance, _ = strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
	}
	episode.CreatedAt = parseTime(r["created_at"])

	return episode
}

func recordsToEpisodes(records []store.Record) []EpisodeItem {
	episodes := make([]EpisodeItem, 0, len(records))
	for _, r := range records {
		episodes = append(episodes, recordToEpisodeItem(r))
	}
	return episodes
}

func episodesToMemoryItems(episodes []EpisodeItem) []MemoryItem {
	items := make([]MemoryItem, 0, len(episodes))
	for i := range episodes {
		items = append(items, episodes[i].ToMemoryItem())
	}
	return items
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func parseTime(v interface{}) *time.Time {
	if v == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", v))
	if err != nil {
		return nil
	}
	return &t
}

func extractIDs(records []store.Record) []string {
	ids := make([]string, 0, len(records))
	for _, r := range records {
		if id := strVal(r["id"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
