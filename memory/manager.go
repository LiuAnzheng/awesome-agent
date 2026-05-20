package memory

import (
	"awesome-agent/core"
	"awesome-agent/memory/store"
	"awesome-agent/memory/types"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

type Manager struct {
	config core.MemoryConfig

	SessionId        string
	EnableWorking    bool
	EnableEpisodic   bool
	EnableSemantic   bool
	EnablePerceptual bool

	vectorStore      store.VectorStore
	structuredStore  store.StructuredStore
	embeddingService store.EmbeddingService
	graphStore       store.GraphStore

	Memories map[types.MemoryType]types.Memory

	compressor []types.Compressor
}

func NewManager(
	config core.AppConfig,
	sessionId string,
	enableWorking bool,
	enableEpisodic bool,
	enableSemantic bool,
	enablePerceptual bool,
	vectorStore store.VectorStore,
	structuredStore store.StructuredStore,
	embeddingService store.EmbeddingService,
	graphStore store.GraphStore,

	llm core.LLMInterface,
) (*Manager, error) {
	if sessionId == "" {
		return nil, errors.New("sessionId is required")
	}
	m := &Manager{
		config: config.Memory,

		SessionId:        sessionId,
		EnableWorking:    enableWorking,
		EnableEpisodic:   enableEpisodic,
		EnableSemantic:   enableSemantic,
		EnablePerceptual: enablePerceptual,
		vectorStore:      vectorStore,
		structuredStore:  structuredStore,
		embeddingService: embeddingService,
		graphStore:       graphStore,

		compressor: make([]types.Compressor, 0),
	}

	m.Memories = make(map[types.MemoryType]types.Memory)
	if enableWorking {
		m.Memories[types.Working] = types.NewWorkingMemory(1024, 360, m.SessionId)
	}
	if enableEpisodic {
		episodic := types.NewEpisodicMemory(m.SessionId,
			"episodes",
			m.structuredStore,
			m.vectorStore,
			m.embeddingService)
		if err := episodic.Init(context.Background()); err != nil {
			return nil, err
		}
		m.Memories[types.Episodic] = episodic
	}
	if enableSemantic {
		semantic := types.NewSemanticMemory(m.SessionId,
			"semantic",
			m.graphStore,
			m.vectorStore,
			m.embeddingService)
		m.Memories[types.Semantic] = semantic
	}
	if enablePerceptual {
		m.Memories[types.Perceptual] = nil
	}

	// 记忆压缩相关
	if m.EnableWorking && m.EnableEpisodic {
		compressor := types.NewWorking2EpisodicCompressor(llm)

		m.compressor = append(m.compressor, compressor)
	}

	return m, nil
}

// Add 添加一条记忆到Working
func (m *Manager) Add(item types.MemoryItem) (string, error) {
	now := core.Now()
	item.SessionID = m.SessionId
	item.CreatedAt = &now
	item.ID = strconv.FormatInt(time.Now().UnixNano(), 10)

	mem, ok := m.Memories[types.Working]
	if !ok {
		return "", errors.New("working memory does not exist")
	}

	wm, ok := mem.(*types.WorkingMemory)
	if !ok {
		return "", errors.New("working memory type assertion failed")
	}

	id, err := wm.Add(item)
	if err != nil {
		return "", err
	}

	// 容量达标 + 压缩器可用 → 异步压缩
	if wm.IsNearCapacity() && len(m.compressor) > 0 {
		slog.Info("Trigger memory compression[Working -> Episodic]")
		go m.compressWorking()
	}

	return id, nil
}

// compressWorking 取当前 Working 快照 → LLM 压缩 → 写入 Episodic → 清理 Working
func (m *Manager) compressWorking() {
	defer func() {
		recovered := recover()
		if recovered != nil {
			slog.Error("Failed to compress working memory", "error", recovered)
		}
	}()

	wm, ok := m.Memories[types.Working].(*types.WorkingMemory)
	if !ok {
		return
	}

	if !wm.TryLockCompress() {
		return // 已有压缩在执行
	}
	defer wm.UnlockCompress()

	items := wm.TakeSnapshot(30)

	ctx := context.Background()
	newItem, err := m.compressor[0].Summarize(ctx, items)
	if err != nil {
		slog.Error("compress working failed", "session_id", m.SessionId, "error", err)
		return
	}

	newItem.SessionID = m.SessionId
	newItem.ID = strconv.FormatInt(time.Now().UnixNano(), 10)
	now := core.Now()
	newItem.CreatedAt = &now

	_, err = m.Memories[types.Episodic].Add(*newItem)
	if err != nil {
		slog.Error("compress: add episodic failed", "session_id", m.SessionId, "error", err)
		return
	}

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	wm.RemoveItems(ids)
	slog.Info(fmt.Sprintf("Memory compression complete[Working -> Episodic]: "+
		"Compressing %d working memories in total", len(items)))
}

// Search 语义检索，支持跨类型检索
func (m *Manager) Search(query string, memoryTypes []types.MemoryType, opts types.SearchOptions) ([]types.MemoryItem, error) {
	allItems := make([]types.MemoryItem, 0)
	for _, mt := range memoryTypes {
		mem, ok := m.Memories[mt]
		if !ok {
			continue
		}
		items, err := mem.Search(query, opts)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", mt, err)
		}
		allItems = append(allItems, items...)
	}
	return allItems, nil
}
