package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"llm-wiki/internal/quality"
)

// QualityCommand implements the `llm-wiki quality` subcommand family.

// NewQualityCmd creates the quality command.
func NewQualityCmd() *cobra.Command {
	
	qualityCmd := &cobra.Command{
		Use:   "quality",
		Short: "Evaluate wiki page quality",
		Long:  `Evaluate the quality of wiki pages based on completeness, accuracy, readability, and coherence.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	qualityCmd.AddCommand(NewQualityCheckCmd())
	qualityCmd.AddCommand(NewQualityDetailsCmd())
	qualityCmd.AddCommand(NewQualityReportCmd())

	qualityCmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "text",
		"Output format: text, json, markdown")
	qualityCmd.Flags().BoolVarP(&cmdCache, "cache", "c", false,
		"Use cached results if available")

	return qualityCmd
}

// NewQualityCheckCmd creates the quality check subcommand.
func NewQualityCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run a full quality check",
		Long:  `Evaluate all wiki pages and generate a comprehensive quality report.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, _ := cmd.Flags().GetString("output")
			evaluator := createQualityEvaluator("")
			return runQualityCheck(evaluator, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "text",
		"Output format: text, json, markdown")
	cmd.Flags().BoolVarP(&cmdCache, "cache", "c", false,
		"Use cached results if available")

	return cmd
}

// NewQualityDetailsCmd creates the quality details subcommand.
func NewQualityDetailsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "details <page-path>",
		Short: "Get detailed quality analysis for a specific page",
		Long:  `Show detailed quality metrics and issues for a specific wiki page.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, _ := cmd.Flags().GetString("output")
			evaluator := createQualityEvaluator("")
			return runQualityDetails(evaluator, args[0], outputFormat)
		},
	}

	cmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "text",
		"Output format: text, json, markdown")

	return cmd
}

// NewQualityReportCmd creates the quality report subcommand.
func NewQualityReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a quality report",
		Long:  `Generate a formatted quality report showing all pages and their scores.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat, _ := cmd.Flags().GetString("output")
			evaluator := createQualityEvaluator("")
			return runQualityReport(evaluator, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&cmdOutputFormat, "output", "o", "markdown",
		"Output format: text, json, markdown")
	cmd.Flags().BoolVarP(&cmdCache, "cache", "c", false,
		"Use cached results if available")

	return cmd
}

// runQualityCheck executes a full quality check.
func runQualityCheck(evaluator *quality.QualityEvaluator, format string) error {
	ctx := context.Background()
	
	fmt.Println("📊 Running Quality Evaluation...")
	start := time.Now()

	var report *quality.Report
	var err error
	
	// Try to use cache first
	report, err = evaluator.GetCachedResults()
	if err == nil {
		fmt.Printf("✅ Using cached results (last checked %s)\n", 
			report.Timestamp.Format(time.DateTime))
		printQualityReport(report, format)
		return nil
	}
	fmt.Println("⚠️ Cache not available, running fresh evaluation...")

	report, err = evaluator.EvaluateAllPages(ctx)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	duration := time.Since(start)
	fmt.Printf("✅ Completed in %v\n\n", duration)
	printQualityReport(report, format)

	// Cache results automatically
	if err := evaluator.CacheResults(report); err != nil {
		fmt.Printf("⚠️ Could not cache results: %v\n", err)
	}

	return nil
}

// runQualityDetails shows detailed analysis for a specific page.
func runQualityDetails(evaluator *quality.QualityEvaluator, pagePath string, format string) error {
	ctx := context.Background()

	fmt.Printf("🔍 Analyzing page: %s\n", pagePath)
	
	score, err := evaluator.EvaluatePage(ctx, pagePath)
	if err != nil {
		return fmt.Errorf("failed to analyze page: %w", err)
	}

	printQualityScore(score, format)
	return nil
}

// runQualityReport generates a full report.
func runQualityReport(evaluator *quality.QualityEvaluator, format string) error {
	ctx := context.Background()

	fmt.Println("📋 Generating Quality Report...")
	start := time.Now()

	report, err := evaluator.EvaluateAllPages(ctx)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	fmt.Printf("Report generated in %v\n\n", time.Since(start))
	printQualityReport(report, format)
	return nil
}

