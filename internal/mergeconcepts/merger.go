// Package mergeconcepts provides cross-namespace concept merging for llm-wiki.
package mergeconcepts

import (
	"context"
	"fmt"
	"strings"

	"llm-wiki/internal/llm"
	"llm-wiki/internal/wiki"
)

// Merger scans all namespaces and merges pages that describe the same concept
// into a global _concepts/ namespace, preserving originals as local views.
type Merger struct {
	store  *wiki.Store
	client llm.Client
}

// NewMerger creates a new Merger.
func NewMerger(store *wiki.Store, client llm.Client) *Merger {
	return &Merger{store: store, client: client}
}

// MergeResult summarises the outcome of one merge operation.
type MergeResult struct {
	ConceptName  string
	SourcePages  []string
	MergedPage   string
	Skipped      bool
	SkipReason   string
}

// Run performs the full merge-concepts pipeline and returns the results.
func (m *Merger) Run(ctx context.Context) ([]MergeResult, error) {
	pages, err := m.store.AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to load pages: %w", err)
	}

	// Group pages by normalised title across namespaces.
	groups := groupByTitle(pages)

	var results []MergeResult
	for title, group := range groups {
		// Need at least 2 pages from different namespaces to consider merging.
		if !hasCrossNamespace(group) {
			continue
		}

		same, err := m.areSameConcept(ctx, title, group)
		if err != nil {
			results = append(results, MergeResult{
				ConceptName: title,
				Skipped:     true,
				SkipReason:  fmt.Sprintf("LLM error: %v", err),
			})
			continue
		}
		if !same {
			results = append(results, MergeResult{
				ConceptName: title,
				Skipped:     true,
				SkipReason:  "LLM determined pages are different concepts",
			})
			continue
		}

		mergedPage, err := m.buildMergedPage(ctx, title, group)
		if err != nil {
			results = append(results, MergeResult{
				ConceptName: title,
				Skipped:     true,
				SkipReason:  fmt.Sprintf("merge build error: %v", err),
			})
			continue
		}

		pagePath, err := m.store.WritePage("_concepts", mergedPage)
		if err != nil {
			results = append(results, MergeResult{
				ConceptName: title,
				Skipped:     true,
				SkipReason:  fmt.Sprintf("write error: %v", err),
			})
			continue
		}

		sources := make([]string, len(group))
		for i, p := range group {
			sources[i] = p.Namespace + "/" + p.Name
		}
		results = append(results, MergeResult{
			ConceptName: title,
			SourcePages: sources,
			MergedPage:  pagePath,
		})
	}

	return results, nil
}

// groupByTitle groups pages by their normalised title (lowercase, trimmed).
func groupByTitle(pages []wiki.Page) map[string][]wiki.Page {
	groups := make(map[string][]wiki.Page)
	for _, p := range pages {
		if p.Namespace == "_concepts" {
			continue // skip already-merged pages
		}
		key := strings.ToLower(strings.TrimSpace(p.Name))
		groups[key] = append(groups[key], p)
	}
	return groups
}

// hasCrossNamespace reports whether pages in a group span more than one namespace.
func hasCrossNamespace(pages []wiki.Page) bool {
	seen := make(map[string]struct{})
	for _, p := range pages {
		seen[p.Namespace] = struct{}{}
	}
	return len(seen) > 1
}

// areSameConcept asks the LLM whether the pages all describe the same concept.
func (m *Merger) areSameConcept(ctx context.Context, title string, pages []wiki.Page) (bool, error) {
	var snippets []string
	for _, p := range pages {
		preview := p.Content
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		snippets = append(snippets, fmt.Sprintf("Namespace: %s\n---\n%s", p.Namespace, preview))
	}

	prompt := fmt.Sprintf(`You are evaluating whether multiple wiki pages all describe the same real-world concept.

Page title: "%s"

Pages:
%s

Answer with exactly one word: YES if they all describe the same concept, NO if they describe different things.`,
		title,
		strings.Join(snippets, "\n\n"),
	)

	resp, err := m.client.Generate(ctx, prompt)
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToUpper(strings.TrimSpace(resp)), "YES"), nil
}

// buildMergedPage creates a merged wiki.Page for the _concepts/ namespace.
func (m *Merger) buildMergedPage(ctx context.Context, title string, pages []wiki.Page) (wiki.Page, error) {
	var sourceSections []string
	var backRefs []string

	for _, p := range pages {
		ref := p.Namespace + "/" + p.Name
		backRefs = append(backRefs, "[["+ref+"]]")

		preview := p.Content
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		sourceSections = append(sourceSections, fmt.Sprintf("### From `%s`\n\n%s", ref, preview))
	}

	// Ask LLM for a unified summary.
	var contentParts []string
	for _, p := range pages {
		contentParts = append(contentParts, p.Content)
	}
	summary, err := m.unifiedSummary(ctx, title, contentParts)
	if err != nil {
		summary = "_Summary unavailable._"
	}

	displayTitle := strings.Title(strings.ReplaceAll(title, "_", " "))

	content := fmt.Sprintf("# %s\n\n## Unified Summary\n\n%s\n\n## Source Pages\n\n%s\n\n## Per-Source Content\n\n%s",
		displayTitle,
		summary,
		strings.Join(backRefs, "\n"),
		strings.Join(sourceSections, "\n\n"),
	)

	return wiki.Page{
		Namespace: "_concepts",
		Name:      title,
		Type:      "concept",
		Content:   content,
		Links:     backRefs,
	}, nil
}

// unifiedSummary asks the LLM to produce a concise unified summary.
func (m *Merger) unifiedSummary(ctx context.Context, concept string, contents []string) (string, error) {
	combined := strings.Join(contents, "\n\n---\n\n")
	if len(combined) > 2000 {
		combined = combined[:2000] + "..."
	}

	prompt := fmt.Sprintf(`Write a concise 2-3 sentence unified summary of the concept "%s" based on these source documents. Return only the summary, no extra commentary.

Sources:
%s`, concept, combined)

	return m.client.Generate(ctx, prompt)
}
