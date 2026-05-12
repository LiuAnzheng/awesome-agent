<div align="center">

<img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go" alt="Go">
<img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License">
<img src="https://img.shields.io/badge/status-active-success.svg" alt="Status">

# AwesomeAgent

**面向 LLM Agent 的认知记忆框架**

<br>
</div>

AwesomeAgent 是一个 Go 实现的 LLM Agent 框架，核心特色是**多类型认知记忆系统** — 工作记忆、情景记忆、语义记忆 — 支持向量检索、知识图谱、时间衰减评分和可插拔存储后端。

---

## 架构

```
AwesomeAgent
├── core/             LLM 客户端 · 消息管理 · 配置加载 · 时区工具
├── agents/           ReAct Agent（推理-行动循环）
├── tools/            工具系统（注册 · 执行 · 链式调用）
│   └── builtins/     MemoryTool · WebSearch · RAG
├── memory/           记忆模块
│   ├── manager.go    门面：Add / Search / Retrieve / Forget / Delete / Status
│   ├── types/        四种记忆类型 + 接口层级
│   │   ├── working.go       工作记忆（FIFO + 过期淘汰）
│   │   ├── episodic.go      情景记忆（SQLite + Qdrant，时间衰减评分）
│   │   ├── semantic.go      语义记忆（Neo4j + Qdrant，图增强评分）
│   │   └── perceptual.go    感知记忆（多模态，规划中）
│   └── store/        存储抽象层
│       ├── store.go          接口：Structured · Vector · Embedding · Graph
│       └── impl/             SQLite · Qdrant · OpenAIEmbedding · Neo4j
└── app-config.yaml   Driver + Options 配置
```

---

## 三种记忆

| 类型 | 存储 | 检索公式 | 适用场景 |
|------|------|----------|----------|
| **Working** | 内存 (FIFO) | 关键词 + 过期淘汰 | 当前对话上下文 |
| **Episodic** | SQLite + Qdrant | `(向量x0.8 + 时间x0.2) x (0.8 + 重要性x0.4)` | 事件、观察、行动、结果 |
| **Semantic** | Neo4j + Qdrant | `(向量x0.7 + 图x0.3) x (0.8 + 重要性x0.4)` | 知识、规则、概念、模式 |

---

## 评分公式

**情景记忆（时间衰减加权）：**

```
Score = (vectorSim x 0.8 + e^(-lambda x hours) x 0.2) x (0.8 + importance x 0.4)
             语义 80%              时间近因 20%           重要性调节
```

- vectorSim [0..1]: Qdrant 余弦相似度
- timeRecency [0..1]: 指数衰减，lambda=0.05（半衰期约 14 小时）
- importance [0..1]: 用户设定权重

**语义记忆（图结构增强）：**

```
Score = (vectorSim x 0.7 + graphSim x 0.3) x (0.8 + importance x 0.4)
             语义 70%         图共现率 30%          重要性调节
```

- graphSim [0..1]: 邻居节点在候选集中的共现比例，分数越高说明该节点是知识枢纽

值域：[0, 1.2]，importance 越高的记忆排名越靠前。

---

## 快速开始

### 依赖

```bash
# Qdrant（向量数据库，情景记忆和语义记忆共用）
docker run -p 6333:6333 qdrant/qdrant

# Neo4j（图数据库，语义记忆专用）
docker run -p 7474:7474 -p 7687:7687 -e NEO4J_AUTH=neo4j/neo4j neo4j:5
```

### 配置

```yaml
# app-config.yaml
awesome-agent:
  llm:
    model_id: "deepseek-v4-pro"
    provider: "deepseek"
    api_key: ${LLM_API_KEY}
    base_url: ${LLM_BASE_URL}
  memory:
    structure:
      driver: "sqlite"
      options:
        db_path: "./data/memory.db"
    embedding:
      driver: "openai"
      options:
        model_id: "text-embedding-v4"
        base_url: ${EMBEDDING_BASE_URL}
        api_key: ${EMBEDDING_API_KEY}
        dimension: 1024
        batch_size: 32
    vector_store:
      driver: "qdrant"
      options: { host: "127.0.0.1", port: 6333 }
    graph:
      driver: "neo4j"
      options:
        url: "http://127.0.0.1:7474"
        db: "neo4j"
        username: "neo4j"
        password: ${NEO4J_PASSWORD}
```

### 代码

