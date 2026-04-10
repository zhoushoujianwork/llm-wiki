# OpenClaw Skill Installation Guide

LLM Wiki can be installed as an OpenClaw skill, enabling AI agents to use it for knowledge base operations.

## Skill Location

The official skill is located at:
- **GitHub**: https://github.com/zhoushoujianwork/llm-wiki
- **Skill file**: `skills/llm-wiki/SKILL.md` (in OpenClaw workspace)

## Installation Methods

### Method 1: Clone to OpenClaw Workspace

```bash
# Clone llm-wiki repo
cd ~/.openclaw/workspace
git clone https://github.com/zhoushoujianwork/llm-wiki.git llm-wiki-repo

# Build and install binary
cd llm-wiki-repo
go build -o llm-wiki ./cmd/llm-wiki
sudo install -m 755 llm-wiki /usr/local/bin/llm-wiki

# Create skill directory
mkdir -p ~/.openclaw/workspace/skills/llm-wiki
```

Then copy or create `SKILL.md` in `~/.openclaw/workspace/skills/llm-wiki/`.

### Method 2: Manual Skill Installation

If you already have the skill definition:

```bash
# Ensure skill directory exists
mkdir -p ~/.openclaw/workspace/skills/llm-wiki

# Copy SKILL.md from repo or create it
cp <path-to-SKILL.md> ~/.openclaw/workspace/skills/llm-wiki/SKILL.md

# Install the CLI binary
sudo install -m 755 llm-wiki /usr/local/bin/llm-wiki
```

### Method 3: Pre-built Binary

Download from GitHub Releases:

```bash
# macOS arm64
curl -LO https://github.com/zhoushoujianwork/llm-wiki/releases/latest/download/llm-wiki-darwin-arm64
sudo install -m 755 llm-wiki-darwin-arm64 /usr/local/bin/llm-wiki

# Linux amd64
curl -LO https://github.com/zhoushoujianwork/llm-wiki/releases/latest/download/llm-wiki-linux-amd64
sudo install -m 755 llm-wiki-linux-amd64 /usr/local/bin/llm-wiki
```

## Configuration

Before using the skill, configure LLM API access:

```bash
# Environment variables (recommended)
export ANTHROPIC_API_KEY=your-key
export ANTHROPIC_BASE_URL=https://api.anthropic.com/v1/messages
export ANTHROPIC_MODEL=claude-3-5-sonnet-20241022

# Or create config file
cat > ~/.openclaw/workspace/skills/llm-wiki/llm-wiki.yaml << EOF
anthropic_base_url: "https://api.anthropic.com/v1/messages"
anthropic_api_key: "your-key"
anthropic_model: "claude-3-5-sonnet-20241022"
wiki_dir: ~/.openclaw/workspace/llm-wiki-data/wiki
sources_dir: ~/.openclaw/workspace/llm-wiki-data/sources
EOF
```

## Verification

Test the installation:

```bash
# Check binary
llm-wiki --help

# Test source add (no API key needed)
llm-wiki source add https://github.com/openclaw/openclaw

# List sources
llm-wiki source list

# Compile (requires API key)
llm-wiki compile
```

## Skill Usage in OpenClaw

Once installed, the skill will be available to OpenClaw agents:

1. **Automatic detection**: OpenClaw scans `~/.openclaw/workspace/skills/*/SKILL.md`
2. **Skill activation**: When user asks about building knowledge bases, compiling docs, or wiki queries
3. **Agent reads SKILL.md**: Follows instructions to use `llm-wiki` CLI

## Common Use Cases

```bash
# Build knowledge base from GitHub repos
llm-wiki source add https://github.com/owner/repo1
llm-wiki source add https://github.com/owner/repo2
llm-wiki compile

# Query the knowledge base
llm-wiki query "how does authentication work?"

# Interactive exploration
llm-wiki ask
```

## Troubleshooting

### Go not found
```bash
# Install Go 1.21+
brew install go  # macOS
sudo apt install go  # Linux
```

### API key errors
```bash
# Verify key is set
echo $ANTHROPIC_API_KEY

# Test with a simple request
curl -X POST "$ANTHROPIC_BASE_URL" \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}'
```

### Permission denied
```bash
# Fix binary permissions
sudo chmod 755 /usr/local/bin/llm-wiki

# Fix skill directory
chmod -R 755 ~/.openclaw/workspace/skills/llm-wiki
```

## Related Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) - Full architecture details
- [AGENTS.md](../AGENTS.md) - Guide for AI coding assistants
- [README.md](../README.md) - Project overview