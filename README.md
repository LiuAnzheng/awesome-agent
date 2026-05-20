<div align="center">

<img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go" alt="Go">
<img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License">
<img src="https://img.shields.io/badge/status-active-success.svg" alt="Status">

# AwesomeAgent

**Golang轻量级Agent框架**

<br>
</div>

---

## 概述

AwesomeAgent 是一个 Go 实现的 LLM Agent 框架，围绕**多类型认知记忆系统**构建 —— 工作记忆、情景记忆、语义记忆 —— 支持向量检索、知识图谱、时间衰减评分和可插拔存储后端。框架提供完整的 ReAct 推理循环、工具系统（注册/执行/链式编排）、RAG 文档检索管线，以及基于 LLM 的记忆自动压缩机制。

### 核心能力

- **三种认知记忆** — Working（FIFO 内存）、Episodic（SQLite + Qdrant，时间衰减评分）、Semantic（Neo4j + Qdrant，图结构增强评分）
- **ReAct Agent** — 思考 → 行动 → 观察 推理循环，完整的消息历史管理
- **工具系统** — 统一 Tool 接口、注册中心、参数校验执行器、支持串行/并行编排的 Chain
- **记忆压缩** — Working → Episodic 自动压缩（容量触发 → LLM 摘要 → 批量写入）
- **RAG 管线** — 文档解析 → 智能切块 → 向量化 → Qdrant 存储 → 语义检索，SHA256 去重
- **可插拔存储** — Driver + Options 模式，替换驱动只需改一行 YAML

---

## 架构

```
┌──────────────────────────────────────────────────────────────┐
│                        ReAct Agent                           │
│  ┌─────────┐    ┌──────────┐    ┌──────────┐               │
│  │ Think   │───▶│   Act    │───▶│ Observe  │──▶ 循环 / 终止  │
│  └─────────┘    └──────────┘    └──────────┘               │
│                      │                                       │
│                      ▼                                       │
│  ┌───────────────────────────────────────────────────────┐  │
│  │                   Tool System                          │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐           │  │
│  │  │ Registry │  │ Executor │  │  Chain   │           │  │
│  │  └──────────┘  └──────────┘  └──────────┘           │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐           │  │
│  │  │ Memory   │  │   RAG    │  │WebSearch │           │  │
│  │  │  Tool    │  │   Tool   │  │   Tool   │           │  │
│  │  └────┬─────┘  └──────────┘  └──────────┘           │  │
│  └───────┼───────────────────────────────────────────────┘  │
└──────────┼──────────────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────────────────────┐
│                    Memory Manager                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ Working  │  │ Episodic │  │ Semantic │  │Perceptual│    │
│  │ Memory   │  │ Memory   │  │ Memory   │  │ (规划中)  │    │
│  │ (内存FIFO)│  │(SQLite+  │  │(Neo4j+  │  │          │    │
│  │          │  │ Qdrant)  │  │ Qdrant)  │  │          │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────────┘    │
│       │             │             │                           │
│       │       ┌─────▼─────┐       │                           │
│       └──────▶│Compressor │       │                           │
│               │(W→E, E→S) │       │                           │
│               └───────────┘       │                           │
└──────────────────────┼────────────┼──────────────────────────┘
                       │            │
                       ▼            ▼
┌──────────────────────────────────────────────────────────────┐
│                     Store Layer                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ SQLite   │  │ Qdrant   │  │ OpenAI   │  │  Neo4j   │    │
│  │ (结构化)  │  │ (向量)    │  │(Embedding)│  │  (图)    │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└──────────────────────────────────────────────────────────────┘
```

---

## 三种记忆

### 工作记忆 (Working Memory)

当前对话上下文的短期记忆，纯内存 FIFO 队列。

| 特性 | 说明 |
|------|------|
| 存储 | 内存切片 + sync.RWMutex |
| 淘汰策略 | FIFO（容量满时移除最早） + 过期淘汰（默认 360 分钟） |
| 检索方式 | 关键词匹配 + 时间倒序 |
| 压缩触发 | 容量达到 90% 时自动压缩为 Episodic Memory |
| 默认容量 | 1024 条 |

