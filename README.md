<p align="center">
  <h1 align="center">🤖 AwesomeAgent</h1>
  <p align="center"><strong>基于 ReAct 范式的 AI Agent 框架</strong></p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.25-00ADD8?style=flat&logo=go" alt="Go 1.25">
  <img src="https://img.shields.io/badge/LLM-OpenAI_Compatible-412991?style=flat&logo=openai" alt="OpenAI Compatible">
  <img src="https://img.shields.io/badge/storage-Qdrant-FF6F00?style=flat" alt="Qdrant">
  <img src="https://img.shields.io/badge/graph-Neo4j-4581C3?style=flat&logo=neo4j" alt="Neo4j">
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat" alt="License">
</p>

---

LLM 推理与工具调用循环 · 多层记忆系统 (Working → Episodic → Semantic) · RAG 文档检索 · 上下文构建管线 (GSSC)

---

## 📖 目录

- [架构总览](#-架构总览)
- [核心流程](#-核心流程)
  - [ReAct 推理循环](#1--react-推理循环)
  - [上下文构建管线 GSSC](#2--上下文构建管线--gssc)
  - [多层记忆系统](#3--多层记忆系统)
  - [RAG 文档检索](#4--rag-文档检索)
  - [工具系统](#5--工具系统)
  - [LLM 客户端](#6-llm-客户端)
  - [配置系统](#7--配置系统)
- [快速开始](#-快速开始)
- [模块依赖](#-模块依赖)
- [设计要点](#-设计要点)

---

## 🏗 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                        agents/react                         │
│                     ┌─────────────────┐                     │
│                     │   ReActAgent    │                     │
│                     │  think → act →  │                     │
│                     │   observe 循环  │                     │
│                     └───────┬─────────┘                     │
│           ┌─────────────────┼─────────────────┐             │
│           ▼                 ▼                  ▼             │
│  ┌────────────────┐ ┌──────────────┐ ┌──────────────────┐  │
│  │   ctx/gssc     │ │    tools     │ │     memory       │  │
│  │  Gatherer      │ │  ToolRegistry│ │   Manager        │  │
│  │  Selector      │ │  ToolExecutor│ │   ┌─Working(inmem)  │
│  │  Structurer    │ │  Chain       │ │   ├─Episodic(sqlite+qdrant)
│  └───────┬────────┘ │  ┌──────────┐│ │   ├─Semantic(neo4j+qdrant)
│          │          │  │ builtins ││ │   └─Perceptual(stub)
│          │          │  ├─Memory   ││ │                    │
│          │          │  ├─RAG      ││ │   store/impl       │
│          │          │  └─WebSearch││ │   ├─SQLite         │
│          │          │  └──────────┘│ │   ├─Qdrant         │
│          │          └──────────────┘ │   ├─Neo4j          │
│          │                           │   └─OpenAIEmbed    │
│          │                           └──────────────────┘  │
│          ▼                                                  │
│  ┌─────────────────────────────────────────────┐            │
│  │                  core                        │            │
│  │  BaseAgent  │  LLMInterface(OpenAI HTTP)     │            │
│  │  Message    │  Config(viper + YAML)          │            │
│  └─────────────────────────────────────────────┘            │
└─────────────────────────────────────────────────────────────┘
```

| 模块 | 职责 |
|:-----|:-----|
| `core/` | `BaseAgent` 基类、OpenAI 兼容 HTTP 客户端、多模态 `Message`、Viper 配置管理 |
| `agents/` | `ReActAgent` —— 推理-行动循环，串联 LLM / 工具 / 上下文 |
| `tools/` | `Tool` 接口定义 + `ToolRegistry` 注册中心 + `ToolExecutor` 执行器 + `Chain` 多步编排 |
| `tools/builtins/` | 内置工具：`MemoryTool`、`RAGTool`、`WebSearchTool` |
| `ctx/gssc/` | GSSC 上下文构建管线：`Gatherer` → `Selector` → `Structurer` → `ContextBuilder` |
| `memory/` | 多层级记忆管理器 + 记忆类型定义 (Working/Episodic/Semantic) |
| `memory/store/` | 存储接口抽象：`StructuredStore` / `VectorStore` / `GraphStore` / `EmbeddingService` |
| `memory/store/impl/` | 驱动实现：SQLite、Qdrant、Neo4j、OpenAI Embedding |
| `memory/retrieval/` | BM25 评分器、稀疏向量、中文分词器 (gse) |
| `memory/rag/` | 文档摄入管线 + 高级检索 (MQE 查询扩展、HyDE 假设文档、RRF 融合) |
| `mcp/` | MCP 协议桩（预留） |

---

## ⚙ 核心流程

### 1. 🧠 ReAct 推理循环

```
      ┌──────────┐      ┌──────────────┐      ┌──────────┐
      │  Build   │─────▶│   LLM Call   │─────▶│ ToolCall?│
      │ Context  │      │ (system+user)│      └────┬─────┘
      └──────────┘      └──────────────┘       是 │   否
                                                     │    │
      ┌──────────────────────────────────────────┐    │
      │  ToolExecutor.Execute(tool_calls)         │    │
      │  · 注入 _session_id 到工具参数            │    │
      │  · 逐工具执行并收集结果为 tool message    │◀───┘
      │  · 回到 Build Context 进入下一轮          │
      └──────────────────────────────────────────┘
                                                     │
                                          ┌──────────▼──────────┐
                                          │  return final answer │
                                          └─────────────────────┘
```

| 步骤 | 动作 |
|:----:|:-----|
| ① | 通过 GSSC 管线聚合系统指令、记忆检索、RAG 检索、历史消息 |
| ② | 向 LLM 发送 `[system, user]` 消息，携带工具 Function Calling Schema |
| ③ | LLM 返回 `tool_calls` → 自动注入 `_session_id` → 执行工具 → 收集结果 → 循环 |
| ④ | LLM 返回 `stop` → 返回最终答案 |

---

### 2. 📐 上下文构建管线 — GSSC

```
  ┌──────────┐     ┌──────────┐     ┌───────────┐     ┌──────────┐
  │ Gatherer │────▶│ Selector │────▶│ Structurer│────▶│  System  │
  │  收集     │     │  筛选     │     │  结构化    │     │  Prompt  │
  └──────────┘     └──────────┘     └───────────┘     └──────────┘
       │                │                 │
       ▼                ▼                 ▼
  ┌─────────┐    综合评分排序        按来源分节
  │· 系统指令│    ┌─────────────┐    ┌──────────────┐
  │· 记忆搜索│    │ relevance   │    │Role&Policies │
  │· RAG搜索│    │  × Jaccard  │    │Task          │
  │· 历史消息│    │ recency     │    │Evidence      │
  └─────────┘    │  × expDecay │    │Context       │
                 └─────────────┘    │Output        │
                                    └──────────────┘
```

**Gatherer** — 从四个来源收集 `ContextPacket`：

| 来源 | 说明 |
|:-----|:-----|
| `SystemInstructions` | Agent 的 System Prompt，相关性固定 1.0 |
| `Memory` | 调用 `MemoryTool.RunSearch()` 检索相关记忆 |
| `RAG` | 调用 `RAGTool.RunSearch()` 检索文档块 |
| `History` | 最近 32 条历史消息，相关性固定 0.6 |

**Selector** — 综合评分 = `relevance_weight × Jaccard 相似度` + `recency_weight × 指数衰减时新度`。语义记忆（抽象知识）跳过时新度衰减。在 token 预算内按分数从高到低截断。

**Structurer** — 将筛选后的 ContextPacket 按来源分配到结构化模板：

```
[Role & Policies]
<系统指令原文>

[Task]
<用户查询原文>

[Evidence]
<RAG 检索到的文档块 / 语义记忆>

[Context]
<工作记忆 / 情景记忆 / 历史消息>

[Output]
Please provide an accurate and well-founded answer...
```

---

### 3. 🧬 多层记忆系统

```
   Working (内存)                    Episodic (SQLite + Qdrant)           Semantic (Neo4j + Qdrant)
  ┌─────────────────┐   压缩触发    ┌─────────────────────────┐         ┌──────────────────────────┐
  │  BM25 关键词检索  │───(90%满)──▶│  向量检索 + 时新度评分    │         │  向量检索 + 图共现评分      │
  │  FIFO 淘汰        │             │  含因果关系网络           │         │  跨会话抽象知识图谱         │
  │  容量: 1024       │             │  score = (cos×0.8 +      │         │  score = (cos×0.7 +        │
  └─────────────────┘             │         recency×0.2) ×    │         │          graph×0.3) ×      │
                                   │         (0.8 + imp×0.4)  │         │          (0.8 + imp×0.4)  │
                                   └─────────────────────────┘         └──────────────────────────┘
```

<blockquote>
📌 <strong>记忆压缩流程</strong>：Working 达到 90% 容量 → CAS 加锁 → 取最早 30 条快照 → LLM 调用 <code>compress_memory</code> function → 输出叙事+摘要+重要性+事件类型 → 写入 Episodic → 删除源 Working 条目
</blockquote>

<blockquote>
💡 <strong>LLM 行为约束</strong>：MemoryTool 的 System Prompt 强制 LLM 每轮对话前 <code>search</code>、每轮对话后 <code>add</code>，跳过即永久遗忘。
</blockquote>

---

### 4. 📚 RAG 文档检索

#### 摄入管线

```
  文件                    分块                     向量化                   存储
┌──────┐   ┌──────────┐   ┌──────────────────┐   ┌──────────────┐   ┌──────────────┐
│ .txt │   │          │   │ RecursiveChunker  │   │              │   │  SQLite      │
│ .md  │──▶│ Parser   │──▶│ · 标题层级递归     │──▶│ OpenAI Embed │──▶│  (元数据)     │
│ .html│   │ Registry │   │ · 段落→行→字符降级  │   │ · 批量请求    │   │  Qdrant      │
│ .csv │   │ 16种格式  │   │ SemanticChunker   │   │ · 3次重试     │   │  (向量)       │
│ .json│   │ → Markdown│   │ · 语义断点检测     │   │ · 指数退避    │   └──────────────┘
│ .xml │   └──────────┘   │ · embedding回退    │   └──────────────┘
│ ...  │                  └──────────────────┘
└──────┘
```

| 阶段 | 关键细节 |
|:-----|:-----|
| 解析 | `NativeParser` 支持 16 种格式，结构化格式（JSON/YAML/XML）按层级渲染为 Markdown 标题树，HTML 转为 Markdown |
| 分块 | `RecursiveChunker`（按标题层级）/ `SemanticChunker`（句子 Embedding → 余弦相似度 → 自适应阈值断点） |
| 去重 | SHA256 内容哈希，入库前检查 `rag_documents.sha256` 索引 |
| 重试 | Embedding 批量请求 3 次指数退避（1s → 2s → 4s），4xx 错误不重试 |
| 容错 | Qdrant 写入失败时标记状态 `vector_pending` 保留数据，不级联回滚 |

#### 高级检索

```
   User Query
       │
       ├──── 原始查询 ─────────────┐
       ├──── MQE × 3 (LLM 扩展) ──┼── 5 路并行向量搜索 ──▶ RRF 融合排序 ──▶ Top-K 结果
       └──── HyDE (LLM 生成文档) ─┘                              (k=60)
```

| 特性 | 说明 |
|:-----|:-----|
| **MQE** | LLM 生成 3 个语义等价/互补的变体查询，扩充召回覆盖面 |
| **HyDE** | LLM 生成一份假设性百科文档，用文档向量搜索（文档↔文档 优于 查询↔文档） |
| **RRF** | Reciprocal Rank Fusion (k=60)，多路检索结果融合排序，不同来源的排名公平竞争 |

---

### 5. 🔧 工具系统

接口定义：

```go
type Tool interface {
    Name() string
    Description() string
    Run(params map[string]interface{}) (string, error)
    Parameters() []ToolParameter
}
```

通过 `ToolToOpenAISchema()` 自动转换为 OpenAI Function Calling JSON Schema。

#### 内置工具

| 工具 | 注册名 | 能力 | 数据流 |
|:-----|:-------|:-----|:-------|
| **MemoryTool** | `memory_tool` | `add` 写入记忆 / `search` 语义检索 | 多 Session 隔离，`_session_id` 自动路由 |
| **RAGTool** | `rag_tool` | `search` 向量检索 / `list` 浏览 / `delete` 删除 / `status` 查询 | 强制引用格式：`[N] doc_name, chunk N (score: X.XX)` |
| **WebSearchTool** | `web_search_tool` | Tavily / SerpAPI 双引擎网络搜索 | 自动降级：Tavily 失败 → SerpAPI |

#### 工具链 (Chain)

将多个工具调用封装为一个复合工具，对 LLM 暴露单一接口。

```
Chain: "综合调研"
  ├── Step 1: web_search  ──(storeAs: search_result)──┐
  ├── Step 2: memory_search ──(storeAs: memory)        │
  │                                                    │
  └── Step 3: LLM 分析                                  │
        prompt: "基于 $steps.search_result 和           │
                $steps.memory 给出结论"  ◀──────────────┘
```

| 模式 | 行为 | 约束 |
|:-----|:-----|:-----|
| **串行** | 按序执行，`$steps.xxx` 引用前面步骤的输出 | 只能引用已完成的步骤 |
| **并行** | 相邻 `Parallel=true` 步骤并发执行 | 禁止同组内互相引用、禁止自引用 |

---

### 6. 🔌 LLM 客户端

- 协议：OpenAI 兼容 Chat Completions API (`POST /chat/completions`)
- 模式：同步 `ChatComplete` + 流式 `ChatStream`（SSE 逐行解析）
- 适配：通过 `BaseURL` + `APIKey` 可接入任何 OpenAI 兼容服务
- 消息：支持文本 + 多模态（`image_url` / `input_audio` / `file` 类型的 `ContentPart` 数组）

---

### 7. 🗂 配置系统

`app-config.yaml` → Viper 加载 → 环境变量 `${VAR}` 展开 → 默认值兜底。

```
awesome-agent
├── llm              模型、API Key、BaseURL、采样参数
├── memory
│   ├── structure    结构化存储驱动 (sqlite)
│   ├── embedding    向量化服务驱动 (openai)
│   ├── vector_store 向量数据库驱动 (qdrant)
│   └── graph        图数据库驱动 (neo4j)
├── rag              文档大小上限、集合名
└── context          token 预算、保留比、相关性/时新度权重
```

<details>
<summary>📄 完整配置示例（点击展开）</summary>

```yaml
awesome-agent:
  llm:
    model_id: "qwen3.5-omni-plus-2026-03-15"
    provider: "dashscope"
    api_key: ${LLM_API_KEY}
    base_url: ${LLM_BASE_URL}
    max_tokens: 65535
    temperature: 0.7
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
        batch_size: 10
    vector_store:
      driver: "qdrant"
      options:
        host: "192.168.187.100"
        port: 6333
    graph:
      driver: "neo4j"
      options:
        url: "http://192.168.187.100:7474"
        db: "neo4j"
        username: "neo4j"
        password: ${NEO4J_PASSWORD}

  rag:
    max_doc_size: 52428800
    collection: "rag"

  context:
    max_tokens: 102400
    reserve_ratio: 0.2
    min_relevance: 0.1
    enable_compression: true
    recency_weight: 0.3
    relevance_weight: 0.7
```
</details>

所有存储后端使用统一的 **Driver 模式**：

```yaml
{backend}:
  driver: "驱动名"         # 选择实现
  options:                # 驱动专用参数，自由键值对
    key1: value1
    key2: value2
```

| 后端 | 可用驱动 | 接口 |
|:-----|:---------|:-----|
| `structure` | `sqlite` | `StructuredStore` |
| `embedding` | `openai` | `EmbeddingService` |
| `vector_store` | `qdrant` | `VectorStore` |
| `graph` | `neo4j` | `GraphStore` |

---

## 🚀 快速开始

### 环境要求

| 组件 | 版本 / 说明 |
|:-----|:-----|
| Go | ≥ 1.25 |
| Qdrant | 向量数据库（当前唯一 VectorStore 驱动） |
| Neo4j | 图数据库（仅 Semantic 记忆需要） |
| LLM API | OpenAI 兼容接口（模型 + Embedding） |

### 启动

```bash
# 1. 安装依赖
go mod download

# 2. 准备配置文件
cp app-config.yaml.example app-config.yaml
# 编辑 app-config.yaml，填入 API Key 和服务地址

# 3. 创建数据目录
mkdir -p ./data

# 4. 运行 Demo
go run ./test/main/
```

### 最小可用示例

```go
package main

import (
    "awesome-agent/agents"
    "awesome-agent/core"
    "awesome-agent/memory/rag/ingestion/chunker"
    "awesome-agent/memory/types"
    "awesome-agent/tools"
    "awesome-agent/tools/builtins"
    "context"
)

func main() {
    sessionID := "my-session-uuid"

    // 加载配置
    core.LoadConfig("./app-config.yaml")

    // 创建 LLM 客户端
    llm, _ := core.NewLLM(core.AppCfg.LLMConfig)

    // 注册工具
    registry := tools.NewToolRegistry()
    mt, _ := builtins.NewMemoryTool(core.AppCfg, types.AvailableMemoryTypes, nil, nil, nil, nil)
    rt, _ := builtins.NewRAGTool(nil, nil, nil, core.AppCfg, chunker.Semantic, true, true)
    registry.Register(mt)
    registry.Register(rt)

    // 摄入知识库文档
    ragTool := rt.(*builtins.RAGTool)
    ragTool.Ingest(context.Background(), "./knowledge_base/people.txt", "people.txt")

    // 创建 Agent 并对话
    agent, _ := agents.NewReActAgent("demo", llm, core.AppCfg, registry, 1024, "", sessionID)
    answer, _ := agent.Run(context.Background(), "技术研发中心里的高级工程师有哪几位？")
    println(answer)
}
```

---

## 📊 模块依赖

```
 ┌────────────────────────────────────────────────────────────────┐
 │                        依赖方向 →                              │
 │                                                               │
 │  agents ─────────────────────────────────────────────┐        │
 │    │ 依赖: core, ctx/gssc, tools, tools/builtins     │        │
 │    │                                                 │        │
 │  ctx/gssc ──────────────────────────────┐            │        │
 │    │ 依赖: core, ctx, tools/builtins     │            │        │
 │    │                                     │            │        │
 │  tools/builtins ────────────┐            │            │        │
 │    │ 依赖: core, tools, memory          │            │        │
 │    │                         │          │            │        │
 │  tools ─────────┐           │          │            │        │
 │    │ 依赖: core  │           │          │            │        │
 │    │             │           │          │            │        │
 │  memory ────────┤           │          │            │        │
 │    │ 依赖: core  │           │          │            │        │
 │    ├─ types ────┤           │          │            │        │
 │    │  依赖: core, store, retrieval     │            │        │
 │    ├─ rag ──────┤           │          │            │        │
 │    │  依赖: core, store    │          │            │        │
 │    ├─ retrieval─┘           │          │            │        │
 │    │  依赖: 外部 gse        │          │            │        │
 │    └─ store/impl ───────────┘          │            │        │
 │       依赖: store 接口                 │            │        │
 │                                        │            │        │
 │  core ─────────────────────────────────┘───────────┘────────  │
 │   无内部依赖（基础类型、配置、LLM HTTP 客户端）                │
 └────────────────────────────────────────────────────────────────┘
```

---

## 💡 设计要点

| 类别 | 要点 |
|:-----|:-----|
| 🔐 Session 隔离 | MemoryTool 内部 `map[string]*Manager`，ReActAgent 注入 `_session_id` 自动路由 |
| 📏 Token 估算 | CJK 字符 ≈ 2 tokens/字，非 CJK ≈ 0.25 tokens/字 (`ctx/gssc/EstimateTokens`) |
| 🔒 并发安全 | WorkingMemory: `RWMutex` + `atomic.Bool` CAS 压缩锁；Tool Chain 并行组: `WaitGroup` |
| 🛡 输入校验 | SQLite 表名/列名正则校验防注入；Tool 参数类型校验 + 默认值填充 + 非声明参数剔除 |
| 💾 内存保护 | BaseAgent 消息历史超过 1024 条截断保留后 512 条 |
| 🕐 时区统一 | 全局使用 `core.Now()` (Asia/Shanghai)，`init()` 中 fallback 为 `CST (UTC+8)` |
