<p align="right">
  <b>中文</b> | <a href="README.md">English</a>
</p>

<p align="center">
  <img src="logo.jfif" alt="Memoria Logo" width="160">
</p>

<p align="center">
  <h1 align="center">Memoria</h1>
  <p align="center">
    <strong>Go 语言 AI Agent 框架 —— 多层记忆 · RAG 检索 · 上下文感知推理</strong>
  </p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.25-00ADD8?style=flat&logo=go" alt="Go 1.25">
  <img src="https://img.shields.io/badge/LLM-OpenAI_Compatible-412991?style=flat&logo=openai" alt="OpenAI Compatible">
  <img src="https://img.shields.io/badge/storage-Qdrant(可选)-FF6F00?style=flat" alt="Qdrant">
  <img src="https://img.shields.io/badge/graph-Neo4j(可选)-4581C3?style=flat&logo=neo4j" alt="Neo4j">
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat" alt="License">
</p>

---

## ⚡ 快速开始

### 安装

```bash
go get github.com/LiuAnzheng/memoria
```

### 运行 Demo

```bash
# 1. 克隆项目
git clone https://github.com/LiuAnzheng/memoria.git && cd memoria

# 2. 设置 LLM API Key（兼容 OpenAI 协议）
export LLM_API_KEY=sk-xxx
export LLM_BASE_URL=https://api.openai.com

# 3. 运行
go run ./test/main/
```

**20 行 Go 代码，只需 API Key，零基础设施依赖：**

```go
package main

import (
    "context"
    "github.com/LiuAnzheng/memoria/agents"
    "github.com/LiuAnzheng/memoria/core"
    "github.com/LiuAnzheng/memoria/memory/types"
    "github.com/LiuAnzheng/memoria/tools"
    "github.com/LiuAnzheng/memoria/tools/builtins"
)

func main() {
    // 仅需配置 LLM — 其余全部使用内存默认实现
    core.AppCfg.LLMConfig.APIKey  = "sk-xxx"
    core.AppCfg.LLMConfig.BaseURL = "https://api.openai.com"

    llm, _ := core.NewLLM(core.AppCfg.LLMConfig)
    registry := tools.NewToolRegistry()

    // 注册 Working Memory（纯内存，零外部依赖）
    mt, _ := builtins.NewMemoryTool(core.AppCfg,
        []types.MemoryType{types.Working}, // 纯内存 — 无需 Qdrant
        nil, nil, nil, nil)
    registry.Register(mt)

    agent, _ := agents.NewReActAgent("demo", llm, core.AppCfg, registry, 64, "", "session-1")

    answer, _ := agent.Run(context.Background(), "我叫李伟，是一名高级工程师。")
    // Agent 记住了。在同一个 session 内继续追问：
    answer, _ = agent.Run(context.Background(), "我叫什么名字？职位是什么？")
    println(answer) // → "你叫李伟，是一名高级工程师。"
}
```

> **前置条件**：Go 1.25+ 和一个 OpenAI 兼容的 API 端点。仅此而已。

<details>
<summary>📦 启用持久化记忆（Qdrant + SQLite）—— 点击展开</summary>

```go
// 添加 Episodic 记忆，支持跨进程重启后仍能回忆：
mt, _ := builtins.NewMemoryTool(core.AppCfg,
    []types.MemoryType{types.Working, types.Episodic}, // + SQLite + Qdrant
    nil, nil, nil, nil)                                // 自动用 config 默认值初始化
```

Qdrant 和 SQLite 仅在需要 Episodic/Semantic 记忆或 RAG 时引入。先用纯 Working Memory 启动——它已经能处理同一 session 内的跨轮次记忆。

</details>

---

## 🆚 为什么选择 Memoria？

