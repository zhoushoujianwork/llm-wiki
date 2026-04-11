// Package conflicts provides formatting utilities for conflict reports.
package conflicts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FormatText formats conflicts as a text table.
func FormatText(report *Report) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Conflict Detection Report\n"))
	sb.WriteString(fmt.Sprintf("=========================\n"))
	sb.WriteString(fmt.Sprintf("\nGenerated: %s\n", report.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Total Pages Scanned: %d\n", report.TotalPages))
	sb.WriteString(fmt.Sprintf("Total Entities Checked: %d\n", report.TotalEntities))
	sb.WriteString(fmt.Sprintf("Conflicts Found: %d\n\n", len(report.Conflicts)))

	if len(report.Conflicts) == 0 {
		sb.WriteString("No conflicts detected!\n")
		return sb.String()
	}

	sb.WriteString("Conflicts:\n")
	sb.WriteString("──────────\n")
	for i, c := range report.Conflicts {
		sb.WriteString(fmt.Sprintf("\n%d. Entity: %s\n", i+1, c.EntityName))
		sb.WriteString(fmt.Sprintf("   Confidence: %.1f%% (%s)\n", c.Confidence*100, formatConfidenceLevel(c.Confidence)))
		sb.WriteString(fmt.Sprintf("   Recommendation: %s\n", c.Recommendation))
		sb.WriteString(fmt.Sprintf("   Page A: %s\n", c.PageA))
		sb.WriteString(fmt.Sprintf("     Statement: %s\n", truncateAndEllipsis(c.StatementA, 100)))
		sb.WriteString(fmt.Sprintf("   Page B: %s\n", c.PageB))
		sb.WriteString(fmt.Sprintf("     Statement: %s\n", truncateAndEllipsis(c.StatementB, 100)))
	}

	sb.WriteString("\n\nSummary:\n")
	sb.WriteString("────────\n")
	sb.WriteString(fmt.Sprintf("• High confidence (≥80%%): %d\n", report.Summary.HighConfidence))
	sb.WriteString(fmt.Sprintf("• Medium confidence (50-80%%): %d\n", report.Summary.MediumConfidence))
	sb.WriteString(fmt.Sprintf("• Low confidence (<50%%): %d\n", report.Summary.LowConfidence))
	sb.WriteString(fmt.Sprintf("• Total samples checked: %d\n", report.Summary.SamplesChecked))
	sb.WriteString(fmt.Sprintf("• Analysis duration: %s\n", report.Summary.Duration))

	return sb.String()
}

// FormatMarkdown formats conflicts as markdown table.
func FormatMarkdown(report *Report) string {
	var sb strings.Builder

	sb.WriteString("# Conflict Detection Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n\n", report.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Total Pages**: %d | **Total Entities**: %d | **Conflicts Found**: %d\n\n",
		report.TotalPages, report.TotalEntities, len(report.Conflicts)))

	if len(report.Conflicts) == 0 {
		sb.WriteString("✅ No conflicts detected! All wiki pages are consistent.\n\n")
		return sb.String()
	}

	sb.WriteString("## Conflicts Detected\n\n")
	sb.WriteString("| # | Entity | Confidence | Page A | Page B |\n")
	sb.WriteString("|---|--------|------------|--------|--------|\n")

	for i, c := range report.Conflicts {
		sb.WriteString(fmt.Sprintf("%d | **%s** | %.1f%% | `%s` | `%s` |\n",
			i+1, c.EntityName, c.Confidence*100, truncateAndEllipsis(filepath.Base(c.PageA), 30),
			truncateAndEllipsis(filepath.Base(c.PageB), 30)))
	}

	sb.WriteString("\n## Summary\n\n")
	sb.WriteString("- **High Confidence (≥80%)**: " + fmt.Sprintf("%d", report.Summary.HighConfidence) + "\n")
	sb.WriteString("- **Medium Confidence (50-80%)**: " + fmt.Sprintf("%d", report.Summary.MediumConfidence) + "\n")
	sb.WriteString("- **Low Confidence (<50%)**: " + fmt.Sprintf("%d", report.Summary.LowConfidence) + "\n")
	sb.WriteString("- **Analysis Duration**: " + report.Summary.Duration + "\n")

	return sb.String()
}

// FormatJSON returns the report as JSON.
func FormatJSON(report *Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// SaveAsFile writes the report to a file in the specified format.
func SaveAsFile(report *Report, path string, format string) error {
	var data []byte
	var err error

	switch strings.ToLower(format) {
	case "json":
		data, err = FormatJSON(report)
	case "markdown", "md":
		markdown := FormatMarkdown(report)
		data = []byte(markdown)
	default:
		text := FormatText(report)
		data = []byte(text)
	}

	if err != nil {
		return fmt.Errorf("failed to format report: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Helper functions

func formatConfidenceLevel(confidence float64) string {
	if confidence >= 0.8 {
		return "high"
	} else if confidence >= 0.5 {
		return "medium"
	}
	return "low"
}

// truncateAndEllipsis is exported from conflicts.go for use here
