package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"llm-wiki/internal/mcp"
	"llm-wiki/internal/web"
)

func NewServeCmd() *cobra.Command {
	var mcpMode bool
	var webMode bool
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start llm-wiki as a server",
		Long:  `Start llm-wiki in server mode. Use --mcp for MCP stdio transport or --web for a local web UI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			wikiDir := getWikiDir()
			if wikiDir == "" {
				return fmt.Errorf("wiki directory not configured")
			}

			store := createWikiStore(wikiDir)

			if webMode {
				addr := fmt.Sprintf(":%d", port)
				srv, err := web.NewServer(store)
				if err != nil {
					return fmt.Errorf("failed to create web server: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "llm-wiki web UI started at http://localhost%s\n", addr)
				return srv.Serve(context.Background(), addr)
			}

			if mcpMode {
				srv := mcp.NewServer(store)
				fmt.Fprintln(cmd.ErrOrStderr(), "llm-wiki MCP server started (stdio)")
				return srv.Serve(context.Background())
			}

			return fmt.Errorf("no server mode specified — use --mcp or --web")
		},
	}

	cmd.Flags().BoolVar(&mcpMode, "mcp", false, "Start MCP stdio server")
	cmd.Flags().BoolVar(&webMode, "web", false, "Start local web UI server")
	cmd.Flags().IntVar(&port, "port", 19876, "Port for the web UI server (used with --web)")
	return cmd
}
