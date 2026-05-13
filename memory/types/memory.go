package types

import "time"

type MemoryType string
type ForgotStrategy string

var AvailableMemoryTypes = []MemoryType{Working, Episodic, Semantic}

// 记忆类型
const (
	Working    MemoryType = "working"
	Episodic   MemoryType = "episodic"
	Semantic   MemoryType = "semantic"
	Perceptual MemoryType = "perceptual"
)

// 遗忘策略
const (
	ImportanceBased ForgotStrategy = "importance_based"
	TimeBased       ForgotStrategy = "time_based"
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
}

type Memory interface {
	Add(item MemoryItem) (string, error)
	Retrieve(query string, limit int64, metadata map[string]string) ([]MemoryItem, error)
	Delete(id string) error
	Status() MemoryStatus
}

type Forgettable interface {
	Memory
	Forget(strategy ForgotStrategy, threshold float64, maxAgeDays int64) (int, error)
}

type Searchable interface {
	Memory
	Search(query string, opts SearchOptions) ([]MemoryItem, error)
}
