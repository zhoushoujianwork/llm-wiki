# LLM Wiki

> Build a personal Wikipedia powered by LLMs. Feed it GitHub repos, documents, and URLs — get back a searchable, compounding knowledge base.

Inspired by Andrej Karpathy's [LLM Wiki concept](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f).

## How It Works

```
┌──────────────────────────────────────────────┐
│  Layer 1: SOURCE LAYER (Read-Only)           │
│  GitHub repos, local files, URLs            │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│  Layer 2: COMPILATION LAYER (LLM)          │
│  Document → Summary + Entity + Concept pages │
│  Cross-references maintained automatically   │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│  Layer 3: WIKI LAYER (Markdown)             │
│  Structured pages organized by namespace     │
│  Git-friendly, no vector database needed     │
└──────────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────┐
│  Layer 4: QUERY LAYER (LLM Synthesis)      │
│  Find relevant pages → LLM synthesizes answer│
│  with source citations                       │
└──────────────────────────────────────────────┘
```

**Key difference from RAG**: Instead of retrieving raw text chunks at query time, documents are "compiled" into structured wiki pages upfront. The LLM works with clean, dense summaries rather than noisy raw prose.

## Features

- 🌐 **Multi-source aggregation** — GitHub repos, local files, URLs
- 🔄 **Incremental compilation** — Only re-processes changed documents
- 🔗 **Cross-reference maintenance** — Pages link to each other automatically
- 📦 **No vector database** — Just Markdown files and an LLM
- 🔍 **LLM-powered query** — Understands intent, not just keywords
- ⚡ **Fallback search** — Works without LLM API key via keyword search

## Quick Start

```bash
# Install
go install

# Add a GitHub repo as source
llm-wiki source add https://github.com/owner/repo

# Compile all sources into wiki pages (requires LLM API key)
llm-wiki compile

# Query the wiki
llm-wiki query "what is this project about?"

# Or enter interactive mode
llm-wiki ask
```

## Configuration

Create `llm-wiki.yaml` in the skill directory or current folder:

```yaml
# LLM Provider (any Anthropic API compatible provider)
anthropic_base_url: "https://api.minimaxi.com/anthropic/v1/messages"
anthropic_api_key: "your-api-key"
anthropic_model: "MiniMax-M2.7"

# Directories
wiki_dir: ./wiki
sources_dir: ./sources
```

Or use environment variables:

```bash
export ANTHROPIC_API_KEY=your-key
export ANTHROPIC_BASE_URL=https://api.minimaxi.com/anthropic/v1/messages
export ANTHROPIC_MODEL=MiniMax-M2.7
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `source add <url>` | Add GitHub repo, URL, or local path |
| `source list` | List all sources |
| `source sync` | Pull latest from all GitHub repos |
| `compile` | Compile all sources → wiki pages |
| `compile <source>` | Compile specific source only |
| `query <question>` | Query the wiki (single shot) |
| `ask` | Interactive query mode |

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for full details.

```
cmd/llm-wiki/       # CLI entry point
internal/
  source/            # GitHub repo management, file discovery
  compiler/          # LLM compilation engine
  wiki/              # Wiki page storage and retrieval
  query/             # Query processing and answering
  index/             # Concept/entity to page mapping
  llm/               # Anthropic API client
```

## Why "Compilation"?

Think of it like a compiler for knowledge:

| Compiler | LLM Wiki |
|----------|----------|
| Source code | Raw documents |
| Compilation | LLM processing |
| Binary | Wiki pages |
| Runtime | Query time |

The LLM does the heavy lifting *before* the query, not during it.

## License

Apache 2.0
