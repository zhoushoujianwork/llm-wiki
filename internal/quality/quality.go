// Package quality provides page quality evaluation services for LLM Wiki.
// This evaluates wiki pages based on multiple criteria and generates quality scores.
package quality

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

const defaultQualityCacheDir = ".quality_cache"

// QualityCriteria defines evaluation criteria for wiki pages.
type QualityCriteria struct {
	Completeness    float64 // 0-1: How complete is the content?
	Accuracy        float64 // 0-1: How factually accurate appears to be?
	Readability     float64 // 0-1: How readable is the text?
	Coherence       float64 // 0-1: How well do sections flow together?
	SourcesProvided bool    // Does it cite sources?
	CrossLinks      int     // Number of internal links
	TagsCount       int     // Number of tags/categorizations
}

// QualityScore represents overall page quality metrics.
type QualityScore struct {
	PaperID         string            `json:"page_id"`
	Path            string            `json:"path"`
	Namespace       string            `json:"namespace"`
	Title           string            `json:"title"`
	Overall         float64           `json:"overall"`
	Detailed        map[string]float64 `json:"detailed"`
	Criteria        QualityCriteria   `json:"criteria"`
	Issues          []string          `json:"issues"`
	Suggestions     []string          `json:"suggestions"`
	EvaluatedAt     time.Time         `json:"evaluated_at"`
}

// Report aggregates quality evaluation results.
type Report struct {
	TotalPages   int        `json:"total_pages"`
	PagesEvaluated int      `json:"pages_evaluated"`
	AverageScore float64    `json:"average_score"`
	QualityDist  QualityDistribution `json:"quality_distribution"`
	Summary      Summary    `json:"summary"`
	Timestamp    time.Time  `json:"timestamp"`
}

// QualityDistribution shows how many pages fall into each quality tier.
type QualityDistribution struct {
	Excellent int `json:"excellent"` // >= 90
	Good      int `json:"good"`      // 70-89
	Fair      int `json:"fair"`      // 50-69
	Poor      int `json:"poor"`    // < 50
}

// Summary provides high-level statistics about the report.
type Summary struct {
	Duration         string `json:"duration"`
	HighestRatedPage string `json:"highest_rated_page,omitempty"`
	LowestRatedPage  string `json:"lowest_rated_page,omitempty"`
	NeedsReviewCount int    `json:"needs_review_count"`
}

// Evaluator interface for quality assessment.
type Evaluator interface {
	// EvaluatePage assesses a single wiki page's quality.
	EvaluatePage(ctx context.Context, path string) (*QualityScore, error)

	// EvaluateAllPages scans all wiki pages for quality issues.
	EvaluateAllPages(ctx context.Context) (*Report, error)

	// GetCachedResults returns cached quality evaluation results if available.
	GetCachedResults() (*Report, error)

	// CacheResults stores quality evaluation results for later retrieval.
	CacheResults(report *Report) error
}

// QualityEvaluator implements Evaluator using heuristic + LLM-based analysis.
type QualityEvaluator struct {
	LLMClient llm.Client
	WikiStore *wiki.Store
	CacheDir  string
}

// NewQualityEvaluator creates a new QualityEvaluator instance.
func NewQualityEvaluator(client llm.Client, store *wiki.Store, cacheDir string) *QualityEvaluator {
	if cacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, ".llm-wiki", defaultQualityCacheDir)
	}
	return &QualityEvaluator{
		LLMClient: client,
		WikiStore: store,
		CacheDir:  cacheDir,
	}
}