```go
import (
    "awesome-agent/core"
    "awesome-agent/tools/builtins"
    "awesome-agent/memory/types"
)

// 1. 加载配置
core.LoadConfig("app-config.yaml")

// 2. 创建 MemoryTool
memTool, _ := builtins.NewMemoryTool(
    "session_001",              // 业务 Session ID
    core.AppCfg.Memory,         // 配置（值传递）
    []types.MemoryType{         // 启用的记忆类型
        types.Working,
        types.Episodic,
        types.Semantic,
    },
    nil, nil, nil, nil,         // 自定义 store（nil = 使用默认实现）
)

mgr := memTool.Manager

// 存储一条对话观察
mgr.Add(types.Episodic, types.MemoryItem{
    Content:    "用户在并发场景下报告登录模块 panic",
    Importance: 0.9,
    Metadata:   map[string]string{"event_type": "observation"},
})

// 存储一条技术知识
mgr.Add(types.Semantic, types.MemoryItem{
    Content:    "sync.Mutex 非可重入锁，同一 goroutine 重复 Lock 会导致 fatal",
    Importance: 0.8,
    Metadata:   map[string]string{"tags": "Go,concurrency,lock"},
})

// 跨类型语义搜索
items, _ := mgr.Search("登录并发问题",
    []types.MemoryType{types.Episodic, types.Semantic},
    types.SearchOptions{Limit: 5, MinImportance: 0.3},
)

// 检索当前会话的所有事件
items, _ = mgr.Retrieve(types.Episodic, "", 10,
    map[string]string{"session_id": "session_001"},
)

// 遗忘低重要性记忆
mgr.Forget(types.Episodic, types.ImportanceBased, 0.2, 0)

// 查看统计
status, _ := mgr.Status(types.Episodic)
```

---

## 接入 ReAct Agent

```go
import "awesome-agent/agents"

react := agents.NewReActAgent(
    "MyAgent", llmClient, core.AppCfg.AgentConfig,
    toolRegistry, 10, systemPrompt,
)

toolRegistry.RegisterTool(memTool)
answer, _ := react.Run("帮我看一下之前登录模块出过什么问题")
```

LLM 通过 MemoryTool 自动 recall 历史上下文、存档关键发现、遗忘噪音——完全由 SystemPrompt + Few-Shot Description 驱动。

---

## 接口层级

```
Memory              Add · Retrieve · Delete · Status
 ├── Searchable     Search(query, SearchOptions)
 └── Forgettable    Forget(strategy, threshold, maxAgeDays)

实现关系:
  WorkingMemory  ->  Memory
  EpisodicMemory ->  Memory + Searchable + Forgettable
  SemanticMemory ->  Memory + Searchable + Forgettable
```

---

## 扩展自定义存储驱动

MemoryConfig 采用 **Driver + Options** 模式，替换存储后端只需实现接口 + YAML 改一行：

```yaml
memory:
  structure:
    driver: "mysql"        # 内置: sqlite
    options:
      host: "127.0.0.1"
      port: 3306
      user: "root"
      database: "awesome_agent"
```

```go
// 实现 StructuredStore 接口 → Manager.initDefaults() 加一个 case
case "mysql":
    m.structuredStore = impl.NewMySQLStore(opts)
```

---

## 存储架构

```
情景记忆 (Episodic):
  SQLite  -- episodes 表、episode_relations 表
  Qdrant  -- "episodes" 集合（余弦距离）

语义记忆 (Semantic):
  Neo4j   -- 节点与边（知识图谱）
  Qdrant  -- "semantic" 集合（余弦距离）

工作记忆 (Working):
  内存切片 + sync.RWMutex
```

---

## 项目结构

```
.
├── agents/react.go           ReAct 推理-行动循环
├── core/
│   ├── agent.go              基础 Agent（消息历史管理）
│   ├── config.go             Viper + YAML + 环境变量展开
│   ├── llm.go                OpenAI 兼容 HTTP 客户端
│   ├── message.go            消息结构定义
│   └── time.go               东八区时间工具
├── memory/
│   ├── manager.go            Manager 门面（6 个统一方法）
│   ├── types/
│   │   ├── memory.go         接口层级 + MemoryItem
│   │   ├── working.go        工作记忆（FIFO + metadata 过滤）
│   │   ├── episodic.go       情景记忆（SQLite + Qdrant + 时间衰减）
│   │   ├── semantic.go       语义记忆（Neo4j + Qdrant + 图增强）
│   │   └── perceptual.go     感知记忆（规划中）
│   └── store/
│       ├── store.go           4 个抽象接口 + 数据结构
│       ├── embedding.go       EmbeddingService 接口
│       └── impl/              4 个内置驱动
│           ├── sqlite_store.go
│           ├── qdrant_store.go
│           ├── openai_embedding.go
│           └── neo4j_store.go
├── tools/
│   ├── tool.go               Tool 接口 + OpenAI Schema 转换
│   ├── registry.go           工具注册中心
│   ├── executor.go           工具执行器
│   └── builtins/
│       ├── memory_tool.go    记忆工具（6 actions, 18 params, Few-Shot）
│       ├── rag_tool.go       RAG 检索工具
│       └── web_search_tool.go Web 搜索工具
├── app-config.yaml           项目配置文件
├── go.mod
└── README.md
```

---

## License

MIT
