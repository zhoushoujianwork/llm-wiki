// Package conflicts provides conflict detection services for LLM Wiki.
// This implements maintenance capabilities to ensure knowledge base consistency.
package conflicts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-wiki/internal/llm"
	"llm-wiki/internal/wiki"
)

const defaultCacheDir = ".cache"

// Conflict represents a detected inconsistency between wiki pages.
type Conflict struct {
	EntityName     string    `json:"entity_name"`
	Confidence     float64   `json:"confidence"` // 0.0-1.0
	PageA          string    `json:"page_a"`
	StatementA     string    `json:"statement_a"`
	PageB          string    `json:"page_b"`
	StatementB     string    `json:"statement_b"`
	Recommendation string    `json:"recommendation"` // "merge", "manual_review", "ignore"
	CreatedAt      time.Time `json:"created_at"`
}

// Report aggregates conflict detection results.
type Report struct {
	TotalPages    int        `json:"total_pages"`
	TotalEntities int        `json:"total_entities"`
	Conflicts     []Conflict `json:"conflicts"`
	Summary       Summary    `json:"summary"`
	Timestamp     time.Time  `json:"timestamp"`
}

// Summary provides high-level statistics about the report.
type Summary struct {
	HighConfidence   int `json:"high_confidence"`   // >= 0.8
	MediumConfidence int `json:"medium_confidence"` // 0.5-0.8
	LowConfidence    int `json:"low_confidence"`    // < 0.5
	SamplesChecked   int `json:"samples_checked"`
	Duration         string `json:"duration"`
}

// Detector interface for conflict detection.
type Detector interface {
	// ScanAllPages performs a full scan of all wiki pages for conflicts.
	ScanAllPages(ctx context.Context) (*Report, error)

	// CheckEntity validates consistency for a specific entity across all pages.
	CheckEntity(ctx context.Context, entityName string) ([]Conflict, error)

	// ValidateConsistency checks logical consistency among specified pages.
	ValidateConsistency(ctx context.Context, pagePaths []string) ([]Conflict, error)

	// GetCachedResults returns cached conflict detection results if available.
	GetCachedResults() (*Report, error)

	// CacheResults stores conflict detection results for later retrieval.
	CacheResults(report *Report) error
}

// ConflictDetector implements Detector using LLM-based semantic analysis.
type ConflictDetector struct {
	LLMClient llm.Client
	WikiStore *wiki.Store
	CacheDir  string
}

// NewConflictDetector creates a new ConflictDetector instance.
func NewConflictDetector(client llm.Client, store *wiki.Store, cacheDir string) *ConflictDetector {
	if cacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, ".llm-wiki", defaultCacheDir)
	}
	return &ConflictDetector{
		LLMClient: client,
		WikiStore: store,
		CacheDir:  cacheDir,
	}
}

// ScanAllPages scans all wiki pages for semantic conflicts.
func (d *ConflictDetector) ScanAllPages(ctx context.Context) (*Report, error) {
	start := time.Now()
	report := &Report{
		Timestamp: time.Now(),
	}

	// Get all pages
	pages, err := d.WikiStore.ListPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}
	report.TotalPages = len(pages)

	// Get all entities and their pages
	entityToPages := d.WikiStore.GetEntities()

	samplesChecked := 0

	// For each entity, check for conflicts
	for entityName, pagePaths := range entityToPages {
		conflicts, err := d.CheckEntity(ctx, entityName)
		if err != nil {
			continue // Skip this entity on error
		}
		report.Conflicts = append(report.Conflicts, conflicts...)
		samplesChecked += len(pagePaths)
	}

	// Update summary
	for _, c := range report.Conflicts {
		switch {
		case c.Confidence >= 0.8:
			report.Summary.HighConfidence++
		case c.Confidence >= 0.5:
			report.Summary.MediumConfidence++
		default:
			report.Summary.LowConfidence++
		}
	}
	report.Summary.SamplesChecked = samplesChecked
	report.Summary.Duration = time.Since(start).String()

	return report, nil
}

