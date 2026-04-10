// Package commands provides CLI commands for llm-wiki.
package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zhoushoujianwork/llm-wiki/internal/conflicts"
	"github.com/zhoushoujianwork/llm-wiki/internal/index"
	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

func NewConflictsCmd() *cobra.Command {
	var outputFormat string
	var outputPath string

	cmd := &cobra.Command{
		Use:   "conflicts",
		Short: "Detect conflicts between wiki pages",
		Long: `Detect and report conflicts, inconsistencies, and other quality issues 
in the wiki documentation. This includes:

- Contradictory content between pages
- Duplicate or overlapping information  
- Broken or missing references
- Circular dependencies

The tool scans all wiki pages and produces a detailed report with recommendations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Use getWikiDir() like other commands
			wikiDir := getWikiDir()
			if wikiDir == "" {
				return fmt.Errorf("wiki directory not configured")
			}

			store := wiki.NewStore(wikiDir)
			
			// Build knowledge graph for semantic analysis
			ctx := context.Background()
			kg, err := index.LoadOrCreateKnowledgeGraph(ctx, store, "")
			if err != nil {
				return fmt.Errorf("failed to build knowledge graph: %w", err)
			}
			
			// Create detector with knowledge graph support
			detector := conflicts.NewDetectorWithKG(store, kg)

			report, err := detector.DetectAll(ctx)
			if err != nil {
				return fmt.Errorf("error detecting conflicts: %w", err)
			}

			switch outputFormat {
			case "table":
				fmt.Print(conflicts.FormatTable(report))
			case "json":
				data, err := conflicts.FormatJSON(report)
				if err != nil {
					return err
				}
				fmt.Println(string(data))
			default:
				return fmt.Errorf("unsupported output format: %s (use table or json)", outputFormat)
			}

			if outputPath != "" {
				if err := conflicts.Save(report, outputPath); err != nil {
					return fmt.Errorf("failed to save report: %w", err)
				}
				fmt.Printf("\nReport saved to %s\n", outputPath)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "format", "f", "table", "Output format (table, json)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Save report to file (JSON format)")

	return cmd
}