// EvaluatePage assesses a single wiki page's quality.
func (e *QualityEvaluator) EvaluatePage(ctx context.Context, path string) (*QualityScore, error) {
	content, err := e.WikiStore.ReadPage(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read page %s: %w", path, err)
	}

	score := &QualityScore{
		Path:        path,
		Namespace:   extractNamespace(path),
		Title:       extractTitle(path),
		EvaluatedAt: time.Now(),
		Detailed:    make(map[string]float64),
	}

	// Calculate heuristic scores
	criteria := e.calculateHeuristicScores(content)
	score.Criteria = criteria
	score.Detailed["completeness"] = criteria.Completeness * 100
	score.Detailed["accuracy"] = criteria.Accuracy * 100
	score.Detailed["readability"] = criteria.Readability * 100
	score.Detailed["coherence"] = criteria.Coherence * 100

	// Calculate overall score (weighted average)
	weights := map[string]float64{
		"completeness":    0.3,
		"accuracy":        0.3,
		"readability":     0.2,
		"coherence":       0.2,
	}

	var weightedSum float64
	for metric, weight := range weights {
		weightedSum += score.Detailed[metric] * weight
	}
	score.Overall = weightedSum

	// Identify issues and suggestions
	issues, suggestions := e.analyzeIssues(content, criteria)
	score.Issues = issues
	score.Suggestions = suggestions

	return score, nil
}

// EvaluateAllPages scans all wiki pages for quality issues.
func (e *QualityEvaluator) EvaluateAllPages(ctx context.Context) (*Report, error) {
	start := time.Now()
	report := &Report{
		Timestamp: time.Now(),
	}

	// Get all pages
	pages, err := e.WikiStore.ListPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}
	report.TotalPages = len(pages)

	highestScore := -1.0
	lowestScore := 101.0
	var totalScore float64

	for _, path := range pages {
		score, err := e.EvaluatePage(ctx, path)
		if err != nil {
			continue
		}

		report.PagesEvaluated++
		totalScore += score.Overall

		// Track highest/lowest
		if score.Overall > highestScore {
			highestScore = score.Overall
			report.Summary.HighestRatedPage = path
		}
		if score.Overall < lowestScore {
			lowestScore = score.Overall
			report.Summary.LowestRatedPage = path
		}

		// Categorize by quality
		switch {
		case score.Overall >= 90:
			report.QualityDist.Excellent++
		case score.Overall >= 70:
			report.QualityDist.Good++
		case score.Overall >= 50:
			report.QualityDist.Fair++
		default:
			report.QualityDist.Poor++
		}

		// Count pages needing review (< 70)
		if score.Overall < 70 {
			report.Summary.NeedsReviewCount++
		}
	}

	if report.PagesEvaluated > 0 {
		report.AverageScore = totalScore / float64(report.PagesEvaluated)
	}

	report.Summary.Duration = time.Since(start).String()

	return report, nil
}

// calculateHeuristicScores calculates basic quality metrics from page content.
func (e *QualityEvaluator) calculateHeuristicScores(content string) QualityCriteria {
	criteria := QualityCriteria{}

	// Completeness: based on word count and structure
	hasHeader := strings.HasPrefix(strings.TrimSpace(content), "#")
	hasSections := strings.Contains(content, "##")
	wordCount := countWords(content)
	
	// Score completeness (0-1)
	completeness := 0.0
	if hasHeader {
		completeness += 0.2
	}
	if hasSections {
		completeness += 0.2
	}
	if wordCount > 500 {
		completeness += 0.3
	} else if wordCount > 200 {
		completeness += 0.2
	} else if wordCount > 100 {
		completeness += 0.1
	}
	criteria.Completeness = min(completeness, 1.0)

	// Accuracy: heuristic based on source citations
	hasSourceSection := strings.Contains(strings.ToLower(content), "source:") ||
		strings.Contains(strings.ToLower(content), "[[")
	criteria.SourcesProvided = hasSourceSection
	criteria.Accuracy = 0.7 // Base score, could be improved with LLM

	// Readability: sentence length, complexity
	sentenceCount := countSentences(content)
	avgSentenceLength := float64(wordCount) / float64(maxInt(sentenceCount, 1))
	readability := 1.0 - (min(avgSentenceLength, 40.0)/40.0)*0.5
	criteria.Readability = max(readability, 0.5)

	// Coherence: section transitions
	sectionCount := strings.Count(content, "##")
	coherence := 0.5
	if sectionCount >= 3 {
		coherence = 0.8
	} else if sectionCount >= 1 {
		coherence = 0.6
	}
	criteria.Coherence = coherence

	// Cross-links and tags
	criteria.CrossLinks = strings.Count(content, "[[")
	criteria.TagsCount = strings.Count(content, "- ## Tags")

	return criteria
}