// CheckEntity validates consistency for a specific entity across all pages.
func (d *ConflictDetector) CheckEntity(ctx context.Context, entityName string) ([]Conflict, error) {
	// Get all pages mentioning this entity
	entityToPages := d.WikiStore.GetEntities()

	pagePaths, exists := entityToPages[entityName]
	if !exists || len(pagePaths) < 2 {
		return nil, nil // No conflicts possible with fewer than 2 pages
	}

	var conflicts []Conflict

	// Compare pairs of pages
	for i := 0; i < len(pagePaths); i++ {
		for j := i + 1; j < len(pagePaths); j++ {
			conflict, err := d.comparePages(ctx, pagePaths[i], pagePaths[j], entityName)
			if err != nil {
				continue
			}
			if conflict != nil {
				conflicts = append(conflicts, *conflict)
			}
		}
	}

	return conflicts, nil
}

// comparePages compares two pages for conflicting statements about an entity.
func (d *ConflictDetector) comparePages(ctx context.Context, pageA, pageB, entity string) (*Conflict, error) {
	// Read page contents
	contentA, err := d.WikiStore.ReadPage(pageA)
	if err != nil {
		return nil, err
	}
	contentB, err := d.WikiStore.ReadPage(pageB)
	if err != nil {
		return nil, err
	}

	// Extract relevant statements about the entity from each page
	statementsA := d.extractRelevantStatements(contentA, entity)
	statementsB := d.extractRelevantStatements(contentB, entity)

	if len(statementsA) == 0 || len(statementsB) == 0 {
		return nil, nil // No relevant statements found
	}

	// Use LLM to detect conflicts
	prompt := d.buildConflictPrompt(entity, statementsA, statementsB)

	response, err := d.LLMClient.Generate(context.Background(), prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Parse response to extract conflict info
	conflict := d.parseConflictResponse(response, entity, pageA, pageB)

	if conflict != nil && conflict.Recommendation != "ignore" {
		return conflict, nil
	}

	return nil, nil
}

// extractRelevantStatements extracts sentences mentioning the entity.
func (d *ConflictDetector) extractRelevantStatements(content, entity string) []string {
	var statements []string

	// Simple heuristic: split by sentence boundaries
	sentences := strings.Split(content, ". ")

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// Case-insensitive search
		if strings.Contains(strings.ToLower(sentence), strings.ToLower(entity)) {
			statements = append(statements, sentence)
		}
	}

	return statements
}

// buildConflictPrompt constructs the LLM prompt for conflict detection.
func (d *ConflictDetector) buildConflictPrompt(entity string, statementsA, statementsB []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Analyze these statements about '%s' from different wiki pages:\n\n", entity))

	sb.WriteString("## Page A Statements:\n")
	for i, stmt := range statementsA {
		stmtShort := truncateString(stmt, 200)
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, stmtShort))
	}

	sb.WriteString("\n## Page B Statements:\n")
	for i, stmt := range statementsB {
		stmtShort := truncateString(stmt, 200)
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, stmtShort))
	}

	sb.WriteString("\n### Task:\n")
	sb.WriteString("Check if there are any contradictions or inconsistencies between these statements.\n")
	sb.WriteString("Return a JSON object with this exact structure (no Markdown formatting):\n")
	sb.WriteString(`{"hasConflict": boolean, "confidence": number (0.0-1.0), "reason": string, "recommendation": "merge"|"manual_review"|"ignore", "resolution": "string or empty"}`)

	return sb.String()
}

