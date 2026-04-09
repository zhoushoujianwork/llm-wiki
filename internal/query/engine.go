package query

import (
	"context"
	"fmt"

	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

// Engine handles querying the wiki with LLM synthesis
type Engine struct {
	store *wiki.Store
	// TODO: LLM client
}

// NewEngine creates a new query engine
func NewEngine(store *wiki.Store) *Engine {
	return &Engine{store: store}
}

// Ask processes a question and returns an answer
func (e *Engine) Ask(ctx context.Context, question string) (string, error) {
	// Find relevant pages
	pages, err := e.store.FindRelevantPages(question)
	if err != nil {
		return "", fmt.Errorf("failed to find relevant pages: %w", err)
	}

	if len(pages) == 0 {
		return "I don't have any relevant information in the wiki yet. Try adding sources and running compile first.", nil
	}

	// Build context from top pages
	context := buildContext(pages)

	// TODO: Send to LLM for synthesis
	// For now, return a simple response
	answer := fmt.Sprintf("Based on the wiki (%d pages relevant to your query):\n\n%s\n\n[LLM synthesis not yet connected — showing top match]", len(pages), context)
	return answer, nil
}

func buildContext(pages []wiki.Page) string {
	if len(pages) == 0 {
		return ""
	}
	// Limit to top 3 pages
	if len(pages) > 3 {
		pages = pages[:3]
	}

	var result string
	for _, p := range pages {
		result += fmt.Sprintf("--- [%s/%s] ---\n%s\n\n", p.Namespace, p.Name, p.Content)
	}
	return result
}
