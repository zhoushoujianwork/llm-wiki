package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/zhoushoujianwork/llm-wiki/internal/llm"
	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

const (
	maxCatalogPages  = 25
	maxSelectedPages = 5
	maxSnippetLength = 280
)

// Engine handles querying the wiki with LLM synthesis.
type Engine struct {
	store  *wiki.Store
	client *llm.AnthropicClient
}

type pageSelectionResponse struct {
	Pages []selectedPage `json:"pages"`
}

type selectedPage struct {
	Identifier string `json:"identifier"`
	Reason     string `json:"reason"`
}

type synthesisResponse struct {
	Answer    string   `json:"answer"`
	Citations []string `json:"citations"`
}

// NewEngine creates a new query engine.
func NewEngine(store *wiki.Store) *Engine {
	return &Engine{
		store:  store,
		client: llm.NewAnthropicClient(),
	}
}

// Ask processes a question and returns an answer.
func (e *Engine) Ask(ctx context.Context, question string) (string, error) {
	allPages, err := e.store.AllPages()
	if err != nil {
		return "", fmt.Errorf("failed to load wiki pages: %w", err)
	}
	if len(allPages) == 0 {
		return "I don't have any relevant information in the wiki yet. Try adding sources and running compile first.", nil
	}

	selected, err := e.selectRelevantPages(ctx, question, allPages)
	if err != nil {
		// LLM not available — fall back to keyword search
		if strings.Contains(err.Error(), "ANTHROPIC_API_KEY is not set") {
			fallback, fallbackErr := e.store.FindRelevantPages(question)
			if fallbackErr == nil && len(fallback) > 0 {
				selected = limitPages(fallback, maxSelectedPages)
			}
		} else {
			return "", err
		}
	}
	if len(selected) == 0 {
		fallback, fallbackErr := e.store.FindRelevantPages(question)
		if fallbackErr == nil && len(fallback) > 0 {
			selected = limitPages(fallback, maxSelectedPages)
		}
	}
	if len(selected) == 0 {
		return "I couldn't find enough relevant wiki context to answer that question.", nil
	}

	// Try LLM synthesis; if no API key, fall back to raw content
	synthesized, err := e.synthesizeAnswer(ctx, question, selected)
	if err != nil {
		if strings.Contains(err.Error(), "ANTHROPIC_API_KEY is not set") {
			return formatRawAnswer(selected), nil
		}
		return "", err
	}

	return renderAnswer(synthesized), nil
}

func (e *Engine) selectRelevantPages(ctx context.Context, question string, allPages []wiki.Page) ([]wiki.Page, error) {
	catalogPages := limitPages(allPages, maxCatalogPages)
	indexByIdentifier := make(map[string]wiki.Page, len(catalogPages))
	var catalog strings.Builder
	for _, page := range catalogPages {
		identifier := pageIdentifier(page)
		indexByIdentifier[strings.ToLower(identifier)] = page
		indexByIdentifier[strings.ToLower(page.Name)] = page
		catalog.WriteString(fmt.Sprintf("- %s\n  snippet: %s\n", identifier, trimSnippet(page.Content)))
	}

	resp, err := e.client.Message(ctx, llm.MessageRequest{
		System:      selectionSystemPrompt,
		User:        buildSelectionPrompt(question, catalog.String()),
		MaxTokens:   700,
		Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to select relevant pages: %w", err)
	}

	var parsed pageSelectionResponse
	if err := llm.UnmarshalJSONObject(resp.Text, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse relevant page selection: %w", err)
	}

	var selected []wiki.Page
	seen := make(map[string]struct{})
	for _, candidate := range parsed.Pages {
		key := strings.ToLower(strings.TrimSpace(candidate.Identifier))
		if key == "" {
			continue
		}
		page, ok := indexByIdentifier[key]
		if !ok {
			continue
		}
		identifier := pageIdentifier(page)
		if _, ok := seen[identifier]; ok {
			continue
		}
		seen[identifier] = struct{}{}
		selected = append(selected, page)
		if len(selected) == maxSelectedPages {
			break
		}
	}

	return selected, nil
}

func (e *Engine) synthesizeAnswer(ctx context.Context, question string, pages []wiki.Page) (*synthesisResponse, error) {
	resp, err := e.client.Message(ctx, llm.MessageRequest{
		System:      synthesisSystemPrompt,
		User:        buildSynthesisPrompt(question, pages),
		MaxTokens:   1400,
		Temperature: 0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to synthesize answer: %w", err)
	}

	var parsed synthesisResponse
	if err := llm.UnmarshalJSONObject(resp.Text, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse synthesized answer: %w", err)
	}

	return &parsed, nil
}

func buildSelectionPrompt(question, catalog string) string {
	return fmt.Sprintf(`You are selecting the most relevant wiki pages for a user question.

Return JSON only:
{
  "pages": [
    {
      "identifier": "namespace/page-name",
      "reason": "short reason"
    }
  ]
}

Rules:
- Pick up to 5 pages.
- Prefer exact topical matches over broad summaries.
- Use only identifiers from the catalog.
- Do not include extra commentary.

Question:
%s

Page catalog:
%s`, question, catalog)
}

func buildSynthesisPrompt(question string, pages []wiki.Page) string {
	var context strings.Builder
	for _, page := range pages {
		context.WriteString(fmt.Sprintf("Page: %s\n%s\n\n", pageIdentifier(page), page.Content))
	}

	return fmt.Sprintf(`Answer the user question using only the provided wiki pages.

Return JSON only:
{
  "answer": "complete answer with inline citations like [namespace/page]",
  "citations": ["namespace/page"]
}

Rules:
- Ground every claim in the provided pages.
- If the pages are insufficient, say so explicitly.
- Include inline citations in the answer where claims appear.
- The citations array should list only pages actually used.
- Do not include Markdown fences or extra commentary.

Question:
%s

Wiki pages:
%s`, question, context.String())
}

func renderAnswer(resp *synthesisResponse) string {
	answer := strings.TrimSpace(resp.Answer)
	citations := dedupe(resp.Citations)
	if len(citations) == 0 {
		return answer
	}
	return fmt.Sprintf("%s\n\nSources:\n- %s", answer, strings.Join(citations, "\n- "))
}

// formatRawAnswer returns a formatted answer from raw page content (no LLM synthesis).
func formatRawAnswer(pages []wiki.Page) string {
	var b strings.Builder
	for _, page := range pages {
		b.WriteString(fmt.Sprintf("## [%s/%s]\n%s\n\n", page.Namespace, page.Name, trimSnippet(page.Content)))
	}
	return strings.TrimSpace(b.String())
}

func pageIdentifier(page wiki.Page) string {
	return strings.Trim(strings.TrimSpace(page.Namespace)+"/"+strings.TrimSpace(page.Name), "/")
}

func trimSnippet(content string) string {
	compact := strings.Join(strings.Fields(content), " ")
	if len(compact) <= maxSnippetLength {
		return compact
	}
	return compact[:maxSnippetLength] + "..."
}

func limitPages(pages []wiki.Page, limit int) []wiki.Page {
	if len(pages) <= limit {
		return pages
	}
	return pages[:limit]
}

func dedupe(values []string) []string {
	var result []string
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

const selectionSystemPrompt = `You rank wiki pages for question answering.
Return strict JSON only.`

const synthesisSystemPrompt = `You answer questions from wiki pages.
Return strict JSON only.`