// analyzeIssues identifies specific problems and suggests improvements.
func (e *QualityEvaluator) analyzeIssues(content string, criteria QualityCriteria) ([]string, []string) {
	var issues []string
	var suggestions []string

	if criteria.Completeness < 0.5 {
		issues = append(issues, "Page content appears incomplete")
		suggestions = append(suggestions, "Expand content with more detailed explanations")
	}

	if !criteria.SourcesProvided {
		issues = append(issues, "No source citation found")
		suggestions = append(suggestions, "Add source information using ## Source section")
	}

	if criteria.CrossLinks < 2 {
		issues = append(issues, "Limited cross-referencing")
		suggestions = append(suggestions, "Add links to related pages using [[Page Name]] syntax")
	}

	if criteria.TagsCount == 0 {
		suggestions = append(suggestions, "Consider adding tags for better categorization")
	}

	// Use LLM for more sophisticated analysis if client is available
	if e.LLMClient != nil {
		llmIssues, llmSuggestions := e.analyzeWithLLM(content)
		issues = append(issues, llmIssues...)
		suggestions = append(suggestions, llmSuggestions...)
	}

	return issues, suggestions
}

// analyzeWithLLM uses LLM for detailed quality analysis.
func (e *QualityEvaluator) analyzeWithLLM(content string) ([]string, []string) {
	var issues, suggestions []string

	prompt := fmt.Sprintf("Analyze this wiki page content for quality issues. "+
		"Content length: %d characters.\n\n%s\n\n"+
		"Return JSON with this exact structure:\n"+
		`{"issues": ["issue1", "issue2"], "suggestions": ["suggestion1"]}`+"\n\n"+
		"Be specific and actionable.",
		len(content), truncateAndEllipsis(content, 5000))

	response, err := e.LLMClient.Generate(context.Background(), prompt)
	if err != nil {
		return issues, suggestions // Return empty arrays on error
	}

	var parsed struct {
		Issues      []string `json:"issues"`
		Suggestions []string `json:"suggestions"`
	}

	if err := llm.UnmarshalJSONObject(response, &parsed); err == nil {
		issues = append(issues, parsed.Issues...)
		suggestions = append(suggestions, parsed.Suggestions...)
	}

	return issues, suggestions
}

// GetCachedResults retrieves cached quality evaluation results.
func (e *QualityEvaluator) GetCachedResults() (*Report, error) {
	cacheFile := filepath.Join(e.CacheDir, "quality.json")

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

	// Check if cache is stale (older than 7 days)
	if time.Since(report.Timestamp) > 7*24*time.Hour {
		return nil, fmt.Errorf("cached results are stale (>7d)")
	}

	return &report, nil
}

// CacheResults stores quality evaluation results for later retrieval.
func (e *QualityEvaluator) CacheResults(report *Report) error {
	if err := os.MkdirAll(e.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	cacheFile := filepath.Join(e.CacheDir, "quality.json")

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize report: %w", err)
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// Helper functions

func extractNamespace(path string) string {
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) > 1 {
		return parts[0]
	}
	return "root"
}

func extractTitle(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

func countWords(content string) int {
	words := strings.Fields(content)
	return len(words)
}

func countSentences(content string) int {
	sentences := strings.Split(content, ".")
	count := 0
	for _, s := range sentences {
		if strings.TrimSpace(s) != "" {
			count++
		}
	}
	return count
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func truncateAndEllipsis(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
