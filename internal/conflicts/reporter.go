// Package conflicts provides conflict detection for wiki pages.
package conflicts

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FormatTable formats conflicts as a table.
func FormatTable(report *Report) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Conflict Detection Report\n"))
	sb.WriteString(fmt.Sprintf("=========================\n"))
	sb.WriteString(fmt.Sprintf("\nGenerated: %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Total Pages Scanned: %d\n", report.TotalPages))
	sb.WriteString(fmt.Sprintf("Conflicts Found: %d\n\n", report.Summary.TotalConflicts))

	if len(report.Conflicts) == 0 {
		sb.WriteString("No conflicts detected!\n")
		return sb.String()
	}

	sb.WriteString("┌────────┬─────┬───────────────────────────┬────────────────────────────────────────┐\n")
	sb.WriteString("│ ID     │ Sev │ Type                      │ Title                                  │\n")
	sb.WriteString("├────────┼─────┼───────────────────────────┼────────────────────────────────────────┤\n")

	for _, c := range report.Conflicts {
		idStr := truncateString(c.ID, 8)
		titleStr := truncateString(c.Title, 36)
		sb.WriteString(fmt.Sprintf("│ %-8s │ %-3s │ %-23s │ %-36s │\n", idStr, string(c.Severity), string(c.Type), titleStr))
	}

	sb.WriteString("└────────┴─────┴───────────────────────────┴────────────────────────────────────────┘\n")
	sb.WriteString("\n\n")

	sb.WriteString("Summary:\n")
	sb.WriteString("────────\n")
	sb.WriteString(fmt.Sprintf("• Total Conflicts:      %d\n", report.Summary.TotalConflicts))
	sb.WriteString(fmt.Sprintf("• High Severity:        %d\n", report.Summary.BySeverity[SeverityHigh]))
	sb.WriteString(fmt.Sprintf("• Medium Severity:      %d\n", report.Summary.BySeverity[SeverityMedium]))
	sb.WriteString(fmt.Sprintf("• Low Severity:         %d\n", report.Summary.BySeverity[SeverityLow]))
	sb.WriteString("\nBy Type:\n")
	for t, count := range report.Summary.ByType {
		sb.WriteString(fmt.Sprintf("  • %s: %d\n", t, count))
	}
	sb.WriteString("\nRecommendations:\n")
	for _, rec := range report.Recommendations {
		sb.WriteString(fmt.Sprintf("  → %s\n", rec))
	}

	return sb.String()
}

// FormatJSON returns the report as JSON.
func FormatJSON(report *Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// Save writes the report to a file.
func Save(report *Report, path string) error {
	data, err := FormatJSON(report)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