### 情景记忆 (Episodic Memory)

记录事件、观察、行动和结果的长期记忆，双存储保障。

| 特性 | 说明 |
|------|------|
| 结构化存储 | SQLite（episodes 表 + episode_relations 表） |
| 向量存储 | Qdrant（"episodes" 集合，余弦距离） |
| 事件类型 | observation / thought / action / result |
| 关系类型 | before / after / caused_by / related_to |
| 检索公式 | `Score = (vectorSim × 0.8 + timeRecency × 0.2) × (0.8 + importance × 0.4)` |

**时间衰减函数：** `timeRecency = max(0.1, e^(-0.05 × hoursAgo))`，半衰期约 14 小时。

### 语义记忆 (Semantic Memory)

存储知识、规则、概念和模式，利用知识图谱增强检索。

| 特性 | 说明 |
|------|------|
| 图存储 | Neo4j（节点与边，知识图谱） |
| 向量存储 | Qdrant（"semantic" 集合，余弦距离） |
| 去重策略 | 向量相似度 > 0.9 时合并，更新重要性取 max |
| 检索公式 | `Score = (vectorSim × 0.7 + graphSim × 0.3) × (0.8 + importance × 0.4)` |

**图增强评分：** `graphSim` = 邻居节点在候选集中的共现比例，值越高说明该节点是知识枢纽。

### 评分值域

三种记忆的最终评分范围均为 **[0, 1.2]**，importance 越高的记忆排名越靠前。

---

## 记忆压缩

当 Working Memory 达到 90% 容量时自动触发异步压缩：

```
Working Memory (N items)
       │
       ▼ TakeSnapshot(30)
       │
       ▼ LLM Summary (Compressor)
       │  ┌─ 提取核心线索：意图 → 关键发现 → 结果
       │  ├─ 过滤过程噪声：工具调用日志、中间推理步骤
       │  ├─ 保留关键细节：代码路径、错误信息、参数值
       │  └─ 输出：content(2-5句) + summary(1句) + importance(max×0.9)
       │
       ▼
Episodic Memory (1 compacted item)
       │
       ▼ RemoveItems(olds) ← 清理已压缩的 Working 条目
```

压缩器通过 **强制 tool_choice** 调用 LLM 的 `compress_memory` function，确保输出格式严格受控。

**扩展压缩路径：** `Episodic2SemanticCompressor` 已预留接口，用于将多条情景记忆提炼为语义知识。

---

## 工具系统

### Tool 接口

```go
type Tool interface {
    Name() string
    Description() string
    Run(parameters map[string]interface{}) (string, error)
    Parameters() []ToolParameter
}
```

任何实现该接口的类型即可注册为工具，LLM 通过 OpenAI Function Calling 协议调用。

### 注册中心 (ToolRegistry)

- `Register(t Tool)` — 注册工具（同名覆盖警告）
- `Tool(name string)` — 按名查找
- `List() []Tool` — 列出全部工具
- `ToOpenAISchemas()` — 转换为 OpenAI 兼容的 function schema 列表

### 执行器 (ToolExecutor)

- 解析 LLM 返回的 `ToolCall[]`
- 校验必填参数 + 类型检查 + 填充默认值
- 自动删除多余参数
- 支持的类型：`string / integer / number / boolean / object / array`

### 工具链 (Chain)

将多个工具编排为有序步骤，对外暴露为单一 Tool。LLM 看到的是一个工具，实际执行时串行/并行运行多个子工具。

