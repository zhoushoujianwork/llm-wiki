package compiler

import (
	"fmt"
	"os"
	"strings"

	"github.com/zhoushoujianwork/llm-wiki/internal/source"
	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

// Compiler handles LLM-powered document compilation
type Compiler struct {
	// TODO: LLM client (Anthropic/Ollama)
}

// NewCompiler creates a new compiler instance
func NewCompiler() *Compiler {
	return &Compiler{}
}

// CompileDocument compiles a single document into wiki pages
func (c *Compiler) CompileDocument(doc source.Document) ([]wiki.Page, error) {
	// Read document content
	content, err := os.ReadFile(doc.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}

	// TODO: Send to LLM for compilation
	// For now, create a basic summary page

	pages := []wiki.Page{
		{
			Namespace: doc.SourceID,
			Name:      pathSlug(doc.RelPath),
			Type:      "summary",
			Content:   c.generateSummaryPage(slugify(doc.RelPath), string(content)),
			Links:     []string{},
		},
	}

	return pages, nil
}

// generateSummaryPage creates a basic summary page from document content
func (c *Compiler) generateSummaryPage(displayTitle, content string) string {
	// Simple extraction: first 500 chars as description
	desc := content
	if len(desc) > 500 {
		desc = desc[:500] + "..."
	}

	return fmt.Sprintf(`# %s

## Summary
%s

## Key Points
<!-- TODO: LLM extract key points -->

## Related
<!-- TODO: Auto-link related entities -->
`, displayTitle, desc)
}

// slugify converts a path to a readable title (for display)
func slugify(path string) string {
	name := strings.TrimSuffix(path, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	// Only replace underscores/hyphens with spaces for display, keep path structure
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return name
}

// pathSlug converts a path to a safe filename (keeps slashes, replaces special chars)
func pathSlug(path string) string {
	// Remove file extension
	name := strings.TrimSuffix(path, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	// Replace dangerous chars but keep path structure
	dangerous := []string{"/", " ", "[", "]", "(", ")", "#"}
	for _, d := range dangerous {
		name = strings.ReplaceAll(name, d, "_")
	}
	return name
}
