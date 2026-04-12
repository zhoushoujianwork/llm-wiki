package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const anthropicEndpoint = "https://api.anthropic.com/v1/messages"

// MessageRequest is a simplified Anthropic request wrapper.
type MessageRequest struct {
	System      string
	User        string
	MaxTokens   int
	Temperature float64
}

// MessageResponse captures the plain text portion of an Anthropic response.
type MessageResponse struct {
	Text string
}

type anthropicMessageRequest struct {
	Model       string                    `json:"model"`
	MaxTokens   int                       `json:"max_tokens"`
	System      string                    `json:"system,omitempty"`
	Temperature float64                   `json:"temperature,omitempty"`
	Messages    []anthropicMessageContent `json:"messages"`
}

type anthropicMessageContent struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicMessageResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *anthropicAPIError      `json:"error,omitempty"`
}

type anthropicAPIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// AnthropicClient calls the Claude Messages API.
type AnthropicClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewAnthropicClient constructs a client from environment configuration.
func NewAnthropicClient() *AnthropicClient {
	return &AnthropicClient{
		apiKey:  strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		model:   firstNonEmpty(os.Getenv("ANTHROPIC_MODEL"), "claude-3-5-sonnet-latest"),
		baseURL: firstNonEmpty(os.Getenv("ANTHROPIC_BASE_URL"), anthropicEndpoint),
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Message sends a single-turn prompt to Anthropic.
func (c *AnthropicClient) Message(ctx context.Context, req MessageRequest) (*MessageResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	payload := anthropicMessageRequest{
		Model:       c.model,
		MaxTokens:   maxTokens,
		System:      req.System,
		Temperature: req.Temperature,
		Messages: []anthropicMessageContent{
			{
				Role: "user",
				Content: []anthropicContentBlock{
					{Type: "text", Text: req.User},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read anthropic response: %w", err)
	}

	var parsed anthropicMessageResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	if httpResp.StatusCode >= 400 {
		if parsed.Error != nil {
			return nil, fmt.Errorf("anthropic API %s: %s", parsed.Error.Type, parsed.Error.Message)
		}
		return nil, fmt.Errorf("anthropic API returned status %s", httpResp.Status)
	}

	var textParts []string
	for _, block := range parsed.Content {
		if block.Type == "text" {
			textParts = append(textParts, block.Text)
		}
	}

	return &MessageResponse{Text: strings.TrimSpace(strings.Join(textParts, "\n"))}, nil
}

// Generate sends a prompt and returns the response text - implements llm.Client interface.
func (c *AnthropicClient) Generate(ctx context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	req := MessageRequest{
		User:        prompt,
		MaxTokens:   2048,
		Temperature: 0.7,
	}

	resp, err := c.Message(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Text, nil
}

// UnmarshalJSONObject extracts the first JSON object from a model response.
func UnmarshalJSONObject(raw string, target any) error {
	candidate := strings.TrimSpace(raw)
	if err := json.Unmarshal([]byte(candidate), target); err == nil {
		return nil
	}

	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start == -1 || end == -1 || end < start {
		return fmt.Errorf("response does not contain a JSON object")
	}

	return json.Unmarshal([]byte(candidate[start:end+1]), target)
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
