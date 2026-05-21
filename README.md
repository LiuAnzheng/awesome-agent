<p align="center">
  <h1 align="center">🦾 AwesomeAgent</h1>
  <p align="center"><b>Go 语言 AI Agent 应用框架</b></p>
  <p align="center">
    <img src="https://img.shields.io/badge/go-1.25+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/LLM-OpenAI%20Compatible-412991?style=flat-square&logo=openai&logoColor=white" alt="LLM">
    <img src="https://img.shields.io/badge/DB-SQLite%20%7C%20Qdrant%20%7C%20Neo4j-003B57?style=flat-square" alt="Database">
    <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License">
  </p>
</p>

---

**AwesomeAgent** 是一个 Go 语言实现的 AI Agent 框架，提供构建 LLM 驱动智能体所需的核心基础设施。它将 **ReAct 推理循环**、**分层记忆系统**、**RAG 检索增强生成**、**可插拔工具系统** 四大能力整合为一个统一的运行时。

---

## 目录

- [核心设计](#核心设计)
- [快速开始](#快速开始)
- [ReAct Agent](#react-agent)
- [分层记忆系统](#分层记忆系统)
- [RAG 检索增强生成](#rag-检索增强生成)
- [工具系统](#工具系统)
- [LLM 客户端](#llm-客户端)
- [存储后端](#存储后端)
- [项目结构](#项目结构)

---

## 核心设计

```
┌─────────────────────────────────────────────────────┐
│                   ReAct Agent                        │
│   ┌─────────┐    ┌─────────┐    ┌───────────────┐  │
│   │ Reason  │───▶│  Act    │───▶│  Observe      │  │
│   │ (LLM)   │◀───│ (Tools) │    │  (Results)    │  │
│   └─────────┘    └─────────┘    └───────────────┘  │
│                         │                            │
│         ┌───────────────┼───────────────┐           │
│         ▼               ▼               ▼           │
│   ┌──────────┐  ┌────────────┐  ┌────────────┐     │
│   │ Memory   │  │ RAG Tool   │  │ Web Search │     │
│   │ Tool     │  │            │  │ Tool       │     │
│   └────┬─────┘  └─────┬──────┘  └─────┬──────┘     │
│        │              │               │             │
│   ┌────┴────┐    ┌────┴────┐    ┌─────┴──────┐     │
│   │Working  │    │ Parse   │    │  Tavily    │     │
│   │Episodic │    │ Chunk   │    │  SerpAPI   │     │
│   │Semantic │    │ Embed   │    └────────────┘     │
│   └─────────┘    │ Store   │                        │
│                  └─────────┘                        │
└─────────────────────────────────────────────────────┘
```

| 模块 | 职责 |
|:---|:---|
| `agents/` | ReAct 推理-行动 Agent |
| `core/` | 配置加载 · LLM 客户端 · 消息协议 |
| `memory/` | 记忆系统 · RAG 流水线 · 检索算法 |
| `tools/` | 工具接口 · 注册/执行 · 调用链编排 |

---

## 快速开始

### 前置依赖

| 服务 | 用途 | 必需 |
|:---|:---|:---:|
| Go 1.25+ | 编译运行 | ✓ |
| Qdrant | 向量存储 | ✓ |
| OpenAI 兼容 API | LLM + Embedding | ✓ |
| Neo4j | 语义记忆（图存储） | 可选 |

### 安装

```bash
git clone https://github.com/your-org/AwesomeAgent.git
cd AwesomeAgent
go mod download
```

### 配置

创建 `app-config.yaml`，敏感信息通过环境变量注入：

```yaml
awesome-agent:
  llm:
    model_id: deepseek-v4-pro
    provider: deepseek
    api_key: ${LLM_API_KEY}
    base_url: ${LLM_BASE_URL}
  rag:
    max_doc_size: 52428800          # 50 MB
    collection: rag
  memory:
    structure:
      driver: sqlite
      options:
        db_path: ./data/memory.db
    embedding:
      driver: openai
      options:
        model_id:  text-embedding-v4
        api_key:   ${EMBEDDING_API_KEY}
        base_url:  ${EMBEDDING_BASE_URL}
        dimension: 1024
        batch_size: 10
    vector_store:
      driver: qdrant
      options:
        host: 192.168.187.100
        port: 6333
    graph:
      driver: neo4j
      options:
        url:      http://192.168.187.100:7474
        db:       neo4j
        username: neo4j
        password: ${NEO4J_PASSWORD}
```

```bash
export LLM_API_KEY="sk-xxx"
export LLM_BASE_URL="https://api.deepseek.com/v1"
export EMBEDDING_API_KEY="sk-xxx"
export EMBEDDING_BASE_URL="https://api.deepseek.com/v1"
export NEO4J_PASSWORD="your-password"
```

### Hello World

```go
package main

import (
    "context"
    "awesome-agent/agents"
    "awesome-agent/core"
    "awesome-agent/memory/types"
    "awesome-agent/tools"
    "awesome-agent/tools/builtins"
)

func main() {
    core.LoadConfig("app-config.yaml")

    llm, _ := core.NewAwesomeLLM(core.AppCfg.LLMConfig, core.AppCfg.AgentConfig)
    registry := tools.NewToolRegistry()

    // 挂载记忆工具
    tool, _ := builtins.NewMemoryTool(core.AppCfg, types.AvailableMemoryTypes,
        nil, nil, nil, nil)
    mt := tool.(*builtins.MemoryTool)
    mt.AddSession("your-session-uuid")
    registry.Register(mt)

    // 创建 Agent 并运行
    agent := agents.NewReActAgent("my-agent", llm,
        core.AppCfg.AgentConfig, registry, 1024, "")
    answer, _ := agent.Run(context.Background(), "你好，请问你能帮我做什么？")
    println(answer)
}
```

---

## ReAct Agent

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  User    │ ──▶ │  LLM     │ ──▶ │  Tool    │ ──▶ │  LLM     │
│  Input   │     │  Think   │     │  Execute │     │  Answer  │
└──────────┘     └──────────┘     └──────────┘     └──────────┘
                      │                │                │
                      │  tool_calls    │   results      │  no tool_calls
                      │  + reasoning   │                │  → final answer
                      ▼                ▼                ▼
                  循环继续 ──────────────────────────── 结束
```

Agent 收到用户输入后，进入 **Reason → Act → Observe** 循环：

1. **Reason** — LLM 分析问题，将推理过程写入 `content`
2. **Act** — 如需外部信息，LLM 返回 `tool_calls` 调用工具
3. **Observe** — 工具执行结果反馈给 LLM，继续推理
4. **Answer** — LLM 判定信息充足，`content` 直接输出最终答案

### 配置参数

| 参数 | 类型 | 说明 |
|:---|:---|:---|
| `MaxSteps` | `int64` | 最大推理步数，防止无限循环 |
| `SystemPrompt` | `string` | 自定义系统提示词，为空时使用内置默认值 |
| `ToolRegistry` | `*ToolRegistry` | 工具注册器，自动转为 OpenAI Function Calling Schema |

---

## 分层记忆系统

受认知科学启发，实现 **Working → Episodic → Semantic** 三层递进式记忆架构。

```
┌──────────────────────────────────────────────────┐
│                  记忆层次                         │
│                                                  │
│  ┌────────────┐   压缩触发 (90% 容量)             │
│  │  Working   │──────────────────────┐           │
│  │  内存·BM25 │  LLM 摘要 → Episodic │           │
│  │  ~1024 条  │                      ▼           │
│  └────────────┘            ┌────────────┐        │
│   FIFO · 过期清理           │  Episodic  │        │
│                             │ SQLite+Qdrant       │
│  ┌────────────┐            │  向量+时间+重要性     │
│  │ Semantic   │            └────────────┘        │
│  │ Neo4j+Qdrant                                   │
│  │  知识图谱  │                                    │
│  └────────────┘                                   │
└──────────────────────────────────────────────────┘
```

### 三级记忆对比

| | Working | Episodic | Semantic |
|:---|:---|:---|:---|
| **生命周期** | 单会话 | 跨会话持久 | 永久 |
| **存储** | 内存 (Go slice) | SQLite + Qdrant | Neo4j + Qdrant |
| **检索算法** | BM25 关键词 | 向量相似度 × 时间衰减 × 重要性 | 向量搜索 + 图邻域重叠 |
| **容量** | 1024 条（可配） | 无上限 | 无上限 |
| **淘汰策略** | FIFO + TTL 过期 | 手动删除 | 手动删除 |

### 自动压缩

工作记忆达到 **90% 容量**时，自动触发 LLM 压缩流程：

1. 取最旧的 30 条记忆快照
2. LLM 摘要为一条结构化情景记忆（摘要 · 事件类型 · 重要性）
3. 情景记忆写入 SQLite + Qdrant
4. 清理工作记忆中已压缩条目

压缩过程使用 **CAS 原子锁**，防止并发重复压缩。

### BM25 检索

工作记忆基于 Lucene 标准的 **BM25** 算法实现关键词检索：

- 中文分词：`go-ego/gse`，区分 **索引切分**（宽口径）和 **查询切分**（精准模式）
- 倒排索引增量重建（仅在脏数据时触发）
- 可配置 `k1` 和 `b` 参数

---

## RAG 检索增强生成

### 摄入流水线

```
┌────────┐    ┌────────┐    ┌────────┐    ┌────────┐
│ Parse  │───▶│ Chunk  │───▶│ Embed  │───▶│ Store  │
└────────┘    └────────┘    └────────┘    └────────┘
 16 种格式      递归切分      批量+重试     SQLite+Qdrant
 统一 Markdown  512/50 默认   指数退避      SHA-256 去重
                                          事务性回滚
```

### 文档格式支持

> `txt` · `md` · `markdown` · `html` · `htm` · `csv` · `json` · `xml` · `log` · `yaml` · `yml` · `toml` · `ini` · `cfg` · `conf` · `env`

非 Markdown 格式（CSV、JSON、XML、HTML、YAML、TOML）自动转换为结构化 Markdown 后再分块，确保下游处理一致。

### 高级检索

| 特性 | 原理 | 效果 |
|:---|:---|:---|
| **MQE** | LLM 生成 3 个语义多样查询变体 | 提升召回覆盖率 |
| **HyDE** | LLM 生成假设性答案段落再向量化检索 | 缩小语义鸿沟 |
| **RRF** | 多查询结果融合重排序 (k=60) | 稳定排序质量 |

所有变体查询 **并行执行**（goroutine），最终通过 RRF 合并为 `topK` 条结果。

### 使用 RAG

```go
rt, _ := builtins.NewRAGTool(nil, nil, nil, core.AppCfg, true, true)
ragTool := rt.(*builtins.RAGTool)

// 摄入文档到知识库
ragTool.Ingest(context.Background(), "./docs/api-spec.md", "api-spec.md")

registry.Register(rt)
agent := agents.NewReActAgent("rag-agent", llm,
    core.AppCfg.AgentConfig, registry, 1024, "")
agent.Run(ctx, "API 鉴权规范是什么？")
```

---

## 工具系统

### 架构

```
┌─────────────────────────────────────────┐
│               Tool Interface             │
│  Name() · Description() · Run() · Parameters()
└────────────┬────────────────────────────┘
             │
    ┌────────┴────────┐
    │                 │
    ▼                 ▼
┌─────────┐    ┌────────────┐
│Registry │    │  Executor  │    ┌─────────────────┐
│ 注册/查找 │    │ 校验·执行   │    │     Chain       │
└─────────┘    └────────────┘    │ 串行/并行编排     │
                                 │ $input / $steps  │
                                 └─────────────────┘
```

### 内置工具

| 工具 | 操作 | 能力 |
|:---|:---|:---|
| **RAGTool** | `search` · `list` · `status` · `delete` | 文档知识库向量检索与来源引用 |
| **MemoryTool** | `add` · `search` | 跨会话持久记忆，多 Session 管理 |
| **WebSearchTool** | — | Tavily + SerpAPI 双后端故障切换 |

### 工具链 (Chain)

Chain 将多个工具编排为有序步骤，对外暴露为单一 Tool——LLM 无感知：

```go
chain := tools.MustNewChain(
    "deep-research",
    "先查本地知识库 → 再查记忆 → 最后搜索网络",
    []tools.ToolParameter{
        {Name: "query", Type: tools.ParamString, Required: true},
    },
    []tools.ChainStep{
        // 步骤 1+2 并行执行
        {ToolName: "RAG",   StoreAs: "rag", ParamMap: map[string]string{
            "action": `"search"`, "query": "$input.query",
        }, Parallel: true},
        {ToolName: "memory", StoreAs: "mem", ParamMap: map[string]string{
            "action": `"search"`, "query": "$input.query",
        }, Parallel: true},
        // 步骤 3 串行（可引用前面结果）
        {ToolName: "web_search", StoreAs: "web", ParamMap: map[string]string{
            "query": "$input.query",
        }},
    },
    registry,
)
```

**编译时安全校验**：构造阶段即检测并行步骤间的依赖冲突、StoreAs 重名、循环引用，违规直接 panic。

**数据流**：`$input.xxx` 引用 Chain 入参，`$steps.xxx` 引用前置步骤输出。

---

## LLM 客户端

`AwesomeLLM` — OpenAI 兼容协议客户端。

| 方法 | 模式 | 说明 |
|:---|:---|:---|
| `ChatComplete(ctx, msgs, tools, extra)` | 同步 | 标准请求-响应 |
| `ChatStream(ctx, msgs, tools, extra)` | SSE 流式 | 逐 token 推送 |

- 可配置 `BaseURL`，兼容 DeepSeek 等任意 OpenAI 接口协议的服务
- 完整支持 Function Calling（Tool Calling）
- 流式响应自动解析 `data:` 帧和 `[DONE]` 终止符

---

## 存储后端

所有组件通过接口抽象，驱动可插拔替换：

| 抽象层 | 接口 | 当前驱动 | 实现文件 |
|:---|:---|:---|:---|
| 结构化存储 | `StructuredStore` | SQLite | `store/impl/sqlite.go` |
| 向量存储 | `VectorStore` | Qdrant | `store/impl/qdrant.go` |
| 图存储 | `GraphStore` | Neo4j | `store/impl/neo4j.go` |
| Embedding | `EmbeddingService` | OpenAI 兼容 | `store/impl/openai_embedding.go` |

扩展新驱动只需实现对应接口，在 `app-config.yaml` 中切换 `driver` 字段即可。

---

## 项目结构

```
AwesomeAgent/
├── agents/
│   └── react.go                         # ReAct Agent — 推理-行动循环
├── core/
│   ├── config.go                        # Viper YAML 配置加载
│   ├── llm.go                           # OpenAI 兼容 LLM 客户端
│   ├── message.go                       # 消息类型（OpenAI 协议对齐）
│   ├── agent.go                         # Agent 基类
│   └── time.go                          # 时区工具
├── memory/
│   ├── manager.go                       # 记忆管理器 — 编排各类型
│   ├── types/
│   │   ├── types.go                     # 类型定义 & 常量
│   │   ├── working.go                   # 工作记忆 — 内存 BM25
│   │   ├── episodic.go                  # 情景记忆 — SQLite+Qdrant
│   │   ├── semantic.go                  # 语义记忆 — Neo4j+Qdrant
│   │   └── compressor.go               # LLM 压缩器 — Working→Episodic
│   ├── store/
│   │   ├── store.go                     # 存储接口定义
│   │   └── impl/
│   │       ├── sqlite.go                # SQLite 结构化存储
│   │       ├── qdrant.go               # Qdrant 向量存储
│   │       ├── neo4j.go                # Neo4j 图存储
│   │       └── openai_embedding.go     # OpenAI Embedding 服务
│   ├── rag/
│   │   ├── ingestion/
│   │   │   ├── pipeline.go             # 摄入流水线：Parse→Chunk→Embed→Store
│   │   │   └── parser/native.go        # 多格式文档解析器（16 种格式）
│   │   └── advanced_features/
│   │       └── recall.go               # MQE · HyDE · RRF 并行检索
│   └── retrieval/
│       ├── bm25.go                      # BM25 评分器 + 倒排索引
│       └── tokenizer.go                 # 中英文分词器（gse 封装）
├── tools/
│   ├── tool.go                          # Tool 接口 & OpenAI Schema 转换
│   ├── registry.go                      # 工具注册器
│   ├── executor.go                      # 工具执行器
│   ├── chain.go                         # 工具链编排器
│   └── builtins/
│       ├── rag_tool.go                  # RAG 文档知识库工具
│       ├── memory_tool.go              # 跨会话记忆工具
│       └── web_search_tool.go          # 网络搜索工具
├── mcp/                                 # MCP 协议（规划中）
├── knowledge_base/                      # 测试用示例文档
├── data/                                # 运行时数据（SQLite .db）
├── test/main/main.go                    # 示例入口
├── app-config.yaml                      # 配置文件
├── go.mod
└── README.md
```

---

## License

MIT
