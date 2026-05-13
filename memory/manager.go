package memory

import (
	"awesome-agent/core"
	"awesome-agent/memory/store"
	"awesome-agent/memory/store/impl"
	"awesome-agent/memory/types"
	"context"
	"fmt"
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
}

func NewManager(
	config core.MemoryConfig,
	sessionId string,
	enableWorking bool,
	enableEpisodic bool,
	enableSemantic bool,
	enablePerceptual bool,
	vectorStore store.VectorStore,
	structuredStore store.StructuredStore,
	embeddingService store.EmbeddingService,
	graphStore store.GraphStore,
) (*Manager, error) {
	m := &Manager{
		config: config,

		SessionId:        sessionId,
		EnableWorking:    enableWorking,
		EnableEpisodic:   enableEpisodic,
		EnableSemantic:   enableSemantic,
		EnablePerceptual: enablePerceptual,
		vectorStore:      vectorStore,
		structuredStore:  structuredStore,
		embeddingService: embeddingService,
		graphStore:       graphStore,
	}

	// 默认实现初始化
	if err := m.initDefaults(); err != nil {
		return nil, err
	}

	m.Memories = make(map[types.MemoryType]types.Memory)
	if enableWorking {
		m.Memories[types.Working] = types.NewWorkingMemory(1024, 360)
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

	return m, nil
}

// initDefaults 默认实现初始化: 仅当用户没有传入自定义实现且开启的记忆模块需要依赖该实现时
func (m *Manager) initDefaults() error {
	cfg := m.config

	if m.structuredStore == nil && m.EnableEpisodic {
		opts := cfg.Structured.Options
		switch cfg.Structured.Driver {
		case "sqlite":
			m.structuredStore = impl.NewSQLiteStore(opts)
		default:
			return fmt.Errorf("unsupported structure driver: %s", cfg.Structured.Driver)
		}
		err := m.structuredStore.Init(context.Background())
		if err != nil {
			return err
		}
	}

	if m.embeddingService == nil && (m.EnableEpisodic || m.EnableSemantic) {
		opts := cfg.Embedding.Options
		switch cfg.Embedding.Driver {
		case "openai":
			m.embeddingService = impl.NewOpenAIEmbedding(opts)
		default:
			return fmt.Errorf("unsupported embedding driver: %s", cfg.Embedding.Driver)
		}
	}

	if m.vectorStore == nil && (m.EnableEpisodic || m.EnableSemantic) {
		opts := cfg.VectorStore.Options
		switch cfg.VectorStore.Driver {
		case "qdrant":
			m.vectorStore = impl.NewQdrantStore(opts)
		default:
			return fmt.Errorf("unsupported vector_store driver: %s", cfg.VectorStore.Driver)
		}
		err := m.vectorStore.Init(context.Background(), "episodes", m.embeddingService.Dimension())
		if err != nil {
			return fmt.Errorf("init vector_store episodic: %w", err)
		}
		err = m.vectorStore.Init(context.Background(), "semantic", m.embeddingService.Dimension())
		if err != nil {
			return fmt.Errorf("init vector_store semantic: %w", err)
		}
	}

	if m.graphStore == nil && m.EnableSemantic {
		opts := cfg.Graph.Options
		switch cfg.Graph.Driver {
		case "neo4j":
			m.graphStore = impl.NewNeo4jStore(opts)
		default:
			return fmt.Errorf("unsupported graph driver: %s", cfg.Graph.Driver)
		}
		err := m.graphStore.Init(context.Background())
		if err != nil {
			return fmt.Errorf("init graph store neo4j: %w", err)
		}
	}

	return nil
}

// Add 添加一条记忆到指定类型
func (m *Manager) Add(mt types.MemoryType, item types.MemoryItem) (string, error) {
	mem, ok := m.Memories[mt]
	if !ok {
		return "", fmt.Errorf("memory type %s not enabled", mt)
	}
	return mem.Add(item)
}

// Retrieve 结构化检索
func (m *Manager) Retrieve(mt types.MemoryType, query string, limit int64, metadata map[string]string) ([]types.MemoryItem, error) {
	mem, ok := m.Memories[mt]
	if !ok {
		return nil, fmt.Errorf("memory type %s not enabled", mt)
	}
	return mem.Retrieve(query, limit, metadata)
}

// Search 语义检索，支持跨类型并发检索
// 实现了 Searchable 接口的类型走语义检索，否则降级到 Retrieve
func (m *Manager) Search(query string, memoryTypes []types.MemoryType, opts types.SearchOptions) ([]types.MemoryItem, error) {
	allItems := make([]types.MemoryItem, 0)
	for _, mt := range memoryTypes {
		mem, ok := m.Memories[mt]
		if !ok {
			continue
		}
		if s, ok := mem.(types.Searchable); ok {
			items, err := s.Search(query, opts)
			if err != nil {
				return nil, fmt.Errorf("search %s: %w", mt, err)
			}
			allItems = append(allItems, items...)
		} else {
			items, err := mem.Retrieve(query, opts.Limit, opts.Filter)
			if err != nil {
				return nil, fmt.Errorf("retrieve %s: %w", mt, err)
			}
			allItems = append(allItems, items...)
		}
	}
	return allItems, nil
}

// Forget 调用遗忘策略
func (m *Manager) Forget(mt types.MemoryType, strategy types.ForgotStrategy, threshold float64, maxAgeDays int64) (int, error) {
	mem, ok := m.Memories[mt]
	if !ok {
		return 0, fmt.Errorf("memory type %s not enabled", mt)
	}
	f, ok := mem.(types.Forgettable)
	if !ok {
		return 0, fmt.Errorf("memory type %s does not support forget", mt)
	}
	return f.Forget(strategy, threshold, maxAgeDays)
}

// Delete 按 ID 删除记忆
func (m *Manager) Delete(mt types.MemoryType, id string) error {
	mem, ok := m.Memories[mt]
	if !ok {
		return fmt.Errorf("memory type %s not enabled", mt)
	}
	return mem.Delete(id)
}

// Status 获取指定类型的统计信息
func (m *Manager) Status(mt types.MemoryType) (types.MemoryStatus, error) {
	mem, ok := m.Memories[mt]
	if !ok {
		return types.MemoryStatus{}, fmt.Errorf("memory type %s not enabled", mt)
	}
	return mem.Status(), nil
}
