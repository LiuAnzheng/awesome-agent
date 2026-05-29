package types

import (
	"awesome-agent/core"
	"awesome-agent/memory/retrieval"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type WorkingItem struct {
	MemoryItem
}

type WorkingMemory struct {
	SessionID     string
	MaxCapacity   int64
	MaxAgeMinutes int64
	items         []WorkingItem
	mu            sync.RWMutex

	compressing atomic.Bool

	tokenizer *retrieval.Tokenizer
	bm25      *retrieval.BM25Scorer
	id2Idx    map[string]int
	bm25Dirty bool
}

func NewWorkingMemory(maxCapacity, maxAgeMinutes int64, sessionID string, tokenizer *retrieval.Tokenizer) *WorkingMemory {
	if maxCapacity <= 0 {
		maxCapacity = 1024
	}
	if maxAgeMinutes <= 0 {
		maxAgeMinutes = 60
	}
	if tokenizer == nil {
		tokenizer = retrieval.GetTokenizer()
	}
	return &WorkingMemory{
		SessionID:     sessionID,
		MaxCapacity:   maxCapacity,
		MaxAgeMinutes: maxAgeMinutes,
		items:         make([]WorkingItem, 0, maxCapacity),
		tokenizer:     tokenizer,
		bm25:          retrieval.NewBM25Scorer(tokenizer),
		id2Idx:        make(map[string]int),
		bm25Dirty:     true,
	}
}

func (w *WorkingMemory) Add(item MemoryItem) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// FIFO 淘汰：容量满时移除最早的
	if int64(len(w.items)) >= w.MaxCapacity {
		w.items = w.items[1:]
	}

	item.Type = Working
	w.items = append(w.items, WorkingItem{MemoryItem: item})
	w.bm25Dirty = true

	return item.ID, nil
}

func (w *WorkingMemory) Search(query string, opts SearchOptions) ([]MemoryItem, error) {
	w.mu.Lock()

	// 重建索引
	if w.bm25Dirty {
		docs := make([]string, len(w.items))
		w.id2Idx = make(map[string]int, len(w.items))
		for i, item := range w.items {
			docs[i] = item.Content
			w.id2Idx[item.ID] = i
		}
		w.bm25.BuildIndex(docs)
		w.bm25Dirty = false
	}
	w.mu.Unlock()

	// 过期截止时间
	cutOff := core.Now().Add(-time.Duration(w.MaxAgeMinutes) * time.Minute)

	if query == "" {
		return w.noQuerySearch(cutOff, opts), nil
	}

	// BM25 检索
	scored := w.bm25.Search(query, int(opts.Limit*3))
	if len(scored) == 0 {
		return nil, nil
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	res := make([]MemoryItem, 0, len(scored))
	for _, sd := range scored {
		if sd.DocID >= len(w.items) {
			continue
		}
		item := w.items[sd.DocID].MemoryItem

		if item.CreatedAt != nil && item.CreatedAt.Before(cutOff) {
			continue
		}

		if item.Metadata == nil {
			item.Metadata = make(map[string]string)
		}
		item.Metadata["score"] = strconv.FormatFloat(sd.Score, 'f', 4, 64)

		res = append(res, item)
		if opts.Limit > 0 && int64(len(res)) >= opts.Limit {
			break
		}
	}

	return res, nil
}

func (w *WorkingMemory) noQuerySearch(cutOff time.Time, opts SearchOptions) []MemoryItem {
	w.mu.RLock()
	defer w.mu.RUnlock()

	res := make([]MemoryItem, 0)
	for i := len(w.items) - 1; i >= 0; i-- {
		item := w.items[i].MemoryItem
		if item.CreatedAt != nil && item.CreatedAt.Before(cutOff) {
			continue
		}
		res = append(res, item)
		if opts.Limit > 0 && int64(len(res)) >= opts.Limit {
			break
		}
	}
	return res
}

func (w *WorkingMemory) Delete(id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, item := range w.items {
		if item.ID == id {
			w.items = append(w.items[:i], w.items[i+1:]...)
			w.bm25Dirty = true
			return nil
		}
	}
	return nil
}

func (w *WorkingMemory) Status() MemoryStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	status := MemoryStatus{
		Type:  Working,
		Count: int64(len(w.items)),
	}

	if len(w.items) > 0 {
		status.OldestItem = w.items[0].CreatedAt
		status.NewestItem = w.items[len(w.items)-1].CreatedAt
	}

	return status
}

// IsNearCapacity 是否达到压缩阈值（90% 容量）且未在压缩中
func (w *WorkingMemory) IsNearCapacity() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return int64(len(w.items)) >= w.MaxCapacity*9/10 && !w.compressing.Load()
}

// TryLockCompress CAS 获取压缩锁，用于防止并发压缩
func (w *WorkingMemory) TryLockCompress() bool {
	return w.compressing.CompareAndSwap(false, true)
}

// UnlockCompress 释放压缩锁
func (w *WorkingMemory) UnlockCompress() {
	w.compressing.Store(false)
}

// TakeSnapshot 获取前 n 条记忆的快照，先到先压缩
func (w *WorkingMemory) TakeSnapshot(n int) []MemoryItem {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if n > len(w.items) {
		n = len(w.items)
	}
	items := make([]MemoryItem, n)
	for i := 0; i < n; i++ {
		items[i] = w.items[i].MemoryItem
	}
	return items
}

// RemoveItems 批量删除指定 ID 的记忆
func (w *WorkingMemory) RemoveItems(ids []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remove := make(map[string]bool, len(ids))
	for _, id := range ids {
		remove[id] = true
	}
	kept := w.items[:0]
	for _, item := range w.items {
		if !remove[item.ID] {
			kept = append(kept, item)
		}
	}
	w.items = kept
	w.bm25Dirty = true
}
