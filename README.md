# LLM Wiki

A personal Wikipedia powered by LLMs. Feed it GitHub repos, documents, and URLs — get back a searchable, compounding knowledge base.

## Architecture

```
Source Layer → Compilation Layer → Wiki Layer → Query Layer
```

- **Source Layer**: GitHub repos, local Markdown files, URLs (read-only)
- **Compilation Layer**: LLM processes documents into structured wiki pages
- **Wiki Layer**: Markdown files organized by namespace (entity pages, summary pages)
- **Query Layer**: Ask questions, LLM reads relevant wiki pages and answers

## Quick Start

```bash
# Add a GitHub repo as source
llm-wiki source add https://github.com/owner/repo

# Compile all sources into wiki pages
llm-wiki compile

# Query the wiki
llm-wiki query "What is the main purpose of this project?"

# Ask in interactive mode
llm-wiki ask
```

## Features

- Multi-source aggregation (GitHub repos, local files, URLs)
- Incremental compilation (only processes new/changed documents)
- Cross-reference maintenance (wiki pages link to each other)
- Namespace organization (by repo, by topic)
- No vector database required — structured Markdown storage

## Project Status

Phase 1: Project skeleton and source management
