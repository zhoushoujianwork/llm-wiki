# AGENTS.md — For AI Coding Assistants

This project follows the LLM Wiki architecture described in `/Volumes/1TB/github/llm-wiki/docs/ARCHITECTURE.md`.

## Key Principles

1. **Source Layer is read-only**: Never modify source files; only read them.
2. **Compilation is idempotent**: Re-running compile should be safe and incremental.
3. **Wiki pages are the source of truth for queries**: Query layer reads wiki, not raw sources.
4. **No vector database**: Use LLM for retrieval via structured wiki pages.
5. **Namespace isolation**: Each source repo gets its own namespace in the wiki.

## Directory Structure

```
cmd/llm-wiki/      # CLI entry point
internal/
  source/          # GitHub repo management, file discovery
  compiler/        # LLM compilation engine
  wiki/            # Wiki page storage and retrieval
  query/           # Query processing and answering
  index/           # Concept/entity to page mapping
sources/           # Cloned repos and cached sources (gitignored)
wiki/              # Generated wiki pages (committable)
```

## Compilation Flow

1. Discover documents in source layer
2. For each document, check if it needs compilation (mtime, existing wiki page)
3. Send document to LLM with compilation prompt
4. LLM returns: summary page, entity pages, cross-reference updates
5. Write wiki pages to `wiki/` directory
6. Update index

## Query Flow

1. User asks question
2. Identify relevant namespaces/pages from index
3. Read top 3-5 relevant wiki pages
4. Send pages + question to LLM
5. Return answer with source citations

## Adding New Sources

```bash
llm-wiki source add <repo-url>     # GitHub repo
llm-wiki source add <local-path>   # Local directory
llm-wiki source add <url>          # URL to scrape
```

## Compilation Modes

- `full`: Re-compile everything, overwrite existing pages
- `incremental`: Only compile new/changed sources (default)
- `single <source>`: Compile specific source only
