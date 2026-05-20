package types

import "time"

type MemoryType string

var AvailableMemoryTypes = []MemoryType{Working, Episodic, Semantic}

// 记忆类型
const (
	Working    MemoryType = "working"
	Episodic   MemoryType = "episodic"
	Semantic   MemoryType = "semantic"
	Perceptual MemoryType = "perceptual"
)

type MemoryStatus struct {
	Type       MemoryType
	Count      int64
	StoreSize  int64
	OldestItem *time.Time
	NewestItem *time.Time
}

type SearchOptions struct {
	Limit         int64
	MinScore      float64
	MinImportance float64
	Filter        map[string]string // 支持session_id、tags/event_type过滤
}

type MemoryItem struct {
	ID         string            `json:"id"`
	SessionID  string            `json:"session_id"`
	Content    string            `json:"content"`
	CreatedAt  *time.Time        `json:"created_at"`
	Importance float64           `json:"importance"`
	Metadata   map[string]string `json:"metadata"`

	SourceIDs      []string   `json:"source_ids,omitempty"`
	CompressedFrom MemoryType `json:"compressed_from,omitempty"`
	Status         string     `json:"status"` // "active" | "compressed"
}

type Memory interface {
	Add(item MemoryItem) (string, error)
	Search(query string, opts SearchOptions) ([]MemoryItem, error)
	Delete(id string) error
	Status() MemoryStatus
}
