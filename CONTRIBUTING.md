# Contributing to LLM Wiki

## Development Setup

```bash
git clone https://github.com/zhoushoujianwork/llm-wiki
cd llm-wiki
go mod tidy

# Build & install to ~/go/bin (version injected from git tag)
make build

# Verify
llm-wiki version
```

## Build Details

`make build` does three things:
1. Compiles the binary to `bin/llm-wiki`
2. Injects the version from `git describe --tags` via `-ldflags`
3. Installs to `~/go/bin/llm-wiki` so it's available system-wide

Ensure `~/go/bin` is in your `PATH`:
```bash
export PATH="$HOME/go/bin:$PATH"
```

## Release Process

Releases are fully automated via `.github/workflows/release.yml`. To publish a new release:

```bash
git tag v1.2.0
git push origin v1.2.0
```

GitHub Actions will:
- Cross-compile for linux-amd64, darwin-arm64, darwin-amd64, windows-amd64
- Create a GitHub Release with auto-generated changelog
- Upload all binaries as release assets

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
