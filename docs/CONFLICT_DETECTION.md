# Conflict Detection Service

## Overview

The Conflict Detection Service is a Phase 1 implementation of the enhancement requested in [issue #2](https://github.com/zhoushoujianwork/llm-wiki/issues/2). It provides automated conflict detection capabilities to ensure consistency across the LLM Wiki knowledge base.

## Background

As described by Andrej Karpathy's [knowledge compilation theory](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f), wiki pages are "compiled" from source documents using LLMs. However, this process can lead to inconsistencies:

- Different sources may contain contradictory information about the same entity
- LLM-generated summaries may introduce conflicts with existing content
- Entity definitions may drift over time as new sources are added

The Conflict Detection Service addresses these issues by systematically identifying and reporting potential contradictions.

## Architecture

```
┌─────────────────────────────────────────────┐
│   ConflictDetectionService                  │
│                                             │
│  ┌──────────────┐    ┌──────────────────┐  │
│  │ scanAllPages │───▶│ CheckEntity      │  │
│  └──────────────┘    │ (pairwise check) │  │
│                      └──────────────────┘  │
│                            │                │
│                            ▼                │
│                    ┌──────────────┐         │
│                    │LLM Validation│         │
│                    │(semantic     │         │
│                    │ comparison)  │         │
│                    └──────────────┘         │
└─────────────────────────────────────────────┘
```

## Components

### `ConflictDetector`

The main detector that orchestrates conflict detection:

```go
type ConflictDetector struct {
    LLMClient llm.Client      // Anthropic API client for semantic analysis
    WikiStore WikiStoreProvider // Access to wiki pages and entities
    CacheDir  string          // Directory for cached results
}
```

### `Conflict`

Represents a detected inconsistency:

```go
type Conflict struct {
    EntityName     string    // The entity being checked
    Confidence     float64   // 0.0-1.0 confidence score
    PageA          string    // First page path
    StatementA     string    // Statement from page A
    PageB          string    // Second page path
    StatementB     string    // Statement from page B
    Recommendation string    // "merge", "manual_review", "ignore"
    CreatedAt      time.Time
}
```

### `Report`

Aggregates all detected conflicts:

```go
type Report struct {
    TotalPages    int           // Pages scanned
    TotalEntities int           // Unique entities checked
    Conflicts     []Conflict    // Detected conflicts
    Summary       Summary       // High-level statistics
    Timestamp     time.Time     // When report was generated
}
```

## Usage

### CLI Command

```bash
# Full scan with default text output
llm-wiki check-conflicts

# JSON output for programmatic use
llm-wiki check-conflicts --output=json

# Markdown report for documentation
llm-wiki check-conflicts --output=markdown
```

### Programmatic API

```go
import "llm-wiki/internal/conflicts"

// Initialize detector
detector := conflicts.NewConflictDetector(llmClient, wikiStore, cacheDir)

// Scan all pages
ctx := context.Background()
report, err := detector.ScanAllPages(ctx)
if err != nil {
    log.Fatal(err)
}

// Check specific entity
conflicts, err := detector.CheckEntity(ctx, "machine_learning")
if err != nil {
    log.Fatal(err)
}

// Process results
for _, c := range report.Conflicts {
    fmt.Printf("Entity: %s, Confidence: %.0f%%\n", 
        c.EntityName, c.Confidence*100)
}
```

## Algorithm

### 1. Entity-Based Scanning

The service uses an entity-indexed approach:

1. **Group pages by entity**: All pages mentioning the same entity are grouped together
2. **Pairwise comparison**: Each pair of pages is compared for conflicting statements
3. **LLM validation**: For each pair, an LLM evaluates whether statements contradict

### 2. Statement Extraction

For a given entity E and page P:

```python
def extract_relevant_statements(content, entity):
    sentences = split_by_sentence_boundaries(content)
    return [s for s in sentences if entity.lower() in s.lower()]
```

### 3. Conflict Detection

The core detection algorithm compares statements S_A and S_B:

```
prompt = f"""
Analyze these statements about '{entity}' from different wiki pages:

## Page A Statements:
1. {statement_a_1}
...

## Page B Statements:
1. {statement_b_1}
...

Task: Identify any contradictions or logical inconsistencies.
Return JSON with hasConflict, confidence, reason, recommendation.
"""
response = LLM.generate(prompt)
```

### 4. Confidence Scoring

Confidence is derived from:
- Number of contradictory indicators found
- Semantic distance between statements
- LLM response certainty

Range: `0.0` (no conflict) to `1.0` (clear contradiction)

### 5. Recommendations

| Confidence | Recommendation | Action |
|------------|----------------|--------|
| ≥ 0.8      | `manual_review` | Requires human judgment |
| 0.5 - 0.8  | `merge` | Can be auto-resolved |
| < 0.5      | `ignore` | Likely false positive |

## Caching Strategy (Future)

Phase 2 will implement result caching:

```yaml
cache_dir: ~/.llm-wiki/cache/conflicts/
  entity_name_timestamp.json: cached report
  timestamp.txt: last scan time
```

Cached results are valid for 7 days unless source files change.

## Testing

### Unit Tests

```bash
cd internal/conflicts
go test -v
```

Mock implementations of `LLMClient` and `WikiStoreProvider` enable isolated testing.

### Integration Tests

```bash
# Create test wiki with known conflicts
mkdir -p tmp/wiki
cp test_data/* tmp/wiki/

# Run detector
go test -run TestIntegrationScanAllPages -v
```

## Limitations

1. **Entity Index Dependency**: Requires accurate entity-to-page mapping
2. **Performance**: O(n²) pairwise comparisons for n pages per entity
3. **False Positives**: Similar wording ≠ contradiction; requires careful review
4. **Missing Context**: LLM may miss nuances requiring domain knowledge

## Future Enhancements (Phases 2-3)

### Phase 2: Quality Evaluation

Add quality scoring for each wiki page based on:
- Cross-reference count
- Update frequency
- User feedback signals
- Citation completeness

### Phase 3: Scheduled Corrections

Implement automated correction pipeline:
- Weekly full scans
- Daily entity-specific checks
- Auto-merge low-confidence conflicts
- Queue high-confidence conflicts for review

## Related Documentation

- [Issue #2](https://github.com/zhoushoujianwork/llm-wiki/issues/2) - Enhancement request
- [Karpathy's LLM Wiki Gist](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) - Core concept
- [Internal Wiki API](../internal/wiki/) - Page storage interface
- [LLM Client](../internal/llm/) - Anthropic API integration

## License

Apache 2.0