```go
chain := tools.MustNewChain(
    "deep_research",
    "Research a topic and save findings",
    []tools.ToolParameter{
        {Name: "topic", Type: tools.ParamString, Required: true},
        {Name: "save", Type: tools.ParamBoolean, Default: true},
    },
    []tools.ChainStep{
        {ToolName: "WebSearch",   StoreAs: "s1", ParamMap: map[string]string{"query": "$input.topic"}},
        {ToolName: "WebSearch",   StoreAs: "s2", ParamMap: map[string]string{"query": "$steps.s1 深度分析"}, Parallel: true},
        {ToolName: "WebSearch",   StoreAs: "s3", ParamMap: map[string]string{"query": "$steps.s1 相关案例"}, Parallel: true},
        {ToolName: "memory",      StoreAs: "",   ParamMap: map[string]string{
            "action": "add", "content": "$steps.s1\n$steps.s2\n$steps.s3", "importance": "0.7",
        }},
    },
    registry,
)
```

**占位符体系：**
- `$input.xxx` — 引用 Chain 入参
- `$steps.xxx` — 引用前置步骤输出

**约束校验：** 并行组内禁止相互引用，禁止自引用，未定义的步骤引用编译期报错。

### 内置工具

| 工具 | Action | 说明 |
|------|--------|------|
| **MemoryTool** | `add` / `search` | 记忆存取，多 session 管理，自动初始化存储驱动 |
| **RAGTool** | `search` / `list` / `delete` / `status` | 文档知识库检索，支持引用格式 |
| **WebSearchTool** | — | Tavily + SerpAPI 双引擎网络搜索 |

---

## RAG 管线

文档摄入全流程，支持多种格式解析和智能切块。

```
文件 / 文本
    │
    ▼ Parser (解析)
    │  ┌─ NativeParser: .txt / .md / .go / .py / .yaml / .json ...
    │  └─ 可扩展：实现 Parser 接口 → Registry.Register()
    │
    ▼ Chunker (切块)
    │  ┌─ RecursiveChunker: 段落 → 句子 → 词 → 字符 递归降级
    │  ├─ 可配置 chunk_size / chunk_overlap
    │  └─ 可扩展：实现 Chunker 接口
    │
    ▼ Embedding (向量化)
    │  ├─ OpenAI 兼容 API，分片批量请求
    │  └─ 网络错误自动重试（4xx 不可重试除外）
    │
    ▼ Store (存储)
    │  ├─ SQLite: rag_documents + rag_chunks 表，外键级联
    │  └─ Qdrant: 向量点，附 doc_id / doc_name / chunk_index
    │
    ▼ 去重：SHA256 内容哈希，重复文档直接返回已有 ID
```

**检索结果引用格式：** LLM 被要求为每个事实标注 `[N]`，并在回答末尾附参考文献列表（含 doc_name、chunk_index、score）。

---

## 存储架构

所有存储驱动采用 **Driver + Options** 模式，替换实现只需改 YAML 一行 + 代码加一个 case。

### 接口层级

```go
// 结构化存储
StructuredStore   Save / Get / Query / Delete / BatchDelete / Exec / Init / Close

// 向量存储
VectorStore       Upsert / BatchUpsert / Search / Delete / Init / Close

// 嵌入服务
EmbeddingService  Embed / EmbedBatch / Dimension

// 图存储
GraphStore        CreateNode / UpdateNode / GetNode / DeleteNode /
                  CreateRelation / GetNeighbors / GetNeighborIDs / Query / Init / Close
```

### 内置驱动

| 接口 | 驱动 | 关键特性 |
|------|------|----------|
| StructuredStore | **sqlite** | WAL 模式，外键约束，防注入校验 |
| VectorStore | **qdrant** | HTTP API，余弦距离，UUID v5 确定性 ID 映射 |
| EmbeddingService | **openai** | OpenAI 兼容 API，分片批量，指数退避重试 |
| GraphStore | **neo4j** | Cypher 事务 API，MERGE 幂等创建，DETACH DELETE |

---

## 快速开始

### 环境依赖

```bash
# Qdrant - 向量数据库（情景记忆和语义记忆共用）
docker run -d -p 6333:6333 qdrant/qdrant

# Neo4j - 图数据库（语义记忆专用）
docker run -d -p 7474:7474 -p 7687:7687 \
  -e NEO4J_AUTH=neo4j/neo4j neo4j:5
```

