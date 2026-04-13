package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"llm-wiki/internal/mcp"
)

func NewServeCmd() *cobra.Command {
	var mcpMode bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start llm-wiki as a server",
		Long:  `Start llm-wiki in server mode. Use --mcp for MCP stdio transport (Claude Desktop, Cursor, etc.).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !mcpMode {
				return fmt.Errorf("no server mode specified — use --mcp")
			}

			wikiDir := getWikiDir()
			if wikiDir == "" {
				return fmt.Errorf("wiki directory not configured")
			}

			store := createWikiStore(wikiDir)
			srv := mcp.NewServer(store)

			fmt.Fprintln(cmd.ErrOrStderr(), "llm-wiki MCP server started (stdio)")
			return srv.Serve(context.Background())
		},
	}

	cmd.Flags().BoolVar(&mcpMode, "mcp", false, "Start MCP stdio server")
	return cmd
}
