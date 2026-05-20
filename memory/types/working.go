package types

import (
	"awesome-agent/core"
	"strings"
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
}

func NewWorkingMemory(maxCapacity, maxAgeMinutes int64, sessionID string) *WorkingMemory {
	if maxCapacity <= 0 {
		maxCapacity = 1024
	}
	if maxAgeMinutes <= 0 {
		maxAgeMinutes = 60
	}
	return &WorkingMemory{
		SessionID:     sessionID,
		MaxCapacity:   maxCapacity,
		MaxAgeMinutes: maxAgeMinutes,
		items:         make([]WorkingItem, 0, maxCapacity),
	}
}

func (w *WorkingMemory) Add(item MemoryItem) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// FIFO 淘汰：容量满时移除最早的
	if int64(len(w.items)) >= w.MaxCapacity {
		w.items = w.items[1:]
	}

	w.items = append(w.items, WorkingItem{MemoryItem: item})

	return item.ID, nil
}

func (w *WorkingMemory) Search(query string, opts SearchOptions) ([]MemoryItem, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	cutOff := core.Now().Add(-time.Duration(w.MaxAgeMinutes) * time.Minute)
	res := make([]MemoryItem, 0)

	// 倒序遍历，优先返回最近的
	for i := len(w.items) - 1; i >= 0; i-- {
		item := w.items[i]

		// 过期跳过
		if item.CreatedAt != nil && item.CreatedAt.Before(cutOff) {
			continue
		}

		// 关键词过滤
		if query != "" && !strings.Contains(item.Content, query) {
			continue
		}

		res = append(res, item.MemoryItem)
		if opts.Limit > 0 && int64(len(res)) >= opts.Limit {
			break
		}
	}

	return res, nil
}

func (w *WorkingMemory) Delete(id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, item := range w.items {
		if item.ID == id {
			w.items = append(w.items[:i], w.items[i+1:]...)
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
}