### 安装

```bash
go get awesome-agent
```

### 配置文件

```yaml
# app-config.yaml
awesome-agent:
  llm:
    model_id: "deepseek-v4-pro"
    provider: "deepseek"
    api_key: ${LLM_API_KEY}
    base_url: ${LLM_BASE_URL}
  agent:
    temperature: 0.7
    max_tokens: 4096
    top_p: 1.0
  memory:
    structure:
      driver: "sqlite"
      options:
        db_path: "./data/memory.db"
    embedding:
      driver: "openai"
      options:
        model_id: "text-embedding-v4"
        api_key: ${EMBEDDING_API_KEY}
        base_url: ${EMBEDDING_BASE_URL}
        dimension: 1024
        batch_size: 32
    vector_store:
      driver: "qdrant"
      options:
        host: "127.0.0.1"
        port: 6333
    graph:
      driver: "neo4j"
      options:
        url: "http://127.0.0.1:7474"
        db: "neo4j"
        username: "neo4j"
        password: ${NEO4J_PASSWORD}
  rag:
    max_doc_size: 52428800   # 50MB
    collection: "rag"
```

### 基础用法

```go
package main

import (
    "awesome-agent/agents"
    "awesome-agent/core"
    "awesome-agent/tools"
    "awesome-agent/tools/builtins"
    "awesome-agent/memory/types"
)

func main() {
    // 1. 加载配置
    core.LoadConfig("app-config.yaml")

    // 2. 创建 LLM 客户端
    llm, _ := core.NewAwesomeLLM(core.AppCfg.LLMConfig, core.AppCfg.AgentConfig)

    // 3. 创建工具注册中心
    registry := tools.NewToolRegistry()

    // 4. 创建并注册 MemoryTool
    memTool, _ := builtins.NewMemoryTool(
        core.AppCfg,
        []types.MemoryType{types.Working, types.Episodic, types.Semantic},
        nil, nil, nil, nil, // nil = 使用默认驱动
    )
    registry.Register(memTool)

    // 5. 添加 session（记忆的隔离单元）
    if mt, ok := memTool.(*builtins.MemoryTool); ok {
        mt.AddSession("session_001")
    }

    // 6. 创建 ReAct Agent
    agent := agents.NewReActAgent(
        "MyAgent", llm, core.AppCfg.AgentConfig,
        registry, 10, "", // 空 prompt = 使用默认 SystemPrompt
    )

    // 7. 运行
    answer, _ := agent.Run(context.Background(), "帮我查一下之前登录模块出过什么问题")
    println(answer)
}
```

### 直接使用记忆管理器

```go
// 创建 Manager（MemoryTool 内部也是用它）
mgr, _ := memory.NewManager(
    core.AppCfg, "session_001",
    true,  // Working
    true,  // Episodic
    true,  // Semantic
    false, // Perceptual
    nil, nil, nil, nil, // nil = 默认驱动
    nil,  // LLM（不启用压缩时可为 nil）
)

// 存储一条观察
mgr.Add(types.MemoryItem{
    Content:    "用户在并发场景下报告登录模块 panic",
    Importance: 0.9,
    Metadata:   map[string]string{"event_type": "observation"},
})

// 存储一条技术知识
mgr.Add(types.MemoryItem{
    Content:    "sync.Mutex 非可重入锁，同一 goroutine 重复 Lock 会导致 fatal",
    Importance: 0.8,
    Metadata:   map[string]string{"tags": "Go,concurrency,lock"},
})

// 跨类型语义搜索
items, _ := mgr.Search("登录并发问题",
    []types.MemoryType{types.Episodic, types.Semantic},
    types.SearchOptions{Limit: 5, MinImportance: 0.3},
)

// 遗忘低重要性记忆
mgr.Forget(types.Episodic, types.ImportanceBased, 0.2, 0)

// 查看统计
status, _ := mgr.Status(types.Episodic)
fmt.Printf("共 %d 条情景记忆\n", status.Count)
```

