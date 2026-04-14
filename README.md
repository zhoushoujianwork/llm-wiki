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
# Build & install to ~/go/bin (version injected from git tag)
make build

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

### Default Paths (Privacy-First)

By default, llm-wiki outputs to **user's private directory** to protect your personal wiki:

```
~/.llm-wiki/
  ├── wiki/           # LLM-generated wiki pages (private)
  ├── sources/        # Cloned repos cache (private)
  └── llm-wiki.yaml   # Your config file
```

This ensures your compiled knowledge base stays private and won't be accidentally committed to GitHub.

### Custom Configuration

Create `llm-wiki.yaml` to override defaults:

```yaml
# LLM Provider (Anthropic API compatible)
anthropic_base_url: "https://api.anthropic.com/v1/messages"
anthropic_api_key: "your-api-key"
anthropic_model: "claude-3-5-sonnet-20241022"

# Directories (optional, defaults shown above)
wiki_dir: ~/.llm-wiki/wiki
sources_dir: ~/.llm-wiki/sources
```

Config file locations (searched in order):
1. `--config` flag
2. `./llm-wiki.yaml` (current directory)
3. `~/.llm-wiki/llm-wiki.yaml`
4. `~/.openclaw/workspace/skills/llm-wiki/llm-wiki.yaml`

Or use environment variables:

```bash
export ANTHROPIC_API_KEY=your-key
export ANTHROPIC_BASE_URL=https://api.anthropic.com/v1/messages
export ANTHROPIC_MODEL=claude-3-5-sonnet-20241022
export LLM_WIKI_DIR=/custom/wiki/path
export LLM_WIKI_SOURCES_DIR=/custom/sources/path
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

## Release

Releases are managed via git tags. Pushing a `v*` tag triggers GitHub Actions to cross-compile binaries for all platforms and publish a GitHub Release automatically.

```bash
git tag v1.0.0
git push origin v1.0.0
```

Binaries published per release:
- `llm-wiki-linux-amd64`
- `llm-wiki-darwin-arm64`
- `llm-wiki-darwin-amd64`
- `llm-wiki-windows-amd64.exe`

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

## OpenClaw Skill Installation

LLM Wiki can be installed as an [OpenClaw](https://github.com/openclaw/openclaw) skill for AI agent usage.

See [docs/SKILL_INSTALLATION.md](docs/SKILL_INSTALLATION.md) for detailed installation guide.

Quick install:

```bash
# Build and install
go build -o llm-wiki ./cmd/llm-wiki
sudo install -m 755 llm-wiki /usr/local/bin/llm-wiki

# Create skill directory
mkdir -p ~/.openclaw/workspace/skills/llm-wiki
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

## MCP Server (Claude Desktop / Cursor)

llm-wiki can run as an [MCP](https://modelcontextprotocol.io) server, exposing your wiki as live tools to any MCP-compatible AI client.

### Start the server

```bash
llm-wiki serve --mcp
```

### Claude Desktop configuration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "llm-wiki": {
      "command": "llm-wiki",
      "args": ["serve", "--mcp"]
    }
  }
}
```

Restart Claude Desktop. You will see three new tools available:

| Tool | Description |
|------|-------------|
| `wiki_query` | Search pages relevant to a question |
| `wiki_list_pages` | List all pages (optional namespace filter) |
| `wiki_read_page` | Read a page's full content by path |
