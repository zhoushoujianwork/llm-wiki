// Package mcp provides a minimal MCP (Model Context Protocol) stdio server
// for llm-wiki. It speaks JSON-RPC 2.0 over stdin/stdout.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"llm-wiki/internal/wiki"
)

// Server is the MCP stdio server.
type Server struct {
	store *wiki.Store
}

// NewServer creates a new MCP server backed by the given wiki store.
func NewServer(store *wiki.Store) *Server {
	return &Server{store: store}
}

// jsonrpcRequest is a generic JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is a generic JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func errResp(id any, code int, msg string) jsonrpcResponse {
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func okResp(id any, result any) jsonrpcResponse {
	return jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

// Serve reads JSON-RPC requests from stdin and writes responses to stdout.
func (s *Server) Serve(_ context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(errResp(nil, -32700, "parse error"))
			continue
		}

		resp := s.dispatch(req)
		_ = enc.Encode(resp)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("stdin read error: %w", err)
	}
	return nil
}

func (s *Server) dispatch(req jsonrpcRequest) jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return okResp(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "llm-wiki",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		})

	case "tools/list":
		return okResp(req.ID, map[string]any{
			"tools": []map[string]any{
				{
					"name":        "wiki_query",
					"description": "Search the wiki for pages relevant to a question or keyword",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"question": map[string]string{"type": "string", "description": "Search query or question"},
						},
						"required": []string{"question"},
					},
				},
				{
					"name":        "wiki_list_pages",
					"description": "List all wiki pages, optionally filtered by namespace",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"namespace": map[string]string{"type": "string", "description": "Optional namespace filter"},
						},
					},
				},
				{
					"name":        "wiki_read_page",
					"description": "Read the full content of a wiki page by its file path",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]string{"type": "string", "description": "Absolute path to the wiki page"},
						},
						"required": []string{"path"},
					},
				},
			},
		})

	case "tools/call":
		return s.handleToolCall(req)

	case "notifications/initialized":
		// No response needed for notifications.
		return jsonrpcResponse{}

	default:
		return errResp(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleToolCall(req jsonrpcRequest) jsonrpcResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errResp(req.ID, -32602, "invalid params")
	}

	switch params.Name {
	case "wiki_query":
		var args struct {
			Question string `json:"question"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil || args.Question == "" {
			return errResp(req.ID, -32602, "missing question")
		}
		pages, err := s.store.FindRelevantPages(args.Question)
		if err != nil {
			return errResp(req.ID, -32603, err.Error())
		}
		var parts []string
		for _, p := range pages {
			parts = append(parts, fmt.Sprintf("## %s/%s\n\n%s", p.Namespace, p.Name, p.Content))
		}
		result := strings.Join(parts, "\n\n---\n\n")
		if result == "" {
			result = "No relevant pages found."
		}
		return okResp(req.ID, toolResult(result))

	case "wiki_list_pages":
		var args struct {
			Namespace string `json:"namespace"`
		}
		_ = json.Unmarshal(params.Arguments, &args)

		paths, err := s.store.ListPages()
		if err != nil {
			return errResp(req.ID, -32603, err.Error())
		}
		var filtered []string
		for _, p := range paths {
			if args.Namespace == "" || strings.Contains(p, "/"+args.Namespace+"/") {
				filtered = append(filtered, p)
			}
		}
		return okResp(req.ID, toolResult(strings.Join(filtered, "\n")))

	case "wiki_read_page":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil || args.Path == "" {
			return errResp(req.ID, -32602, "missing path")
		}
		content, err := s.store.ReadPage(args.Path)
		if err != nil {
			return errResp(req.ID, -32603, err.Error())
		}
		return okResp(req.ID, toolResult(content))

	default:
		return errResp(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}
}

// toolResult wraps plain text as an MCP tool result.
func toolResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
}