### RAG 文档摄入

```go
ragTool, _ := builtins.NewRAGTool(embedSvc, vectorStore, docStore, core.AppCfg)
rt := ragTool.(*builtins.RAGTool)

// 摄入文档
result, err := rt.Ingest(context.Background(), "./docs/系统设计文档.md", "")
fmt.Printf("摄入完成: %s, %d chunks\n", result.DocID, result.ChunkCount)

// 重复摄入自动跳过（SHA256 去重）
result2, _ := rt.Ingest(context.Background(), "./docs/系统设计文档.md", "")
fmt.Printf("去重: %v\n", result2.Duplicate) // true
```

---

## 项目结构

```
AwesomeAgent/
├── core/                           LLM 客户端 · 消息管理 · 配置加载 · 时区工具
│   ├── agent.go                    BaseAgent — 消息历史管理
│   ├── config.go                   Viper + YAML + ${ENV} 环境变量展开
│   ├── llm.go                      OpenAI 兼容 HTTP 客户端（同步 + SSE 流式）
│   ├── message.go                  消息结构定义 + ToolCall
│   └── time.go                     东八区时间（Asia/Shanghai）
│
├── agents/
│   └── react.go                    ReAct Agent — Think → Act → Observe 循环
│
├── tools/                          工具系统
│   ├── tool.go                     Tool 接口 + OpenAI Function Calling Schema 转换
│   ├── registry.go                 工具注册中心
│   ├── executor.go                 工具执行器（参数校验 + 类型检查 + 默认值填充）
│   ├── chain.go                    工具链编排（串行/并行 + $input/$steps 占位符）
│   └── builtins/
│       ├── memory_tool.go          记忆工具（add / search + 多 session 管理）
│       ├── rag_tool.go             RAG 检索工具（search / list / delete / status）
│       └── web_search_tool.go      Web 搜索（Tavily + SerpAPI 双引擎 fallback）
│
├── memory/                         认知记忆模块
│   ├── manager.go                  Manager 门面 — Add / Search / Forget / Status
│   ├── types/                      记忆类型定义
│   │   ├── memory.go               接口层级 + MemoryItem + SearchOptions
│   │   ├── working.go              工作记忆（FIFO + 过期淘汰 + 压缩快照）
│   │   ├── episodic.go             情景记忆（SQLite + Qdrant + 时间衰减评分）
│   │   ├── semantic.go             语义记忆（Neo4j + Qdrant + 图增强评分）
│   │   ├── perceptual.go           感知记忆（多模态，规划中）
│   │   └── compressor.go           记忆压缩器（Working→Episodic + Episodic→Semantic）
│   ├── store/                      存储抽象层
│   │   ├── store.go                 4 个核心接口 + 通用数据结构
│   │   ├── embedding.go             EmbeddingService 接口
│   │   └── impl/                    4 个内置驱动实现
│   │       ├── sqlite_store.go       SQLite（WAL + 防注入 + 复杂类型序列化）
│   │       ├── qdrant_store.go       Qdrant（UUID v5 确定性 ID 映射）
│   │       ├── openai_embedding.go   OpenAI Embedding（分片批量 + 指数退避重试）
│   │       └── neo4j_store.go        Neo4j（Cypher 事务 API + token 校验）
│   └── rag/
│       └── ingestion/               RAG 文档摄入管线
│           ├── pipeline.go           主流程（解析→切块→向量化→存储 + SHA256 去重）
│           ├── parser/               文档解析器（原生解析器 + 注册中心）
│           └── chunker/              智能切块器（递归降级分隔符切分）
│
├── mcp/
│   ├── client.go                   MCP 客户端
│   └── server.go                   MCP 服务端
│
├── app-config.yaml                 项目配置文件
├── go.mod
└── README.md
```

---

## 接口层级

