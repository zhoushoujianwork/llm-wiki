// Package llm provides an abstract interface for LLM clients.
package llm

import "context"

// Client is the interface that all LLM clients must implement.
type Client interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