| | Memoria | [Eino](https://github.com/cloudwego/eino) | [langchaingo](https://github.com/tmc/langchaingo) | [ADK-Go](https://github.com/google/adk-go) |
|:---|:---|:---|:---|:---|
| **记忆** | Working → Episodic → Semantic<br>三层记忆 + LLM 压缩<br>+ BM25 + 向量 + 图检索 | Summarization 中间件<br>（仅上下文窗口压缩） | 对话历史缓冲<br>（无长期记忆） | Session 记忆<br>（键值存储） |
| **RAG** | 语义分块（自适应阈值）<br>+ MQE + HyDE + RRF 融合<br>+ 带重试和去重的摄入管线 | Retriever 组件<br>（委托给外部实现） | 文档加载器 + 向量库<br>（标准 LangChain RAG） | —（未内置） |
| **上下文** | GSSC 管线<br>（Gather → Select → Structure）<br>Token 预算 + 综合评分 | History rewriter<br>+ 摘要中间件 | Prompt 模板 +<br>消息历史 | 自动上下文窗口管理 |
| **工具** | Chain 编排 + $steps.xxx 步骤间引用<br>+ 串行/并行 DAG<br>+ 编译期校验 | Graph/Chain/Workflow<br>+ 编译期类型检查<br>+ 中断与恢复 | Tool + Agent 执行器<br>（标准 LangChain 工具） | Tool + Function Calling<br>+ A2A 协议 |
| **多 Agent** | —（专注单 Agent 深度） | Supervisor / Plan-Execute /<br>Layered Supervisor | 基础 Agent 组合 | A2A 开放协议<br>+ Agent Transfer |
| **生态** | 零外部依赖<br>驱动插件架构 | 字节跳动 CloudWeGo<br>+ DevOps 工具链 | 丰富的提供商生态<br>（20+ LLM, 10+ 向量库） | Google Cloud 原生<br>Vertex AI 集成 |
| **适用场景** | 需要**跨轮次持久记忆**<br>和精细化 RAG 的项目 | 需要复杂编排的<br>生产级微服务 | 需要丰富集成的<br>快速原型开发 | Google Cloud 技术栈<br>多 Agent 需求 |

---

## 🏗 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     agents/react                            │
│                  ┌──────────────┐                            │
│                  │  ReActAgent  │  感知 → 思考 → 行动      │
│                  └──────┬───────┘                            │
│        ┌────────────────┼────────────────┐                   │
│        ▼                ▼                 ▼                   │
│  ┌──────────┐   ┌────────────┐   ┌──────────────┐           │
│  │ ctx/gssc │   │   tools    │   │   memory     │           │
│  │ Gather   │   │ Registry   │   │  Manager     │           │
│  │ Select   │   │ Executor   │   │  ├─Working   │ 内存中    │
│  │ Structure│   │ Chain      │   │  ├─Episodic  │ SQLite    │
│  └──────────┘   │ ┌────────┐ │   │  ├─Semantic  │ Neo4j     │
│                 │ │builtins│ │   │  └─Perceptual│ 预留      │
│                 │ ├Memory  │ │   │              │           │
│                 │ ├RAG     │ │   │  store/impl  │           │
│                 │ └WebSrch │ │   │  ├─SQLite    │           │
│                 │ └────────┘ │   │  ├─Qdrant    │           │
│                 └────────────┘   │  ├─Neo4j     │           │
│                                  │  └─OpenAIEmb │           │
│  ┌───────────────────────────────┴──────────────┘           │
│  │                    core                                  │
│  │  BaseAgent │ LLMInterface (OpenAI HTTP) │ Config (viper) │
│  └──────────────────────────────────────────────────────────┘
```

| 模块 | 职责 |
|:-----|:-----|
| `core/` | `BaseAgent` 基类、OpenAI 兼容 HTTP 客户端、多模态 `Message`、Viper 配置管理 |
| `agents/` | `ReActAgent` — 感知 → 思考 → 行动循环 |
| `tools/` | `Tool` 接口、`ToolRegistry` 注册中心、`ToolExecutor` 执行器、`Chain` 多步编排 |
| `tools/builtins/` | 内置工具：`MemoryTool`、`RAGTool`、`WebSearchTool` |
| `ctx/gssc/` | GSSC 管线：`Gatherer` → `Selector` → `Structurer` → `ContextBuilder` |
| `memory/` | 多层级记忆管理器 + 类型定义（Working / Episodic / Semantic） |
| `memory/store/` | 存储接口：`StructuredStore` / `VectorStore` / `GraphStore` / `EmbeddingService` |
| `memory/store/impl/` | 驱动实现：SQLite、Qdrant、Neo4j、OpenAI Embedding |
| `memory/retrieval/` | BM25 评分器、稀疏向量、中文分词器（gse） |
| `memory/rag/` | 文档摄入管线 + 高级检索（MQE 查询扩展、HyDE 假设文档、RRF 融合） |
| `mcp/` | MCP 协议桩（预留） |

---

## 🧠 多层记忆系统

每层记忆有独立的检索策略：

```
Working (内存)               Episodic (SQLite+Qdrant)       Semantic (Neo4j+Qdrant)
┌──────────────────┐  90%   ┌──────────────────────┐      ┌─────────────────────┐
│ BM25 关键词检索    │──满──▶│ 向量 + 时新度评分      │      │ 向量 + 图结构评分     │
│ FIFO 淘汰         │──▶   │ score = (cos*0.8 +    │      │ score = (cos*0.7 +   │
│ 容量: 1024        │       │   recency*0.2) *      │      │   graph*0.3) *       │
└──────────────────┘       │   (0.8 + imp*0.4)     │      │   (0.8 + imp*0.4)   │
                            │ 因果关系网络            │      │ 知识图谱               │
                            └──────────────────────┘      └─────────────────────┘
```

> **压缩流程**：Working 达到 90% 容量 → CAS 加锁 → 取最早 30 条快照 → LLM 调用 `compress_memory` 函数 → 输出叙事+摘要+重要性+事件类型 → 写入 Episodic → 删除源条目。

| 层级 | 评分公式 | 差异化机制 |
|:-----|:-----|:-----|
| **Episodic** | `(cos×0.8 + recency×0.2) × (0.8 + imp×0.4)` | 时新度衰减惩罚过时事件；重要性控制留存 |
| **Semantic** | `(cos×0.7 + graph×0.3) × (0.8 + imp×0.4)` | 图共现评分过滤孤立误匹配，Precision@5 提升 10%~15% |

---

## 📚 RAG 检索引擎

### 文档摄入管线

```
文件                  分块                   向量化              存储
┌──────┐  ┌───────┐  ┌────────────────┐  ┌──────────────┐  ┌──────────────┐
│ .txt │  │Parser │  │RecursiveChunker│  │              │  │ SQLite       │
│ .md  │─▶│(16种   │─▶│ 标题层级递归     │─▶│ OpenAI Embed │─▶│ (元数据)      │
│ .html│  │格式)   │  │ 段落→行→字符降级  │  │ 批量+重试     │  │ Qdrant       │
│ .csv │  │       │  │SemanticChunker  │  │ 3次指数退避   │  │ (向量)        │
│ .json│  └───────┘  │ 自适应余弦阈值   │  └──────────────┘  └──────────────┘
│ ...  │             └────────────────┘
└──────┘
```

### 高级检索

| 特性 | 机制 |
|:-----|:-----|
| **MQE** | LLM 生成 3 个语义等价变体查询，扩充召回覆盖 |
| **HyDE** | LLM 生成假设性百科文档，用文档向量搜索（文档↔文档 优于 查询↔文档） |
| **RRF** | Reciprocal Rank Fusion（k=60），5 路并行搜索结果融合排序 |

---

## 📐 GSSC 上下文构建管线

```
 ┌──────────┐     ┌──────────┐     ┌───────────┐     ┌──────────┐
 │ Gatherer │────▶│ Selector │────▶│ Structurer│────▶│  System  │
 │ 收集      │     │ 评分筛选  │     │ 模板渲染   │     │  Prompt  │
 └──────────┘     └──────────┘     └───────────┘     └──────────┘
```

| 阶段 | 动作 |
|:-----|:-----|
| **Gather** | 从四个来源收集 `ContextPacket`：系统指令、记忆搜索、RAG 搜索、最近 32 条历史消息 |
| **Select** | 综合评分 = `relevance_weight × Jaccard 相似度` + `recency_weight × 指数衰减时新度`，在 token 预算内按分数截断。语义记忆自动跳过时新度衰减 |
| **Structure** | 按来源渲染为结构化段落：[Role & Policies] → [Task] → [Evidence] → [Context] → [Output] |

---

## 🔧 工具系统与编排

```go
type Tool interface {
    Name() string
    Description() string
    Run(params map[string]interface{}) (string, error)
    Parameters() []ToolParameter
}
```

通过 `ToolToOpenAISchema()` 自动转换为 OpenAI Function Calling JSON Schema。

### Chain：多步骤工具编排

```
Chain: "rag_research"
  ├── Step 1: web_search("topic")  ──(storeAs: web)──┐
  ├── Step 2: rag.search("topic")  ──(storeAs: docs) │
  │                                                   │
  └── Step 3: analyze                                  │
        prompt: "基于 $steps.web 和 $steps.docs         │
                的信息给出综合结论"  ◀───────────────────┘
```

| 模式 | 行为 | 约束 |
|:-----|:-----|:-----|
| **串行** | 按序执行，`$steps.xxx` 引用前序步骤的输出 | 只能引用已完成的步骤 |
| **并行** | 相邻 `Parallel=true` 步骤并发执行 | 禁止同组内互相引用 |

编译期校验：重名检测、并行组循环依赖检测、未定义引用检测——将编排错误从运行时提前到启动期。

---

## 🗂 配置系统

基于 Viper 加载 YAML，支持 `${ENV_VAR}` 环境变量展开：

```yaml
awesome-agent:
  llm:       { model_id, provider, api_key, base_url, max_tokens, temperature, top_p }
  memory:
    structure:    { driver: "sqlite",  options: { db_path } }
    embedding:    { driver: "openai",  options: { model_id, dimension, batch_size } }
    vector_store: { driver: "qdrant",  options: { host, port } }
    graph:        { driver: "neo4j",   options: { url, username, password } }
  rag:       { max_doc_size, collection }
  context:   { max_tokens, reserve_ratio, min_relevance, recency_weight, relevance_weight }
```

所有存储后端采用统一的 **Driver + Options** 插件模式——切换实现无需改动代码。

---

## 💡 关键设计点

- **Session 隔离**：`MemoryTool` 内部持有 `map[string]*Manager`，通过 `_session_id` 自动路由到对应会话
- **并发安全**：WorkingMemory 使用 `RWMutex` + `atomic.Bool`（CAS 压缩锁）；Tool Chain 并行组使用 `WaitGroup`
- **输入校验**：SQLite 表名/列名正则防注入；Tool 参数类型校验 + 默认值填充 + 非声明参数剔除
- **Token 估算**：CJK 字符 ≈ 2 tokens/字，非 CJK ≈ 0.25 tokens/字
- **内存保护**：Agent 消息历史超过 1024 条时截断保留后 512 条
- **时区统一**：全局使用 `core.Now()`（Asia/Shanghai，CST fallback）

---

## 📊 模块依赖

```
 ┌────────────────────────────────────────────────────────────────┐
 │                      依赖方向 →                               │
 │                                                               │
 │  agents ─────────────────────────────────────────────┐        │
 │    └─ 依赖: core, ctx/gssc, tools, tools/builtins    │        │
 │  ctx/gssc ──────────────────────────────┐            │        │
 │    └─ 依赖: core, ctx, tools/builtins    │            │        │
 │  tools/builtins ────────────┐           │            │        │
 │    └─ 依赖: core, tools, memory          │            │        │
 │  tools ─────────┐           │           │            │        │
 │    └─ 依赖: core            │           │            │        │
 │  memory ────────┤           │           │            │        │
 │    └─ 依赖: core            │           │            │        │
 │    ├─ types: core, store, retrieval     │            │        │
 │    ├─ rag: core, store                 │            │        │
 │    ├─ retrieval: 外部 gse              │            │        │
 │    └─ store/impl: store 接口           │            │        │
 │  core ─────────────────────────────────┴───────────┴────────  │
 │   零内部依赖                                                   │
 └────────────────────────────────────────────────────────────────┘
```