```
Memory ◀─────────────────────────── 基础接口
 ├── Add(item) → (id, error)
 ├── Search(query, opts) → ([]MemoryItem, error)
 ├── Delete(id) → error
 └── Status() → MemoryStatus

实现关系:
  WorkingMemory  →  Memory                          (内存 FIFO)
  EpisodicMemory →  Memory + Searchable + Forgettable (SQLite + Qdrant)
  SemanticMemory →  Memory + Searchable + Forgettable (Neo4j + Qdrant)
```

**Searchable** 提供语义检索能力，**Forgettable** 支持基于策略的遗忘（重要性阈值 / 时间阈值）。

---

## 扩展指南

### 替换存储驱动

```yaml
# 将结构化存储从 SQLite 替换为 MySQL
memory:
  structure:
    driver: "mysql"
    options:
      host: "127.0.0.1"
      port: 3306
      user: "root"
      database: "awesome_agent"
```

```go
// 在 store/impl/ 下实现 StructuredStore 接口，然后在 memory_tool.go initDefaults() 加 case:
case "mysql":
    m.structuredStore = impl.NewMySQLStore(opts)
```

### 添加自定义工具

```go
type MyTool struct{}

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "我的自定义工具" }
func (t *MyTool) Parameters() []tools.ToolParameter {
    return []tools.ToolParameter{
        {Name: "input", Type: tools.ParamString, Required: true, Description: "输入参数"},
    }
}
func (t *MyTool) Run(params map[string]interface{}) (string, error) {
    input := params["input"].(string)
    return "处理结果: " + input, nil
}

registry.Register(&MyTool{})
```

### 添加新的记忆类型

```go
// 1. 在 memory/types/ 下实现 Memory 接口
type CustomMemory struct { ... }
func (c *CustomMemory) Add(item MemoryItem) (string, error) { ... }
func (c *CustomMemory) Search(query string, opts SearchOptions) ([]MemoryItem, error) { ... }
// ...

// 2. 在 Manager 中注册
m.Memories[types.MemoryType("custom")] = NewCustomMemory(...)
```

### 添加新的压缩路径

```go
// 实现 Compressor 接口
type Semantic2EpisodicCompressor struct { llm core.LLMInterface }
func (s *Semantic2EpisodicCompressor) Summarize(ctx context.Context, items []MemoryItem) (*MemoryItem, error) { ... }
func (s *Semantic2EpisodicCompressor) SourceType() MemoryType { return types.Semantic }
func (s *Semantic2EpisodicCompressor) TargetType() MemoryType { return types.Episodic }

// 在 Manager 中挂载
m.compressor = append(m.compressor, NewSemantic2EpisodicCompressor(llm))
```

---

## 配置参考

| 路径 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `llm.model_id` | string | `"gpt-5.4"` | 模型 ID |
| `llm.provider` | string | `"openai"` | 提供商标识 |
| `llm.api_key` | string | — | API 密钥（支持 `${ENV}`） |
| `llm.base_url` | string | `"https://api.openai.com/"` | API 端点 |
| `agent.temperature` | float64 | `0.7` | 采样温度 |
| `agent.max_tokens` | int64 | `1024` | 最大输出 token |
| `agent.top_p` | float64 | `1.0` | 核采样 |
| `memory.structure.driver` | string | `"sqlite"` | 结构化存储驱动 |
| `memory.embedding.driver` | string | `"openai"` | Embedding 驱动 |
| `memory.vector_store.driver` | string | `"qdrant"` | 向量存储驱动 |
| `memory.graph.driver` | string | `"neo4j"` | 图存储驱动 |
| `rag.max_doc_size` | int64 | `52428800` | 文档最大字节数 |
| `rag.collection` | string | `"rag"` | Qdrant 集合名 |

---

## 路线图

- [ ] Perceptual Memory（多模态感知记忆）
- [ ] Episodic → Semantic 压缩器实现
- [ ] 记忆检索结果 LLM 重排序
- [ ] 更多 Parser 格式支持（PDF、HTML、DOCX）
- [ ] 工具调用超时与重试
- [ ] Agent 并行工具调用
- [ ] 记忆可视化 Dashboard

---

## License

MIT
