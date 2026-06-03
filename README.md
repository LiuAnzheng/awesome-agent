<p align="right">
  <a href="README.zh-CN.md">中文</a> | <b>English</b>
</p>

<p align="center">
  <img src="logo.jfif" alt="Memoria Logo" width="160">
</p>

<p align="center">
  <h1 align="center">Memoria</h1>
  <p align="center">
    <strong>A Go-native AI Agent framework with multi-tier memory, RAG, and context-aware reasoning</strong>
  </p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.25-00ADD8?style=flat&logo=go" alt="Go 1.25">
  <img src="https://img.shields.io/badge/LLM-OpenAI_Compatible-412991?style=flat&logo=openai" alt="OpenAI Compatible">
  <img src="https://img.shields.io/badge/storage-Qdrant_(optional)-FF6F00?style=flat" alt="Qdrant">
  <img src="https://img.shields.io/badge/graph-Neo4j_(optional)-4581C3?style=flat&logo=neo4j" alt="Neo4j">
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat" alt="License">
</p>

---

## ⚡ Quick Start

### Install

```bash
go get github.com/LiuAnzheng/memoria
```

### Run the Demo

```bash
# 1. Clone
git clone https://github.com/LiuAnzheng/memoria.git && cd memoria

# 2. Set your LLM API key (OpenAI-compatible)
set MODEL_API_KEY=sk-xxx           # Windows
set MODEL_BASE_URL=https://api.openai.com

# 3. Run
go run ./test/main/
```

