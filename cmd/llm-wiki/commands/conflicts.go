// Package commands provides CLI commands for llm-wiki.
package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"llm-wiki/internal/conflicts"
)

func NewCheckConflictsCmd() *cobra.Command {
	var outputFormat string
	var outputPath string
	var useCache bool

	cmd := &cobra.Command{
		Use:   "check-conflicts",
		Short: "Detect semantic conflicts across wiki pages",
		Long: `Analyze all wiki pages to detect semantic conflicts and inconsistencies.
Uses LLM-powered analysis to find contradictions between pages about the same entities.

Examples:
  llm-wiki check-conflicts                                    # Run full conflict detection
  llm-wiki check-conflicts --cache                           # Use cached results if available
  llm-wiki check-conflicts --output json                     # Output as JSON
  llm-wiki check-conflicts --output markdown                 # Output as Markdown table
  llm-wiki check-conflicts -o report.md                      # Save to file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			
			wikiDir := getWikiDir()
			if wikiDir == "" {
				return fmt.Errorf("wiki directory not configured")
			}

			detector := createConflictDetector(wikiDir)

			// Get or run conflict detection
			var report *conflicts.Report
			var err error

			if useCache {
				report, err = detector.GetCachedResults()
				if err != nil {
					fmt.Println("No cached results found. Running full scan...")
					report, err = detector.ScanAllPages(ctx)
					if err != nil {
						return fmt.Errorf("error detecting conflicts: %w", err)
					}
				} else {
					fmt.Println("Using cached conflict detection results")
				}
			} else {
				report, err = detector.ScanAllPages(ctx)
				if err != nil {
					return fmt.Errorf("error detecting conflicts: %w", err)
				}
			}

			// Format and display output
			switch outputFormat {
			case "json":
				data, err := conflicts.FormatJSON(report)
				if err != nil {
					return err
				}
				fmt.Println(string(data))
			case "markdown", "md":
				markdown := conflicts.FormatMarkdown(report)
				fmt.Println(markdown)
			default:
				text := conflicts.FormatText(report)
				fmt.Println(text)
			}

			// Save if requested
			if outputPath != "" {
				if err := conflicts.SaveAsFile(report, outputPath, outputFormat); err != nil {
					return fmt.Errorf("failed to save report: %w", err)
				}
				fmt.Printf("\n✓ Report saved to %s\n", outputPath)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", 
		"Output format: text, json, markdown (default: text)")
	cmd.Flags().StringVarP(&outputPath, "save", "s", "", 
		"Save report to file (requires -o format)")
	cmd.Flags().BoolVarP(&useCache, "cache", "c", false, 
		"Use cached results if available")

	return cmd
}
