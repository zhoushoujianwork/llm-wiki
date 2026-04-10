# Pull Request: Conflict Detection Service (Phase 1)

**Related Issue**: #2 - Suggest adding conflict detection, quality evaluation, and scheduled correction capabilities

---

## 🎯 What This PR Adds

This PR implements **Phase 1** of the enhancement request: a comprehensive **Conflict Detection Service** that identifies inconsistencies across wiki pages using LLM-powered semantic analysis.

### Key Features

| Feature | Description |
|---------|-------------|
| **Entity-Based Scanning** | Groups pages by entity and compares for conflicts |
| **LLM Semantic Validation** | Uses Anthropic API to detect contradictions |
| **Confidence Scoring** | Rates conflicts from 0.0-1.0 |
| **Auto-Recommendations** | Suggests merge/manual_review/ignore actions |
| **CLI Integration** | `llm-wiki check-conflicts [--output=json|markdown]` |

---

## 📦 Files Changed

```
docs/CONFLICT_DETECTION.md          (150 lines) - Architecture & API docs
internal/conflicts/conflicts.go     (330 lines) - Core implementation  
internal/conflicts/conflicts_test.go (280 lines) - Unit tests with mocks
```

---

## 🔧 How It Works

```
1. Entity Index → 2. Group Pages by Entity → 3. Pairwise Comparison → 
4. LLM Validation → 5. Generate Report
```

### Example Usage

```bash
# Full scan
llm-wiki check-conflicts

# JSON output
llm-wiki check-conflicts --output=json

# Markdown report
llm-wiki check-conflicts --output=markdown
```

### Sample Output (Markdown)

```markdown
# Conflict Detection Report

**Generated**: 2026-04-10T16:00:00Z

| Metric | Value |
|--------|-------|
| Pages Scanned | 45 |
| Entities Checked | 23 |
| Conflicts Found | 7 |

## Detected Conflicts

| # | Entity | Page A | Page B | Confidence | Recommendation |
|---|--------|--------|--------|------------|----------------|
| 1 | `python` | `python.md` | `python_alt.md` | 90% | manual_review |
```

---

## ✅ Testing

```bash
cd internal/conflicts
go test -v

# Test coverage includes:
# - No-conflict scenarios
# - Clear contradiction detection
# - Case-insensitive statement matching
# - Confidence classification
# - Mock-based isolation
```

---

## 📊 Performance Considerations

- **Time Complexity**: O(n²) pairwise comparisons per entity
- **Mitigation**: Caching in Phase 2 will reduce redundant scans
- **Optimization**: Parallel processing can be added later

---

## 🔮 Next Steps (Phases 2-3)

| Phase | Goal | Status |
|-------|------|--------|
| **Phase 2** | Quality Evaluation Service | Planned |
| **Phase 3** | Scheduled Correction Pipeline | Planned |

See [CONFLICT_DETECTION.md](docs/CONFLICT_DETECTION.md) for detailed roadmap.

---

## 🏷️ Labels

Please apply:
- `enhancement` - New feature request
- `ready-for-agent` - ClawFlow task complete
- In-progress workflow label can be removed

---

## 📝 Author Notes

This is **Phase 1 of 3** as outlined in issue #2. The service provides automated conflict detection but still requires human review for high-confidence conflicts. Future phases will add:

1. **Quality scoring** for wiki pages
2. **Automated corrections** based on community feedback
3. **Scheduled maintenance** runs

Contributions welcome for Phases 2-3!

---

**Tested on**: Go 1.22+ on Ubuntu 22.04  
**Requires**: Anthropic API key for full functionality  
**Dependencies**: Minimal - uses existing llm-wiki infrastructure
