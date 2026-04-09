package compiler

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zhoushoujianwork/llm-wiki/internal/llm"
	"github.com/zhoushoujianwork/llm-wiki/internal/source"
	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

// Compiler handles LLM-powered document compilation.
type Compiler struct {
	client *llm.AnthropicClient
}

type compilationResponse struct {
	Summary         string              `json:"summary"`
	Tags            []string            `json:"tags"`
	CrossReferences []string            `json:"cross_references"`
	Entities        []compiledEntityRef `json:"entities"`
}

type compiledEntityRef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// NewCompiler creates a new compiler instance.
func NewCompiler() *Compiler {
	return &Compiler{client: llm.NewAnthropicClient()}
}

// CompileDocument compiles a single document into wiki pages.
func (c *Compiler) CompileDocument(doc source.Document) ([]wiki.Page, error) {
	content, err := readDocumentContent(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}

	req := llm.MessageRequest{
		System: compilationSystemPrompt,
		User:   buildCompilationPrompt(doc, string(content)),
	}

	resp, err := c.client.Message(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("anthropic compilation request failed: %w", err)
	}

	var parsed compilationResponse
	if err := llm.UnmarshalJSONObject(resp.Text, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse compilation response: %w", err)
	}

	return c.buildPages(doc, parsed), nil
}

func (c *Compiler) buildPages(doc source.Document, compiled compilationResponse) []wiki.Page {
	summaryPageName := pathSlug(doc.RelPath)
	displayTitle := slugify(doc.RelPath)

	pages := []wiki.Page{
		{
			Namespace: doc.SourceID,
			Name:      summaryPageName,
			Type:      "summary",
			Content:   generateSummaryPage(displayTitle, doc.RelPath, compiled),
			Links:     dedupeStrings(append(entityPageNames(summaryPageName, compiled.Entities), compiled.CrossReferences...)),
			Tags:      dedupeStrings(compiled.Tags),
		},
	}

	for _, entity := range compiled.Entities {
		if strings.TrimSpace(entity.Name) == "" {
			continue
		}

		pageName := entityPageName(summaryPageName, entity.Name)
		related := []string{summaryPageName}
		for _, other := range compiled.Entities {
			otherName := strings.TrimSpace(other.Name)
			if otherName == "" || strings.EqualFold(otherName, entity.Name) {
				continue
			}
			related = append(related, entityPageName(summaryPageName, otherName))
		}
		related = append(related, compiled.CrossReferences...)

		pages = append(pages, wiki.Page{
			Namespace: doc.SourceID,
			Name:      pageName,
			Type:      "entity",
			Content:   generateEntityPage(entity, summaryPageName, doc.RelPath, related),
			Links:     dedupeStrings(related),
			Tags:      dedupeStrings(append(compiled.Tags, entity.Tags...)),
		})
	}

	return pages
}

func readDocumentContent(doc source.Document) ([]byte, error) {
	if len(doc.Content) > 0 {
		return doc.Content, nil
	}
	return os.ReadFile(doc.Path)
}

func buildCompilationPrompt(doc source.Document, content string) string {
	return fmt.Sprintf(`Compile this source document into structured wiki metadata.

Document path: %s
Document type: %s

Return JSON only using this exact shape:
{
  "summary": "1-2 sentence summary",
  "tags": ["tag-a", "tag-b"],
  "cross_references": ["Existing Page", "Other Concept"],
  "entities": [
    {
      "name": "Entity Name",
      "description": "1-2 sentence description grounded in the document",
      "tags": ["tag-a"]
    }
  ]
}

Rules:
- Keep the summary concise and factual.
- Extract the most important entities or concepts only.
- "cross_references" must be page titles without brackets. They will later be rendered as [[Page Name]] links.
- Tags should be short lowercase categories.
- If a field has no values, return an empty array.
- Do not include Markdown fences or extra commentary.

Document content:
<document>
%s
</document>`, doc.RelPath, doc.Type, content)
}

func generateSummaryPage(displayTitle, relPath string, compiled compilationResponse) string {
	var sections []string
	sections = append(sections, fmt.Sprintf("# %s", displayTitle))
	sections = append(sections, "## Summary\n"+strings.TrimSpace(compiled.Summary))
	sections = append(sections, "## Source\n`"+relPath+"`")

	if len(compiled.Tags) > 0 {
		sections = append(sections, "## Tags\n"+renderBulletList(compiled.Tags))
	}

	if len(compiled.Entities) > 0 {
		var entityLinks []string
		for _, entity := range compiled.Entities {
			name := strings.TrimSpace(entity.Name)
			if name == "" {
				continue
			}
			entityLinks = append(entityLinks, "[["+entityPageName(pathSlug(relPath), name)+"]]")
		}
		if len(entityLinks) > 0 {
			sections = append(sections, "## Key Entities\n"+renderBulletList(entityLinks))
		}
	}

	if len(compiled.CrossReferences) > 0 {
		sections = append(sections, "## Related Pages\n"+renderBracketedLinks(compiled.CrossReferences))
	}

	return strings.Join(sections, "\n\n")
}

func generateEntityPage(entity compiledEntityRef, summaryPageName, relPath string, related []string) string {
	title := strings.TrimSpace(entity.Name)
	if title == "" {
		title = "Untitled Entity"
	}

	var sections []string
	sections = append(sections, "# "+title)
	sections = append(sections, "## Summary\n"+strings.TrimSpace(entity.Description))
	sections = append(sections, "## Source Document\n[["+summaryPageName+"]]\n\nOriginal path: `"+relPath+"`")

	if len(entity.Tags) > 0 {
		sections = append(sections, "## Tags\n"+renderBulletList(entity.Tags))
	}

	if len(related) > 0 {
		sections = append(sections, "## Related Pages\n"+renderBracketedLinks(related))
	}

	return strings.Join(sections, "\n\n")
}

func renderBracketedLinks(items []string) string {
	var links []string
	for _, item := range dedupeStrings(items) {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		links = append(links, "[["+name+"]]")
	}
	return renderBulletList(links)
}

func renderBulletList(items []string) string {
	uniq := dedupeStrings(items)
	if len(uniq) == 0 {
		return ""
	}

	lines := make([]string, 0, len(uniq))
	for _, item := range uniq {
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}

func entityPageNames(summaryPageName string, entities []compiledEntityRef) []string {
	names := make([]string, 0, len(entities))
	for _, entity := range entities {
		name := strings.TrimSpace(entity.Name)
		if name == "" {
			continue
		}
		names = append(names, entityPageName(summaryPageName, name))
	}
	return names
}

func pageSlug(name string) string {
	return pathSlug(strings.TrimSpace(name))
}

func entityPageName(summaryPageName, entityName string) string {
	return summaryPageName + "__" + pageSlug(entityName)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	uniq := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, trimmed)
	}
	sort.Strings(uniq)
	return uniq
}

// slugify converts a path to a readable title.
func slugify(path string) string {
	name := strings.TrimSuffix(path, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return name
}

// pathSlug converts a path or title to a safe filename.
func pathSlug(path string) string {
	name := strings.TrimSuffix(path, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	dangerous := []string{"/", " ", "[", "]", "(", ")", "#", ":"}
	for _, d := range dangerous {
		name = strings.ReplaceAll(name, d, "_")
	}
	name = strings.Trim(name, "_")
	if name == "" {
		return "untitled"
	}
	return name
}

const compilationSystemPrompt = `You transform source documents into structured wiki compilation metadata.
Return strict JSON only. Do not wrap the response in Markdown fences.`
