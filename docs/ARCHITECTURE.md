# Architecture

LLM Wiki follows a 4-layer architecture inspired by Andrej Karpathy's compilation pattern.

## Layer Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Layer 1: SOURCE LAYER (Read-Only)                         │
│  GitHub repos, local files, URLs                           │
│  - github: cloned via git                                   │
│  - local: path-based                                       │
│  - url: scraped content                                    │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Layer 2: COMPILATION LAYER (LLM Processing)               │
│  Transform raw documents → structured wiki pages          │
│  - Document → Summary page (1-liner + key points)          │
│  - Document → Entity pages (key concepts/entities)         │
│  - Cross-reference updates (link to related pages)        │
│  - Incremental: only process changed/new docs             │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Layer 3: WIKI LAYER (Structured Markdown)                 │
│  Human-readable, git-friendly storage                      │
│  - Organized by namespace (source repo / topic)           │
│  - Page types: summary, entity, concept                   │
│  - Wiki-style links [[Page Name]]                         │
│  - No vector database required                             │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│  Layer 4: QUERY LAYER (LLM Synthesis)                      │
│  User question → relevant pages → synthesized answer       │
│  - Identify relevant namespaces/pages                      │
│  - Read top 3-5 pages                                      │
│  - LLM synthesizes answer with source citations            │
└─────────────────────────────────────────────────────────────┘
```

## Key Design Decisions

### No Vector Database
Traditional RAG uses embeddings + cosine similarity for retrieval. LLM Wiki instead relies on:
- **Structured pages**: Each page has clear boundaries and meaning
- **LLM-driven retrieval**: At query time, LLM reads relevant pages and synthesizes
- **Cross-links**: Wiki pages link to each other, enabling navigation

This trades some retrieval speed for better comprehension (LLM reads clean pages, not raw chunks).

### Incremental Compilation
- Source documents are never modified
- Each document's compilation is idempotent
- Checksum/mtime tracking determines what needs recompilation
- Re-compiling doesn't delete existing pages unless explicitly requested

### Namespace Isolation
Each source (repo, topic) gets its own namespace directory:
```
wiki/
├── my-github-repo/
│   ├── overview.md
│   ├── api-design.md
│   └── authentication.md
├── local-notes/
│   └── meeting-notes-2024.md
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `source add <url>` | Add GitHub repo or URL as source |
| `source list` | List all sources |
| `source sync` | Pull latest from all GitHub repos |
| `compile` | Compile all sources → wiki pages |
| `query <question>` | Ask a question |
| `ask` | Interactive query mode |

## Data Flow

```
User adds source
    ↓
GitHub: git clone → sources/
URL: scrape → sources/
    ↓
User runs compile
    ↓
For each document:
    Read content
    Send to LLM with compilation prompt
    LLM returns structured pages
    Write pages to wiki/
    Update index
    ↓
User queries
    ↓
Find relevant pages (via index or LLM-guided)
Read page content
Send to LLM with question
Return synthesized answer
```
