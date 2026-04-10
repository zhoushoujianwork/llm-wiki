// Package conflicts provides conflict detection services for LLM Wiki.
// This implements Phase 1 of issue #2: adding maintenance capabilities to ensure
// knowledge base consistency and accuracy.
package conflicts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm-wiki/internal/llm"
)

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
	TotalPages  int        `json:"total_pages"`
	TotalEntities int      `json:"total_entities"`
	Conflicts   []Conflict `json:"conflicts"`
	Summary     Summary    `json:"summary"`
	Timestamp   time.Time  `json:"timestamp"`
}

// Summary provides high-level statistics about the report.
type Summary struct {
	HighConfidence  int     `json:"high_confidence"` // >= 0.8
	MediumConfidence int    `json:"medium_confidence"` // 0.5-0.8
	LowConfidence   int     `json:"low_confidence"` // < 0.5
	Samples         int     `json:"samples_checked"`
	Duration        string  `json:"duration"`
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
}

// ConflictDetector implements Detector using LLM-based semantic analysis.
type ConflictDetector struct {
	LLMClient llm.Client
	WikiStore WikiStoreProvider
	CacheDir  string
}

// WikiStoreProvider provides access to wiki pages.
type WikiStoreProvider interface {
	ListPages(ctx context.Context) ([]PageInfo, error)
	ReadPage(ctx context.Context, path string) (string, error)
	GetEntities(ctx context.Context) (map[string][]string, error)
}

// PageInfo contains metadata about a wiki page.
type PageInfo struct {
	Path     string
	Title    string
	Entities []string
}

// NewConflictDetector creates a new ConflictDetector instance.
func NewConflictDetector(client llm.Client, store WikiStoreProvider, cacheDir string) *ConflictDetector {
	return &ConflictDetector{
		LLMClient: client,
		WikiStore: store,
		CacheDir:  cacheDir,
	}
}

// ScanAllPages scans all wiki pages for semantic conflicts.
func (d *ConflictDetector) ScanAllPages(ctx context.Context) (*Report, start := time.Now(), err error) {
	report := &Report{
		Timestamp: time.Now(),
	}

	// Get all pages
	pages, err := d.WikiStore.ListPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}
	report.TotalPages = len(pages)

	// Get all entities and their pages
	entityToPages, err := d.WikiStore.GetEntities(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}
	report.TotalEntities = len(entityToPages)

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
	report.Summary.Samples = samplesChecked
	report.Summary.Duration = time.Since(start).String()

	return report, nil
}

// CheckEntity validates consistency for a specific entity across all pages.
func (d *ConflictDetector) CheckEntity(ctx context.Context, entityName string) ([]Conflict, error) {
	// Get all pages mentioning this entity
	entityToPages, err := d.WikiStore.GetEntities(ctx)
	if err != nil {
		return nil, err
	}

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
	contentA, err := d.WikiStore.ReadPage(ctx, pageA)
	if err != nil {
		return nil, err
	}
	contentB, err := d.WikiStore.ReadPage(ctx, pageB)
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
	
	response, err := d.LLMClient.Generate(ctx, prompt)
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
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, stmt))
	}
	
	sb.WriteString("\n## Page B Statements:\n")
	for i, stmt := range statementsB {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, stmt))
	}
	
	sb.WriteString("\n### Task:\n")
	sb.WriteString("Check if there are any contradictions or inconsistencies between these statements.\n")
	sb.WriteString("Return a JSON object with the following structure:\n")
	sb.WriteString(`{"hasConflict": true/false, "confidence": 0.0-1.0, "reason": "explanation", "recommendation": "merge|manual_review|ignore", "resolution": "suggested text if applicable"}`)
	
	return sb.String()
}

// parseConflictResponse parses LLM response into Conflict struct.
func (d *ConflictDetector) parseConflictResponse(response, entity, pageA, pageB string) *Conflict {
	// Simplified parsing - in production, use proper JSON parsing
	// This is a placeholder implementation
	
	confidence := 0.0
	recommendation := "ignore"
	
	// Check response for indicators
	if strings.Contains(strings.ToLower(response), "\"hasConflict\": true") {
		confidence = 0.8
		recommendation = "manual_review"
	} else if strings.Contains(strings.ToLower(response), "conflict") {
		confidence = 0.5
		recommendation = "merge"
	}
	
	if confidence == 0 {
		return nil
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
	contentA, err := d.WikiStore.ReadPage(ctx, pageA)
	if err != nil {
		return nil, err
	}
	contentB, err := d.WikiStore.ReadPage(ctx, pageB)
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(
		"Compare these two wiki pages for logical consistency. "+
			"Page A (%d chars): %s\n\nPage B (%d chars): %s\n\n"+
			"Identify any logical contradictions, unsupported claims, or factual errors.",
		len(contentA), contentA[:min(2000, len(contentA))],
		len(contentB), contentB[:min(2000, len(contentB))],
	)

	response, err := d.LLMClient.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return d.parseGeneralValidation(response), nil
}

// parseGeneralValidation parses general validation responses.
func (d *ConflictDetector) parseGeneralValidation(response string) []Conflict {
	// Placeholder implementation
	return nil
}

// GetCachedResults retrieves cached conflict detection results.
func (d *ConflictDetector) GetCachedResults() (*Report, error) {
	// TODO: Implement caching logic
	// Store results in d.CacheDir with timestamp
	// Return nil if no cache found
	return nil, fmt.Errorf("caching not yet implemented")
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