**Minimal setup — API key only, zero infrastructure:**

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
    // LLM config with env fallback — set MODEL_API_KEY + MODEL_BASE_URL
    llm, _ := core.NewLLM(core.LLMConfig{ModelID: "gpt-5.4"}.ApplyEnv())
    registry := tools.NewToolRegistry()

    // Working memory only — in-memory, zero deps
    mt, _ := builtins.NewMemoryTool(
        core.MemoryConfig{}, nil,                     // nil LLM = no compression
        []types.MemoryType{types.Working},
    )
    registry.Register(mt)

    // Terminal tool — command execution with sandbox & whitelist
    tt, _ := builtins.NewTerminalTool(core.TerminalConfig{
        AllowedCommands: []string{"ls", "dir", "cat", "head", "tail", "find", "grep",
            "wc", "sort", "uniq", "pwd", "file", "stat", "echo", "go", "git"},
    })
    registry.Register(tt)

    agent, _ := agents.NewReActAgent("demo", llm, core.ContextConfig{}, registry, 64, "", "session-1")

    answer, _ := agent.Run(context.Background(), "My name is Li Wei. I'm a senior engineer.")
    // Agent remembers. Ask a follow-up question in the same session.
    answer, _ = agent.Run(context.Background(), "What's my name and role?")
    println(answer) // → "You are Li Wei, a senior engineer."
}
```

> **Prerequisites**: Go 1.25+ and an OpenAI-compatible API endpoint. That's it.

<details>
<summary>📦 Adding persistent memory (Qdrant + SQLite) — click to expand</summary>

```go
// Add Episodic memory for multi-turn persistence across restarts:
// Pass MemoryConfig to auto-init stores + LLM for compression
mt, _ := builtins.NewMemoryTool(
    core.MemoryConfig{
        Structured:  core.DriverConfig{Driver: "sqlite", Options: map[string]any{"db_path": "./data/memory.db"}},
        Embedding:   core.DriverConfig{Driver: "openai", Options: map[string]any{"model_id": "text-embedding-3-small"}},
        VectorStore: core.DriverConfig{Driver: "qdrant", Options: map[string]any{"host": "127.0.0.1", "port": 6333}},
    },
    llm,                                             // LLM for memory compression
    []types.MemoryType{types.Working, types.Episodic},
)
```

Qdrant and SQLite are needed for Episodic/Semantic memory and RAG. Start with Working-only as above — it already handles cross-turn recall within a session.

</details>

---

## 🆚 Why Memoria?

| | Memoria | [Eino](https://github.com/cloudwego/eino) | [langchaingo](https://github.com/tmc/langchaingo) | [ADK-Go](https://github.com/google/adk-go) |
|:---|:---|:---|:---|:---|
| **Memory** | Working → Episodic → Semantic<br>three-tier with LLM compression<br>+ BM25 + vector + graph retrieval | Summarization middleware<br>(context window compression only) | Chat history buffer<br>(no long-term memory) | Session memory<br>(key-value storage) |
| **RAG** | Semantic chunking (adaptive threshold)<br>+ MQE + HyDE + RRF fusion<br>+ pipeline with retry & dedup | Retriever component<br>(delegate to external impl) | Document loader + vectorstore<br>(standard LangChain RAG) | — (not built-in) |
| **Context** | GSSC pipeline<br>(Gather → Select → Structure)<br>token budget with scoring | History rewriter<br>+ summarization middleware | Prompt template +<br>message history | Automatic context window management |
| **Tools** | Chain with `$steps.xxx` inter-step refs<br>+ serial/parallel DAG<br>+ compile-time validation | Graph/Chain/Workflow<br>+ compile-time type check<br>+ interrupt & resume | Tool + Agent executor<br>(standard LangChain tooling) | Tool + function calling<br>+ A2A protocol |
| **Multi-Agent** | — (single agent focus) | Supervisor / Plan-Execute /<br>Layered Supervisor | Basic agent composition | A2A open protocol<br>+ agent transfer |
| **Ecosystem** | 0 dependencies beyond Go<br>driver plugin architecture | ByteDance CloudWeGo<br>+ DevOps tools | Rich provider ecosystem<br>(20+ LLMs, 10+ vector DBs) | Google Cloud native<br>Vertex AI integration |
| **Best for** | Projects that need **persistent memory**<br>across turns with structured recall | Production microservices<br>with complex orchestration | Quick prototyping with<br>rich integrations | Google Cloud shops<br>with multi-agent needs |

---

## 🏗 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     agents/react                            │
│                  ┌──────────────┐                            │
│                  │  ReActAgent  │  PERCEIVE → THINK → ACT  │
│                  └──────┬───────┘                            │
│        ┌────────────────┼────────────────┐                   │
│        ▼                ▼                 ▼                   │
│  ┌──────────┐   ┌────────────┐   ┌──────────────┐           │
│  │ ctx/gssc │   │   tools    │   │   memory     │           │
│  │ Gather   │   │ Registry   │   │  Manager     │           │
│  │ Select   │   │ Executor   │   │  ├─Working   │ in-mem    │
│  │ Structure│   │ Chain      │   │  ├─Episodic  │ SQLite    │
│  └──────────┘   │ ┌────────┐ │   │  ├─Semantic  │ Neo4j     │
│                 │ │builtins│ │   │  └─Perceptual│ stub     │
│                 │ ├Memory  │ │   │              │           │
│                 │ ├RAG     │ │   │  store/impl  │           │
│                 │ ├WebSrch │ │   │  ├─SQLite    │           │
│                 │ ├Note    │ │   │  ├─Qdrant    │           │
│                 │ └Terminal│ │   │  ├─Neo4j     │           │
│                 │ └────────┘ │   │  └─OpenAIEmb │           │
│  ┌───────────────────────────────┴──────────────┘           │
│  │                    core                                  │
│  │  BaseAgent │ LLMInterface (OpenAI HTTP) │ Config          │
│  └──────────────────────────────────────────────────────────┘
```

| Package | Responsibility |
|:-----|:-----|
| `core/` | `BaseAgent`, OpenAI-compatible HTTP client, multimodal `Message`, typed config structs |
| `agents/` | `ReActAgent` — PERCEIVE → THINK → ACT loop |
| `tools/` | `Tool` interface, `ToolRegistry`, `ToolExecutor`, `Chain` (multi-step orchestration) |
| `tools/builtins/` | `MemoryTool`, `RAGTool`, `WebSearchTool`, `NoteTool`, `TerminalTool` |
| `ctx/gssc/` | GSSC pipeline: `Gatherer` → `Selector` → `Structurer` → `ContextBuilder` |
| `memory/` | Multi-tier memory manager + types (Working / Episodic / Semantic) |
| `memory/store/` | Storage interfaces: `StructuredStore` / `VectorStore` / `GraphStore` / `EmbeddingService` |
| `memory/store/factory/` | Pluggable driver registry — `Register("postgres", ...)` → auto-used by all tools |
| `memory/store/impl/` | Built-in drivers: SQLite, Qdrant, Neo4j, OpenAI Embedding (auto-registered via `init()`) |
| `memory/retrieval/` | BM25 scorer, sparse vectors, Chinese tokenizer (gse) |
| `memory/rag/` | Ingestion pipeline + advanced recall (MQE, HyDE, RRF fusion) |
| `note/` | Note data model: `NoteType`, `NoteMetadata`, `NoteIndex` |
| `mcp/` | MCP protocol stubs (reserved) |

