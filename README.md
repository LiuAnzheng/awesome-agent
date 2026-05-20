# AwesomeAgent

<div align="center">

**Go 语言实现的 LLM 驱动的 AI Agent 框架**

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

AwesomeAgent 是一个功能丰富的 AI Agent 框架，实现了 **ReAct（推理-行动）** 代理模式，集成**多层持久化记忆系统**、**RAG 检索增强生成**、**并行工具链编排**以及 **MQE / HyDE** 等高级检索特性。

---

## 目录

- [架构概览](#架构概览)
- [核心特性](#核心特性)
- [项目结构](#项目结构)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [核心模块](#核心模块)
  - [Agent 引擎](#agent-引擎)
  - [工具系统](#工具系统)
  - [记忆系统](#记忆系统)
  - [RAG 流水线](#rag-流水线)
  - [LLM 客户端](#llm-客户端)
- [高级特性](#高级特性)
- [API 参考](#api-参考)
- [依赖项](#依赖项)

---

## 架构概览

```
                         ┌──────────────────┐
                         │  app-config.yaml │
                         └────────┬─────────┘
                                  │
                         ┌────────▼─────────┐
                         │   core/config     │
                         │   (Viper 加载)    │
                         └────────┬─────────┘
                                  │
           ┌──────────────────────┼──────────────────────┐
           │                      │                      │
    ┌──────▼──────┐     ┌────────▼────────┐    ┌────────▼────────┐
    │  core/llm   │     │   memory/       │    │    tools/       │
    │  (HTTP API) │     │   manager       │    │  registry       │
    │  流式/同步   │     │   + stores      │    │  executor       │
    └──────┬──────┘     │   + types       │    │  chain          │
           │            │   + rag         │    │  builtins       │
           │            └────────┬────────┘    └────────┬────────┘
           │                     │                      │
           │            ┌────────▼──────────────────────▼──┐
           └────────────►         agents/ReActAgent        │
                        │    Think → Act → Observe 循环    │
                        └─────────────────────────────────┘
```

**数据流**: 配置文件 → LLM 客户端 + 工具注册表（含 RAG / Memory 工具）→ ReAct Agent 对话循环（思考→行动→观察）→ 最终答案。

---

## 核心特性

### Agent 引擎
- **ReAct 模式**: 完整的 "推理 → 行动 → 观察" 循环，支持最大步数控制
- **工具调用**: 自动解析 LLM 返回的 `tool_calls`，执行工具并将结果注入对话上下文
- **可定制系统提示词**: 内置默认 ReAct 提示词，支持完全自定义

### 工具系统
- **插件化注册**: 线程安全的工具注册表，任意扩展
- **工具链编排**: 将多个工具组合为有序流水线，对外暴露为单一工具
  - **串行执行**: 步骤按序执行，后续步骤可引用前序输出
  - **并行执行**: 连续标记为 `Parallel` 的步骤通过 goroutine 并发执行
  - **变量引用**: `$input.xxx` 引用链入参，`$steps.xxx` 引用中间步骤输出
- **内置工具**: Web 搜索、记忆管理、RAG 知识库检索

### 多层记忆系统
| 记忆类型 | 存储后端 | 用途 |
|---------|---------|------|
| **Working** (工作记忆) | 内存 (FIFO) | 短期对话上下文，容量/时间限制，自动淘汰 |
| **Episodic** (情景记忆) | SQLite + Qdrant | 持久化事件摘要，支持语义检索 |
| **Semantic** (语义记忆) | Neo4j + Qdrant | 知识图谱，支持结构化推理和图相似度计算 |
| **Perceptual** (感知记忆) | (预留) | 多模态感知数据存储 |

- **自动压缩**: Working → Episodic (LLM 摘要压缩)，Episodic → Semantic (计划中)
- **跨类型检索**: 统一的 `Search()` 接口，跨所有已启用的记忆类型

### RAG 检索增强生成
- **文档摄入流水线**: 解析 → 去重 (SHA256) → 分块 → 向量化 → 双存储 (SQLite + Qdrant)
- **多格式解析**: 支持 15+ 格式 (Markdown, HTML, CSV, JSON, YAML, TOML, XML, Log 等)
- **递归分块**: Markdown 结构感知，CJK 文本友好，可配置重叠窗口
- **容错机制**: 指数退避重试，事务安全（向量存储失败可标记 `vector_pending` 待重试）

### 高级检索特性
- **MQE** (Multi-Query Expansion): LLM 生成 3 个多样化查询变体，多角度召回
- **HyDE** (Hypothetical Document Embeddings): LLM 生成假设性文档段落进行向量匹配
- **RRF 融合** (Reciprocal Rank Fusion, k=60): 合并原始查询 + MQE + HyDE 的结果

### LLM 客户端
- **OpenAI 兼容**: 支持任意兼容 OpenAI API 的服务（DeepSeek, OpenAI 等）
- **同步 + 流式**: `ChatComplete` (同步) 和 `ChatStream` (SSE 流式)
- **Function Calling**: 完整的 tool_choice / tools 参数支持
- **扩展参数**: `OpenAIExtraInfo` 支持传递供应商特定参数

---

## 项目结构

```
AwesomeAgent/
├── agents/                     # Agent 实现
│   └── react.go                #   ReActAgent: Think → Act → Observe 循环
├── core/                       # 核心框架
│   ├── agent.go                #   BaseAgent: 消息历史、Agent 配置
│   ├── config.go               #   配置加载 (Viper + YAML)
│   ├── llm.go                  #   LLM 客户端 (OpenAI 兼容 HTTP API, 同步/流式)
│   ├── message.go              #   消息类型 (OpenAI 格式)
│   └── time.go                 #   时区工具 (Asia/Shanghai)
├── tools/                      # 工具系统
│   ├── tool.go                 #   Tool 接口定义 + OpenAI Schema 转换
│   ├── registry.go             #   线程安全的工具注册表
│   ├── executor.go             #   工具执行器 (参数校验, 结果格式化)
│   ├── chain.go                #   工具链编排器 (串行/并行, 变量引用)
│   └── builtins/               #   内置工具
│       ├── rag_tool.go         #     RAG 工具 (search / list / delete / status)
│       ├── memory_tool.go      #     记忆工具 (add / search)
│       └── web_search_tool.go  #     网络搜索工具 (Tavily / SerpAPI)
├── memory/                     # 记忆系统 + RAG
│   ├── manager.go              #   记忆管理器 (统一入口)
│   ├── types/                  #   记忆类型实现
│   │   ├── memory.go           #     公共类型 (MemoryItem, Memory 接口)
│   │   ├── working.go          #     工作记忆 (内存 FIFO)
│   │   ├── episodic.go         #     情景记忆 (SQLite + Qdrant)
│   │   ├── semantic.go         #     语义记忆 (Neo4j + Qdrant)
│   │   ├── perceptual.go       #     感知记忆 (预留)
│   │   └── compressor.go       #     压缩器 (LLM 摘要)
│   ├── store/                  #   存储后端
│   │   ├── store.go            #     接口定义 (Structured / Vector / Graph)
│   │   ├── embedding.go        #     Embedding 服务接口
│   │   └── impl/               #     实现
│   │       ├── sqlite_store.go       SQLite (结构化存储)
│   │       ├── qdrant_store.go       Qdrant (向量存储)
│   │       ├── neo4j_store.go        Neo4j (图存储)
│   │       └── openai_embedding.go   OpenAI Embedding
│   └── rag/                    #   RAG 流水线
│       ├── ingestion/          #     文档摄入
│       │   ├── pipeline.go     #       摄入流水线
│       │   ├── parser/         #       文档解析器 (15+ 格式)
│       │   └── chunker/        #       文本分块器 (递归分块)
│       └── advanced_features/  #     高级检索
│           └── recall.go       #       MQE + HyDE + RRF 融合
├── mcp/                        # MCP 协议支持 (规划中)
├── knowledge_base/             # 示例文档
├── data/                       # SQLite 数据库文件
├── test/main/                  # 入口 / 集成测试
├── app-config.yaml             # 主配置文件
├── go.mod / go.sum             # Go 模块依赖
└── CLAUDE.md                   # 项目规则
```

---

## 快速开始

### 前置条件

- **Go** 1.25+
- **LLM API**: OpenAI 兼容的 API 端点（如 DeepSeek）
- **Qdrant** (向量数据库): 用于 RAG 和记忆的向量检索
- **Neo4j** (图数据库, 可选): 用于语义记忆

### 安装与运行

```bash
# 克隆项目
git clone <repo-url>
cd AwesomeAgent

# 安装依赖
go mod download

# 配置环境变量
export LLM_API_KEY="your-api-key"
export LLM_BASE_URL="https://api.deepseek.com/v1"
export EMBEDDING_API_KEY="your-embedding-api-key"
export EMBEDDING_BASE_URL="https://api.deepseek.com/v1"
export NEO4J_PASSWORD="your-neo4j-password"

# 运行示例
go run test/main/main.go
```

### 最小示例

```go
package main

import (
    "awesome-agent/agents"
    "awesome-agent/core"
    "awesome-agent/tools"
    "awesome-agent/tools/builtins"
    "context"
)

func main() {
    // 1. 加载配置
    core.LoadConfig("app-config.yaml")

    // 2. 创建 LLM 客户端
    llm, _ := core.NewAwesomeLLM(core.AppCfg.LLMConfig, core.AppCfg.AgentConfig)

    // 3. 创建工具注册表并注册 RAG 工具
    registry := tools.NewToolRegistry()
    ragTool, _ := builtins.NewRAGTool(nil, nil, nil, core.AppCfg, true, true)
    ragTool.(*builtins.RAGTool).Ingest(context.Background(), "./doc.pdf", "doc.pdf")
    registry.Register(ragTool)

    // 4. 创建 ReAct Agent
    agent := agents.NewReActAgent("my-agent", llm, core.AppCfg.AgentConfig, registry, 1024, "")

    // 5. 运行
    answer, _ := agent.Run(context.Background(), "文档的主要内容是什么？")
    println(answer)
}
```

---

## 配置说明

配置文件 `app-config.yaml` 使用 Viper 加载，支持 `${ENV_VAR}` 环境变量展开：

```yaml
awesome-agent:
  llm:
    model_id: "deepseek-v4-pro"       # 模型 ID
    provider: "deepseek"              # 供应商标识
    api_key: ${LLM_API_KEY}           # API 密钥 (环境变量)
    base_url: ${LLM_BASE_URL}         # API 地址 (环境变量)

  rag:
    max_doc_size: 52428800            # 最大文档大小 (50MB)
    collection: "rag"                 # Qdrant 集合名称

  memory:
    structure:                        # 结构化存储
      driver: "sqlite"
      options:
        db_path: "./data/memory.db"

    embedding:                        # 向量嵌入服务
      driver: "openai"
      options:
        model_id: "text-embedding-v4"
        dimension: 1024
        batch_size: 10

    vector_store:                     # 向量数据库
      driver: "qdrant"
      options:
        host: "192.168.187.100"
        port: 6333

    graph:                            # 图数据库 (可选)
      driver: "neo4j"
      options:
        url: "http://192.168.187.100:7474"
        db: "neo4j"
        username: "neo4j"
        password: ${NEO4J_PASSWORD}
```

---

## 核心模块

### Agent 引擎

ReActAgent 实现了经典的 ReAct 范式：

```
循环 (最多 MaxSteps 步):
  1. 将完整消息历史 + 工具 Schema 发送给 LLM
  2. 如果 LLM 返回 tool_calls → 执行工具，结果追加到历史，继续循环
  3. 如果 LLM 无 tool_calls → 返回最终答案，结束
```

```go
// 创建 Agent
agent := agents.NewReActAgent(
    "react-agent",     // 名称
    llm,               // LLM 客户端
    config,            // Agent 配置
    registry,          // 工具注册表
    1024,              // 最大步数
    "",                // 系统提示词（空则使用默认）
)

// 运行
answer, err := agent.Run(ctx, "你的问题")
```

### 工具系统

#### 自定义工具

```go
type MyTool struct{}

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "我的自定义工具" }
func (t *MyTool) Run(params map[string]interface{}) (string, error) {
    // 实现工具逻辑
    return "result", nil
}
func (t *MyTool) Parameters() []tools.ToolParameter {
    return []tools.ToolParameter{
        {Name: "query", Type: "string", Required: true, Description: "查询参数"},
    }
}

// 注册
registry.Register(&MyTool{})
```

#### 工具链编排

将多个工具组合为有序流水线，对外暴露为单一工具：

```go
chain := tools.MustNewChain(
    "research_pipeline",
    "搜索网络并分析结果",
    []tools.ToolParameter{
        {Name: "topic", Type: "string", Required: true},
    },
    []tools.ChainStep{
        {ToolName: "web_search", ParamMap: map[string]string{
            "query": "$input.topic",
        }, StoreAs: "search_results"},

        {ToolName: "analyze", ParamMap: map[string]string{
            "data": "$steps.search_results",
        }, StoreAs: "analysis", Parallel: true},

        {ToolName: "summarize", ParamMap: map[string]string{
            "data": "$steps.search_results",
        }, StoreAs: "summary", Parallel: true},
    },
    registry,
)

registry.Register(chain)
```

**变量引用规则**:
- `$input.xxx` — 引用工具链接收的外部入参
- `$steps.xxx` — 引用前序步骤的 `StoreAs` 输出
- 并行组内步骤不可相互引用（保证并发安全）

### 记忆系统

#### 架构设计

```
┌─────────────┐    压缩     ┌─────────────┐    压缩     ┌─────────────┐
│   Working   │ ────────►  │  Episodic   │ ────────►  │  Semantic   │
│  (短期记忆)  │   LLM摘要   │  (情景记忆)  │   (计划中)  │  (语义记忆)  │
│  内存 FIFO  │            │ SQLite+Qdrant│            │ Neo4j+Qdrant│
└─────────────┘            └─────────────┘            └─────────────┘
```

#### Working Memory
- **存储**: 内存 FIFO 队列
- **容量**: 默认 1024 条，可配置
- **TTL**: 默认 60 分钟
- **淘汰策略**: 超出容量时移除最旧条目
- **压缩触发**: 容量达 90% 时通过 CAS 锁异步触发

#### Episodic Memory
- **存储**: SQLite (结构化) + Qdrant (向量)
- **事件类型**: Observation, Thought, Action, Result
- **检索评分**: 向量相似度 (80%) + 时间衰减 (20%) × 重要度因子

#### Semantic Memory
- **存储**: Neo4j (知识图谱) + Qdrant (向量)
- **去重**: 向量相似度 > 0.9 自动合并
- **检索评分**: 向量相似度 (70%) + 图结构相似度 (30%) × 重要度因子
- **图相似度**: 基于邻居节点重叠度计算

### RAG 流水线

#### 摄入流程

```
文档 ──► 解析器 ──► SHA256 去重 ──► 递归分块 ──► 向量嵌入 ──► SQLite + Qdrant
  │        │                                    │              │
  │   格式检测                              可配置重叠        事务安全
  │   (15+ 格式)                         CJK 感知          失败回滚
```

#### 支持的文件格式

| 类别 | 格式 |
|------|------|
| 标记语言 | `md`, `markdown`, `html`, `htm`, `xml` |
| 数据格式 | `json`, `csv`, `yaml`, `yml`, `toml`, `ini`, `cfg`, `conf` |
| 文本格式 | `txt`, `log`, `env` |

所有格式统一转换为 Markdown 后再分块，确保下游处理一致性。

#### 分块策略

- **结构感知**: 解析 Markdown 标题层级作为边界
- **递进切分**: 标题 → 段落 → 行 → 字符级别
- **上下文保留**: 每个分块携带标题路径信息
- **重叠窗口**: 相邻分块可配置重叠区域

### LLM 客户端

```go
// 同步调用
resp, finishReason, err := llm.ChatComplete(ctx, messages, tools, extraInfo)

// 流式调用
stream, err := llm.ChatStream(ctx, messages, tools, extraInfo)
for chunk := range stream {
    fmt.Print(chunk.Content)
}
```

支持的 `finish_reason`: `stop`, `length`, `tool_calls`, `content_filter`。

---

## 高级特性

### MQE (Multi-Query Expansion)

LLM 将用户原始查询扩展为 3 个不同角度的查询变体，从多个维度覆盖语义空间：

```
用户查询: "OpenAI API 的鉴权方式"
  → 变体1: "OpenAI API 支持哪些认证方法"
  → 变体2: "如何使用 Bearer Token 进行 OpenAI API 认证"
  → 变体3: "OpenAI API Key 的配置和使用方式"
```

### HyDE (Hypothetical Document Embeddings)

LLM 先生成一个假设性文档段落来回答查询，再用这个"理想答案"进行向量检索：

```
用户查询: "OpenAI API 鉴权规范"
  → 假设文档: "OpenAI API 使用 Bearer Token 认证方式，
               需要在请求头中携带 Authorization: Bearer {API_KEY}..."
  → 用假设文档的向量进行相似度检索
```

### RRF (Reciprocal Rank Fusion)

将原始查询、MQE 变体、HyDE 假设文档的检索结果，通过 RRF 算法 (k=60) 合并排序，获得更稳健的多查询融合结果。

### 记忆压缩

当 Working Memory 接近容量上限 (90%) 时，自动触发异步压缩：

```
Working Memory (多条零散的对话片段)
      │
      ▼ LLM 压缩 (compress_memory function calling)
      │
Episodic Memory (一条结构化摘要)
  - 提取核心线索
  - 过滤闲聊噪声
  - 保留关键细节
```

---

## API 参考

### 核心接口

```go
// LLM 客户端
type LLMInterface interface {
    ChatComplete(ctx context.Context, messages []Message,
        tools []map[string]interface{}, extraInfo map[string]interface{}) (Message, FinishReasonType, error)
    ChatStream(ctx context.Context, messages []Message,
        tools []map[string]interface{}, extraInfo map[string]interface{}) (<-chan StreamChunk, error)
}

// 工具
type Tool interface {
    Name() string
    Description() string
    Parameters() []ToolParameter
    Run(parameters map[string]interface{}) (string, error)
}

// 记忆
type Memory interface {
    Add(item *MemoryItem) error
    Search(ctx context.Context, query string, limit int, embedding EmbeddingService, vector VectorStore) ([]*MemoryItem, error)
    Delete(id string) error
    Status() *MemoryStatus
}

// 存储后端
type StructuredStore interface { /* Init, Save, Get, Query, Delete, BatchDelete, Exec, Close */ }
type VectorStore interface     { /* Init, Upsert, BatchUpsert, Search, Delete, Close */ }
type GraphStore interface      { /* Init, CreateNode, UpdateNode, GetNode, DeleteNode, CreateRelation, Query, Close */ }
```

### 关键类型

```go
// 消息
type Message struct {
    Role       string
    Content    interface{}
    ToolCalls  []ToolCall
    ToolCallID string
}

// 工具调用
type ToolCall struct {
    ID       string
    Type     string
    Function FunctionCall
}

// 记忆条目
type MemoryItem struct {
    ID             string
    SessionID      string
    Content        string
    Importance     float64
    CompressedFrom []string
    Status         MemoryStatus
}
```

---

## 依赖项

| 依赖 | 用途 |
|------|------|
| `github.com/spf13/viper` | YAML 配置加载 + 环境变量展开 |
| `modernc.org/sqlite` | 纯 Go SQLite 驱动 (无 CGO) |
| `golang.org/x/net` | HTML 解析 (文档解析器) |
| `gopkg.in/yaml.v3` | YAML 解析 |
| `github.com/BurntSushi/toml` | TOML 解析 |
| `github.com/google/uuid` | UUID 生成 (向量 ID) |

### 外部服务依赖

| 服务 | 用途 | 必需 |
|------|------|------|
| OpenAI 兼容 API | LLM 推理 + 文本嵌入 | ✅ 是 |
| Qdrant | 向量检索 | ✅ 是 (RAG / 记忆) |
| Neo4j | 知识图谱 (语义记忆) | ❌ 可选 |
| Tavily / SerpAPI | 网络搜索 | ❌ 可选 |

---
