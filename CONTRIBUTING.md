# Contributing to LLM Wiki

## Development Setup

```bash
git clone https://github.com/zhoushoujianwork/llm-wiki
cd llm-wiki
go mod tidy
go build -o llm-wiki ./cmd/llm-wiki/
```

## Testing

```bash
# Run from skill directory with config
llm-wiki source add https://github.com/owner/repo
llm-wiki compile
llm-wiki query "your question"
```

## Project Structure

- `cmd/llm-wiki/` — CLI commands
- `internal/source/` — Source management (GitHub repos, local files)
- `internal/compiler/` — LLM compilation engine
- `internal/wiki/` — Wiki page storage
- `internal/query/` — Query engine
- `internal/llm/` — Anthropic API client

## Adding New LLM Providers

LLM communication uses the Anthropic Messages API protocol. Any provider supporting it works:
- Anthropic (Claude)
- MiniMax
- Groq
- Other Anthropic-compatible providers

Set `ANTHROPIC_BASE_URL` and `ANTHROPIC_API_KEY` in config.