---

## 🧠 Multi-Tier Memory

The memory system has distinct retrieval strategies per tier:

```
Working (in-memory)          Episodic (SQLite+Qdrant)       Semantic (Neo4j+Qdrant)
┌──────────────────┐  90%   ┌──────────────────────┐      ┌─────────────────────┐
│ BM25 keyword      │──full─▶│ Vector + recency      │      │ Vector + graph       │
│ FIFO eviction     │──▶    │ score = (cos*0.8 +    │      │ score = (cos*0.7 +   │
│ capacity: 1024    │       │   recency*0.2) *      │      │   graph*0.3) *       │
└──────────────────┘       │   (0.8 + imp*0.4)     │      │   (0.8 + imp*0.4)   │
                            │ causal relations       │      │ knowledge graph       │
                            └──────────────────────┘      └─────────────────────┘
```

> **Compression flow**: Working hits 90% capacity → CAS lock → snapshot 30 oldest items → LLM `compress_memory` function call → structured narrative + summary + importance + event_type → write to Episodic → remove source items.

| Tier | Scoring Formula | Key Differentiator |
|:-----|:-----|:-----|
| **Episodic** | `(cos×0.8 + recency×0.2) × (0.8 + imp×0.4)` | Recency decay penalizes stale events; importance gates retention |
| **Semantic** | `(cos×0.7 + graph×0.3) × (0.8 + imp×0.4)` | Graph co-occurrence filters out isolated false positives |

---

## 📚 RAG Engine

### Ingestion Pipeline

```
File                Chunking              Embedding            Storage
┌──────┐  ┌───────┐  ┌────────────────┐  ┌──────────────┐  ┌──────────────┐
│ .txt │  │Parser │  │RecursiveChunker │  │              │  │ SQLite       │
│ .md  │─▶│(16   │─▶│ heading-tree     │─▶│ OpenAI Embed │─▶│ (metadata)   │
│ .html│  │fmts)  │  │ para→line→char   │  │ batch + retry│  │ Qdrant       │
│ .csv │  │       │  │SemanticChunker  │  │ 3x backoff   │  │ (vectors)    │
│ .json│  └───────┘  │ adaptive cosine  │  └──────────────┘  └──────────────┘
│ ...  │             └────────────────┘
└──────┘
```

### Advanced Retrieval

| Feature | Mechanism |
|:-----|:-----|
| **MQE** | LLM generates 3 semantic query variants for broader recall |
| **HyDE** | LLM writes a hypothetical document; search with doc vector (doc↔doc > query↔doc) |
| **RRF** | Reciprocal Rank Fusion (k=60) merges 5 parallel search result lists |

---

## 📐 GSSC Context Pipeline

```
 ┌──────────┐     ┌──────────┐     ┌───────────┐     ┌──────────┐
 │ Gatherer │────▶│ Selector │────▶│ Structurer│────▶│  System  │
 │ collect  │     │ score &  │     │ template  │     │  Prompt  │
 │ 4 sources│     │ truncate │     │ render    │     └──────────┘
 └──────────┘     └──────────┘     └───────────┘
```

| Stage | Action |
|:-----|:-----|
| **Gather** | Collect `ContextPacket`s from: system prompt, memory, RAG, notes (prioritizes blockers), last 32 history messages |
| **Select** | Score = `relevance_weight × Jaccard` + `recency_weight × expDecay(time)`. Truncate within token budget. Semantic memory skips recency decay. |
| **Structure** | Render into sections: [Role & Policies] → [Task] → [Evidence] → [Context] → [Output] |

---

## 🔧 Tool System & Chain

```go
type Tool interface {
    Name() string
    Description() string
    Run(params map[string]interface{}) (string, error)
    Parameters() []ToolParameter
}
```

Schemas auto-convert to OpenAI Function Calling JSON Schema via `ToolToOpenAISchema()`.

### Chain: Multi-Step Tool Orchestration

