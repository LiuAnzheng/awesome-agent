package types

import (
	"awesome-agent/core"
	"strconv"
	"strings"
	"sync"
	"time"
)

type WorkingItem struct {
	MemoryItem
}

type WorkingMemory struct {
	MaxCapacity   int64
	MaxAgeMinutes int64
	items         []WorkingItem
	mu            sync.RWMutex
}

func NewWorkingMemory(maxCapacity, maxAgeMinutes int64) *WorkingMemory {
	if maxCapacity <= 0 {
		maxCapacity = 1024
	}
	if maxAgeMinutes <= 0 {
		maxAgeMinutes = 60
	}
	return &WorkingMemory{
		MaxCapacity:   maxCapacity,
		MaxAgeMinutes: maxAgeMinutes,
		items:         make([]WorkingItem, 0, maxCapacity),
	}
}

func (w *WorkingMemory) Add(item MemoryItem) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if item.ID == "" {
		item.ID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	now := core.Now()
	item.CreatedAt = &now

	// FIFO 淘汰：容量满时移除最早的
	if int64(len(w.items)) >= w.MaxCapacity {
		w.items = w.items[1:]
	}

	w.items = append(w.items, WorkingItem{MemoryItem: item})
	return item.ID, nil
}

func (w *WorkingMemory) Retrieve(query string, limit int64, metadata map[string]string) ([]MemoryItem, error) {
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

		// metadata 精确匹配过滤
		if !matchMetadata(item.Metadata, metadata) {
			continue
		}

		res = append(res, item.MemoryItem)
		if limit > 0 && int64(len(res)) >= limit {
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
	return nil // 不存在也不报错
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

// Clear 清空所有工作记忆
func (w *WorkingMemory) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.items = w.items[:0]
}

// matchMetadata 检查 item 的 metadata 是否包含所有 filter 中的键值对
func matchMetadata(itemMeta map[string]string, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}
	if len(itemMeta) == 0 {
		return false
	}
	for k, v := range filter {
		if itemMeta[k] != v {
			return false
		}
	}
	return true
}