// parseConflictResponse parses LLM response into Conflict struct.
func (d *ConflictDetector) parseConflictResponse(response, entity, pageA, pageB string) *Conflict {
	// Try to parse as JSON first
	var parsed struct {
		HasConflict  bool    `json:"hasConflict"`
		Confidence   float64 `json:"confidence"`
		Reason       string  `json:"reason"`
		Recommendation string `json:"recommendation"`
		Resolution   string  `json:"resolution"`
	}

	if err := llm.UnmarshalJSONObject(response, &parsed); err == nil && parsed.HasConflict {
		return &Conflict{
			EntityName:     entity,
			Confidence:     parsed.Confidence,
			PageA:          pageA,
			StatementA:     "[see source]",
			PageB:          pageB,
			StatementB:     "[see source]",
			Recommendation: firstNonEmpty(parsed.Recommendation, "manual_review"),
			CreatedAt:      time.Now(),
		}
	}

	// Fallback: check for conflict keywords in text
	if strings.Contains(strings.ToLower(response), "conflict") || 
	   strings.Contains(strings.ToLower(response), "contradiction") {
		confidence := 0.7
		recommendation := "manual_review"

		if strings.Contains(strings.ToLower(response), "slight") || 
		   strings.Contains(strings.ToLower(response), "minor") {
			confidence = 0.5
			recommendation = "merge"
		}

		return &Conflict{
			EntityName:     entity,
			Confidence:     confidence,
			PageA:          pageA,
			StatementA:     "[see source]",
			PageB:          pageB,
			StatementB:     "[see source]",
			Recommendation: recommendation,
			CreatedAt:      time.Now(),
		}
	}

	return nil
}

// ValidateConsistency checks logical consistency among specified pages.
func (d *ConflictDetector) ValidateConsistency(ctx context.Context, pagePaths []string) ([]Conflict, error) {
	var allConflicts []Conflict

	for i := 0; i < len(pagePaths); i++ {
		for j := i + 1; j < len(pagePaths); j++ {
			conflicts, err := d.validatePair(ctx, pagePaths[i], pagePaths[j])
			if err != nil {
				continue
			}
			allConflicts = append(allConflicts, conflicts...)
		}
	}

	return allConflicts, nil
}

// validatePair validates consistency between two pages.
func (d *ConflictDetector) validatePair(ctx context.Context, pageA, pageB string) ([]Conflict, error) {
	contentA, err := d.WikiStore.ReadPage(pageA)
	if err != nil {
		return nil, err
	}
	contentB, err := d.WikiStore.ReadPage(pageB)
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(
		"Compare these two wiki pages for logical consistency. "+
			"Page A (%d chars): %s\n\nPage B (%d chars): %s\n\n"+
			"Identify any logical contradictions, unsupported claims, or factual errors. "+
			"Return JSON: {\"hasIssues\": boolean, \"issues\": [{\"type\": string, \"description\": string}]}",
		min(len(contentA), 3000), truncateAndEllipsis(contentA, 3000),
		min(len(contentB), 3000), truncateAndEllipsis(contentB, 3000),
	)

	response, err := d.LLMClient.Generate(context.Background(), prompt)
	if err != nil {
		return nil, err
	}

	return d.parseGeneralValidation(response), nil
}

// parseGeneralValidation parses general validation responses.
func (d *ConflictDetector) parseGeneralValidation(response string) []Conflict {
	// Placeholder implementation - could be extended for detailed validation
	var parsed struct {
		HasIssues bool     `json:"hasIssues"`
		Issues    []Issue  `json:"issues"`
	}

	if err := llm.UnmarshalJSONObject(response, &parsed); err == nil && len(parsed.Issues) > 0 {
		// Return generic conflict for now
		return nil
	}

	return nil
}

// Issue represents a specific issue found during validation.
type Issue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// GetCachedResults retrieves cached conflict detection results.
func (d *ConflictDetector) GetCachedResults() (*Report, error) {
	cacheFile := filepath.Join(d.CacheDir, "conflicts.json")
	
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no cached results found")
		}
		return nil, fmt.Errorf("failed to read cache: %w", err)
	}

	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}

	// Check if cache is stale (older than 24 hours)
	if time.Since(report.Timestamp) > 24*time.Hour {
		return nil, fmt.Errorf("cached results are stale (>24h)")
	}

	return &report, nil
}

// CacheResults stores conflict detection results for later retrieval.
func (d *ConflictDetector) CacheResults(report *Report) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(d.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	cacheFile := filepath.Join(d.CacheDir, "conflicts.json")
	
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize report: %w", err)
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// truncateString truncates a string to max length.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// truncateAndEllipsis truncates with ellipsis at the end.
func truncateAndEllipsis(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// firstNonEmpty returns the first non-empty string or a default.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "manual_review"
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