```
Chain: "rag_research"
  ├── Step 1: web_search("topic")  ──(storeAs: web)──┐
  ├── Step 2: rag.search("topic")  ──(storeAs: docs) │
  │                                                   │
  └── Step 3: analyze                                  │
        prompt: "Synthesize $steps.web and             │
                $steps.docs into a report" ◀───────────┘
```

| Mode | Behavior | Constraint |
|:-----|:-----|:-----|
| **Serial** | Execute in order; `$steps.xxx` references prior outputs | Can only reference completed steps |
| **Parallel** | Adjacent `Parallel=true` steps run concurrently | No cross-referencing within a parallel group |

Compile-time validation catches: duplicate `StoreAs` names, parallel-group circular dependencies, and undefined `$steps.xxx` references.

---

## 🗂 Configuration

All config is typed struct literals — no YAML, no global state. Zero-value fields get sensible defaults; `.ApplyEnv()` fills in empty values from environment variables.

```go
// LLM: explicit fields + env fallback
llm, _ := core.NewLLM(core.LLMConfig{
    ModelID: "gpt-5.4",
}.ApplyEnv())  // fills APIKey, BaseURL from MODEL_API_KEY / MODEL_BASE_URL

// Memory: Driver + Options plugin pattern — swap backends with zero code changes
memoryCfg := core.MemoryConfig{
    Structured:  core.DriverConfig{Driver: "sqlite", Options: map[string]any{"db_path": "./data/memory.db"}},
    Embedding:   core.DriverConfig{Driver: "openai", Options: map[string]any{"model_id": "text-embedding-3-small"}},
    VectorStore: core.DriverConfig{Driver: "qdrant", Options: map[string]any{"host": "127.0.0.1", "port": 6333}},
    Graph:       core.DriverConfig{Driver: "neo4j", Options: map[string]any{"url": "http://..."}},
}

// Context: zero-value = defaults (100k tokens, 0.1 reserve, 0.3/0.7 weights)
agent, _ := agents.NewReActAgent("demo", llm, core.ContextConfig{}, registry, 64, "", "session-1")
```

All storage backends use a **Driver + Options** plugin pattern — set `Driver: "sqlite"` and the factory auto-creates it. Add custom backends with `factory.RegisterStructuredStore("postgres", ...)` in your package's `init()` and all tools pick it up automatically.

---

## 💡 Design Highlights

- **Pluggable backends**: `store/factory` registry — `init()`-based driver registration; adding a new DB/vector store requires zero changes to core code
- **Session isolation**: `MemoryTool` holds `map[string]*memory.Manager`; `_session_id` auto-routes tool calls to the correct session
- **Concurrency**: `RWMutex` + `atomic.Bool` (CAS compression lock) for WorkingMemory; `WaitGroup` for parallel tool chains
- **Input safety**: SQLite table/column name validation (injection prevention); tool param type validation + default fill + unknown param stripping
- **Token estimation**: CJK chars ≈ 2 tokens, non-CJK ≈ 0.25 tokens per character
- **Memory guard**: Agent message history truncates to last 512 when exceeding 1024
- **Terminal safety**: `TerminalTool` enforces workspace sandbox (`cd` boundary check via `filepath.Rel`) and optional command whitelist
- **Timezone**: All timestamps use `core.Now()` (Asia/Shanghai with CST fallback)

---

## 📊 Module Dependency Graph

```
 ┌────────────────────────────────────────────────────────────────┐
 │                    Dependency direction →                      │
 │                                                               │
 │  agents ─────────────────────────────────────────────┐        │
 │    └─ depends: core, ctx/gssc, tools, tools/builtins │        │
 │  ctx/gssc ──────────────────────────────┐            │        │
 │    └─ depends: core, ctx, tools/builtins │            │        │
 │  tools/builtins ────────────┐           │            │        │
 │    └─ depends: core, tools, memory      │            │        │
 │  tools ─────────┐           │           │            │        │
 │    └─ depends: core         │           │            │        │
 │  memory ────────┤           │           │            │        │
 │    └─ depends: core         │           │            │        │
 │    ├─ types: core, store, retrieval     │            │        │
 │    ├─ rag: core, store                 │            │        │
 │    ├─ retrieval: ext. gse              │            │        │
 │    └─ store/impl: store interface      │            │        │
 │  core ─────────────────────────────────┴───────────┴────────  │
 │    Zero internal dependencies                                 │
 └────────────────────────────────────────────────────────────────┘
```

---