// printQualityScore prints a single page's quality score.
func printQualityScore(score *quality.QualityScore, format string) {
	var output string
	
	switch format {
	case "json":
		data, _ := json.MarshalIndent(score, "", "  ")
		output = string(data)
	default:
		output = formatQualityScoreText(score)
	}

	fmt.Println(output)
}

// formatQualityScoreText formats score as plain text.
func formatQualityScoreText(s *quality.QualityScore) string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("\nQuality Score for: %s\n", s.Title))
	sb.WriteString(fmt.Sprintf("Path: %s\n\n", s.Path))
	
	sb.WriteString(fmt.Sprintf("Overall Score: %.1f / 100\n", s.Overall))
	
	tier := "Fair"
	if s.Overall >= 90 {
		tier = "Excellent ⭐"
	} else if s.Overall >= 70 {
		tier = "Good 👍"
	} else if s.Overall >= 50 {
		tier = "Fair 👌"
	} else {
		tier = "Needs Work ⚠️"
	}
	sb.WriteString(fmt.Sprintf("Tier: %s\n\n", tier))
	
	sb.WriteString("Detailed Scores:\n")
	sb.WriteString(fmt.Sprintf("  Completeness:  %.0f%%\n", s.Detailed["completeness"]))
	sb.WriteString(fmt.Sprintf("  Accuracy:      %.0f%%\n", s.Detailed["accuracy"]))
	sb.WriteString(fmt.Sprintf("  Readability:   %.0f%%\n", s.Detailed["readability"]))
	sb.WriteString(fmt.Sprintf("  Coherence:     %.0f%%\n", s.Detailed["coherence"]))
	
	if len(s.Issues) > 0 {
		sb.WriteString("\nIssues Found:\n")
		for _, issue := range s.Issues {
			sb.WriteString(fmt.Sprintf("  ⚠️ %s\n", issue))
		}
	}
	
	if len(s.Suggestions) > 0 {
		sb.WriteString("\nSuggestions:\n")
		for _, suggestion := range s.Suggestions {
			sb.WriteString(fmt.Sprintf("  💡 %s\n", suggestion))
		}
	}
	
	sb.WriteString(fmt.Sprintf("\nEvaluated at: %s\n", s.EvaluatedAt.Format(time.DateTime)))
	
	return sb.String()
}

// printQualityReport prints a full quality report.
func printQualityReport(report *quality.Report, format string) {
	switch format {
	case "json":
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
	default:
		fmtQualityReportText(report)
	}
}

// fmtQualityReportText formats report as text.
func fmtQualityReportText(r *quality.Report) {
	var sb strings.Builder
	
	sb.WriteString("\n=== Wiki Quality Report ===\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", r.Timestamp.Format(time.DateTime)))
	sb.WriteString(fmt.Sprintf("Duration: %s\n\n", r.Summary.Duration))
	
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("Total Pages: %d\n", r.TotalPages))
	sb.WriteString(fmt.Sprintf("Pages Evaluated: %d\n", r.PagesEvaluated))
	sb.WriteString(fmt.Sprintf("Average Score: %.1f/100\n\n", r.AverageScore))
	
	sb.WriteString("## Quality Distribution\n\n")
	sb.WriteString(fmt.Sprintf("⭐ Excellent (90+):    %d\n", r.QualityDist.Excellent))
	sb.WriteString(fmt.Sprintf("👍 Good (70-89):      %d\n", r.QualityDist.Good))
	sb.WriteString(fmt.Sprintf("👌 Fair (50-69):      %d\n", r.QualityDist.Fair))
	sb.WriteString(fmt.Sprintf("⚠️ Poor (<50):         %d\n\n", r.QualityDist.Poor))
	
	if r.Summary.NeedsReviewCount > 0 {
		sb.WriteString(fmt.Sprintf("**%d pages need attention** (score < 70)\n\n", r.Summary.NeedsReviewCount))
	}
	
	if r.Summary.HighestRatedPage != "" {
		sb.WriteString(fmt.Sprintf("Best rated: %s\n", r.Summary.HighestRatedPage))
	}
	if r.Summary.LowestRatedPage != "" {
		sb.WriteString(fmt.Sprintf("Lowest rated: %s\n", r.Summary.LowestRatedPage))
	}
	
	sb.WriteString("\n")
	fmt.Print(sb.String())
}
