// Package feedback provides user feedback collection and processing services for LLM Wiki maintenance.
package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"llm-wiki/internal/quality"
)

const defaultFeedbackCacheDir = ".feedback_cache"

// FeedbackType represents the type of feedback.
type FeedbackType string

const (
	FeedbackTypeError         FeedbackType = "error"          // Page contains factual errors
	FeedbackTypeOutdated      FeedbackType = "outdated"       // Information is outdated
	FeedbackTypeIncomplete    FeedbackType = "incomplete"     // Page lacks necessary information
	FeedbackTypeUnclear       FeedbackType = "unclear"        // Writing is confusing
	FeedbackTypeBrokenLink    FeedbackType = "broken_link"    // Link points to non-existent page
	FeedbackTypeSuggestion    FeedbackType = "suggestion"     // Improvement suggestion
	FeedbackTypeDuplicate     FeedbackType = "duplicate"      // Content exists elsewhere
	FeedbackTypeOther         FeedbackType = "other"          // Other feedback
)

// FeedbackStatus indicates the current state of feedback.
type FeedbackStatus string

const (
	StatusNew         FeedbackStatus = "new"           // Just submitted, not reviewed
	StatusInReview    FeedbackStatus = "in_review"     // Being reviewed by maintainer
	StatusResolved    FeedbackStatus = "resolved"      // Addressed or will be addressed
	StatusRejected    FeedbackStatus = "rejected"      // Not actionable
	StatusArchived    FeedbackStatus = "archived"      // Closed without action
)

// Feedback represents user feedback about wiki content.
type Feedback struct {
	ID          string        `json:"id"`
	PagePath    string        `json:"page_path"`
	Namespace   string        `json:"namespace"`
	Title       string        `json:"title"`
	Type        FeedbackType  `json:"type"`
	Description string        `json:"description"`
	Priority    int           `json:"priority"` // 1-5, higher is more urgent
	Status      FeedbackStatus `json:"status"`
	Source      string        `json:"source,omitempty"` // Where feedback came from (user, automated, etc.)
	Correlated  []string      `json:"correlated_feedback,omitempty"` // Related feedback IDs
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	ResolvedAt  *time.Time    `json:"resolved_at,omitempty"`
	Resolution  string        `json:"resolution,omitempty"` // How it was resolved
}

// FeedbackStats holds aggregated feedback statistics.
type FeedbackStats struct {
	Total          int                   `json:"total"`
	ByType         map[FeedbackType]int  `json:"by_type"`
	ByStatus       map[FeedbackStatus]int `json:"by_status"`
	AvgPriority    float64               `json:"avg_priority"`
	ResolutionRate float64               `json:"resolution_rate"`
	MostRecent     time.Time             `json:"most_recent"`
}

// Collector interface for feedback management.
type Collector interface {
	// SubmitFeedback records new feedback.
	SubmitFeedback(ctx context.Context, feedback *Feedback) error

	// GetFeedback retrieves feedback by ID.
	GetFeedback(feedbackID string) (*Feedback, error)

	// ListFeedback returns all feedback, optionally filtered.
	ListFeedback(ctx context.Context, filters *FeedbackFilters) ([]*Feedback, error)

	// UpdateStatus changes feedback status.
	UpdateStatus(feedbackID string, status FeedbackStatus, resolution string) error

	// CorrelateFeedback links related feedback items.
	CorrelateFeedback(feedbackID string, relatedIDs []string) error

	// GetStats returns feedback statistics.
	GetStats(ctx context.Context) (*FeedbackStats, error)

	// ExportFeedback exports all feedback for backup/migration.
	ExportFeedback() ([]byte, error)

	// ImportFeedback imports feedback from JSON export.
	ImportFeedback(data []byte) error
}

// Collector implementation.
type CollectorImpl struct {
	cacheDir string
	feedback map[string]*Feedback
}

// NewCollector creates a new FeedbackCollector instance.
func NewCollector(cacheDir string) *CollectorImpl {
	if cacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, ".llm-wiki", defaultFeedbackCacheDir)
	}

	c := &CollectorImpl{
		cacheDir: cacheDir,
		feedback: make(map[string]*Feedback),
	}

	c.loadFeedback()
	return c
}

// SubmitFeedback records new feedback.
func (c *CollectorImpl) SubmitFeedback(ctx context.Context, feedback *Feedback) error {
	if feedback.ID == "" {
		feedback.ID = generateFeedbackID()
	}

	now := time.Now()
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = now
	}
	feedback.UpdatedAt = now
	if feedback.Status == "" {
		feedback.Status = StatusNew
	}
	if feedback.Priority == 0 {
		feedback.Priority = 3 // Default medium priority
	}

	// Auto-extract namespace and title from path
	if feedback.PagePath != "" {
		feedback.Namespace = extractNamespace(feedback.PagePath)
		feedback.Title = extractTitle(feedback.PagePath)
	}

	c.feedback[feedback.ID] = feedback

	return c.saveFeedback()
}

// GetFeedback retrieves feedback by ID.
func (c *CollectorImpl) GetFeedback(feedbackID string) (*Feedback, error) {
	fb, ok := c.feedback[feedbackID]
	if !ok {
		return nil, fmt.Errorf("feedback not found: %s", feedbackID)
	}
	return fb, nil
}

// FeedbackFilters defines filtering criteria.
type FeedbackFilters struct {
	Type       FeedbackType
	Status     FeedbackStatus
	Namespace  string
	PagePath   string
	MinPriority int
	MaxResults int
}

// ListFeedback returns all feedback, optionally filtered.
func (c *CollectorImpl) ListFeedback(ctx context.Context, filters *FeedbackFilters) ([]*Feedback, error) {
	var results []*Feedback

	for _, fb := range c.feedback {
		// Apply filters
		if filters.Type != "" && fb.Type != filters.Type {
			continue
		}
		if filters.Status != "" && fb.Status != filters.Status {
			continue
		}
		if filters.Namespace != "" && fb.Namespace != filters.Namespace {
			continue
		}
		if filters.PagePath != "" && fb.PagePath != filters.PagePath {
			continue
		}
		if filters.MinPriority > 0 && fb.Priority < filters.MinPriority {
			continue
		}

		results = append(results, fb)
	}

	// Sort by priority (descending) then created_at (ascending)
	sortFeedbackByPriorityAndDate(results)

	// Limit results if specified
	if filters.MaxResults > 0 && len(results) > filters.MaxResults {
		results = results[:filters.MaxResults]
	}

	return results, nil
}

// UpdateStatus changes feedback status.
func (c *CollectorImpl) UpdateStatus(feedbackID string, status FeedbackStatus, resolution string) error {
	fb, ok := c.feedback[feedbackID]
	if !ok {
		return fmt.Errorf("feedback not found: %s", feedbackID)
	}

	oldStatus := fb.Status
	fb.Status = status
	fb.UpdatedAt = time.Now()

	if status == StatusResolved || status == StatusRejected {
		fb.Resolution = resolution
		now := time.Now()
		fb.ResolvedAt = &now
	}

	c.feedback[feedbackID] = fb

	// Update correlated feedback status if parent changed
	if oldStatus == StatusNew && (status == StatusResolved || status == StatusRejected) {
		for _, relatedID := range fb.Correlated {
			if relatedFb, ok := c.feedback[relatedID]; ok {
				if relatedFb.Status == StatusNew {
					relatedFb.Status = StatusInReview
					relatedFb.UpdatedAt = fb.UpdatedAt
					c.feedback[relatedID] = relatedFb
				}
			}
		}
	}

	return c.saveFeedback()
}

// CorrelateFeedback links related feedback items.
func (c *CollectorImpl) CorrelateFeedback(feedbackID string, relatedIDs []string) error {
	fb, ok := c.feedback[feedbackID]
	if !ok {
		return fmt.Errorf("feedback not found: %s", feedbackID)
	}

	seen := make(map[string]bool)
	cleaned := make([]string, 0, len(relatedIDs))
	
	for _, id := range relatedIDs {
		if _, exists := c.feedback[id]; exists && !seen[id] {
			cleaned = append(cleaned, id)
			seen[id] = true
		}
	}

	fb.Correlated = cleaned
	fb.UpdatedAt = time.Now()
	c.feedback[feedbackID] = fb

	return c.saveFeedback()
}

// GetStats returns feedback statistics.
func (c *CollectorImpl) GetStats(ctx context.Context) (*FeedbackStats, error) {
	stats := &FeedbackStats{
		ByType:     make(map[FeedbackType]int),
		ByStatus:   make(map[FeedbackStatus]int),
	}

	var totalPriority int
	now := time.Time{}

	for _, fb := range c.feedback {
		stats.Total++
		stats.ByType[fb.Type]++
		stats.ByStatus[fb.Status]++
		totalPriority += fb.Priority

		if fb.CreatedAt.After(now) || now.IsZero() {
			now = fb.CreatedAt
		}
	}

	if stats.Total > 0 {
		stats.AvgPriority = float64(totalPriority) / float64(stats.Total)
		
		resolvedCount := stats.ByStatus[StatusResolved] + stats.ByStatus[StatusRejected]
		stats.ResolutionRate = float64(resolvedCount) / float64(stats.Total)
	}

	stats.MostRecent = now

	return stats, nil
}

// ExportFeedback exports all feedback as JSON.
func (c *CollectorImpl) ExportFeedback() ([]byte, error) {
	data, err := json.MarshalIndent(c.feedback, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize feedback: %w", err)
	}
	return data, nil
}

// ImportFeedback imports feedback from JSON export.
func (c *CollectorImpl) ImportFeedback(data []byte) error {
	var importedMap map[string]*Feedback
	
	if err := json.Unmarshal(data, &importedMap); err != nil {
		return fmt.Errorf("failed to parse feedback data: %w", err)
	}

	// Merge imported feedback
	for id, fb := range importedMap {
		if existing, ok := c.feedback[id]; ok {
			// Keep existing if newer
			if fb.CreatedAt.Before(existing.CreatedAt) {
				importedMap[id] = existing
			}
		}
	}

	c.feedback = importedMap

	return c.saveFeedback()
}

// loadFeedback loads feedback from disk.
func (c *CollectorImpl) loadFeedback() error {
	feedbackFile := filepath.Join(c.cacheDir, "feedback.json")

	data, err := os.ReadFile(feedbackFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read feedback file: %w", err)
	}

	var feedbackMap map[string]*Feedback
	if err := json.Unmarshal(data, &feedbackMap); err != nil {
		return fmt.Errorf("failed to parse feedback: %w", err)
	}

	c.feedback = feedbackMap
	return nil
}

// saveFeedback persists feedback to disk.
func (c *CollectorImpl) saveFeedback() error {
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	feedbackFile := filepath.Join(c.cacheDir, "feedback.json")

	data, err := json.MarshalIndent(c.feedback, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize feedback: %w", err)
	}

	if err := os.WriteFile(feedbackFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write feedback: %w", err)
	}

	return nil
}

// getFeedbackByPage returns all feedback for a specific page.
func (c *CollectorImpl) getFeedbackByPage(pagePath string) ([]*Feedback, error) {
	var results []*Feedback

	for _, fb := range c.feedback {
		if fb.PagePath == pagePath {
			results = append(results, fb)
		}
	}

	return results, nil
}

// getHighPriorityFeedback returns pending high-priority feedback.
func (c *CollectorImpl) getHighPriorityFeedback(maxCount int) ([]*Feedback, error) {
	highPriority := []*Feedback{}

	for _, fb := range c.feedback {
		if (fb.Status == StatusNew || fb.Status == StatusInReview) && fb.Priority >= 4 {
			highPriority = append(highPriority, fb)
		}
	}

	sortFeedbackByPriorityAndDate(highPriority)

	if len(highPriority) > maxCount {
		highPriority = highPriority[:maxCount]
	}

	return highPriority, nil
}

// integrateWithQualityEvaluation integrates feedback into quality evaluation.
func (c *CollectorImpl) IntegrateWithQualityEvaluation(evaluator *quality.QualityEvaluator, ctx context.Context) error {
	// Get all pages with unresolved feedback
	allFeedback, _ := c.ListFeedback(ctx, &FeedbackFilters{})

	// Group by page
	pageToFeedback := make(map[string][]*Feedback)
	for _, fb := range allFeedback {
		if fb.PagePath != "" {
			pageToFeedback[fb.PagePath] = append(pageToFeedback[fb.PagePath], fb)
		}
	}

	// For each page with feedback, re-evaluate quality
	for pagePath, feedbacks := range pageToFeedback {
		if evaluator == nil {
			continue
		}

		score, err := evaluator.EvaluatePage(ctx, pagePath)
		if err != nil {
			continue
		}

		// Adjust score based on unresolved feedback
		unresolved := filterFeedbackByStatus(feedbacks, StatusNew, StatusInReview)
		if len(unresolved) > 0 {
			// Reduce score based on number of issues
			adjustment := float64(len(unresolved)) * 2.0
			if score.Overall-adjustment > 0 {
				score.Overall -= adjustment
			}
		}

		// Add feedback sources to suggestions
		for _, fb := range unresolved {
			switch fb.Type {
			case FeedbackTypeError, FeedbackTypeOutdated:
				score.Issues = append(score.Issues, fmt.Sprintf("User-reported issue: %s", fb.Description))
			case FeedbackTypeSuggestion:
				score.Suggestions = append(score.Suggestions, fb.Description)
			}
		}

		// Cache updated score could be added here
		_ = score // Use the score somehow
	}

	return nil
}

// Helper functions

func generateFeedbackID() string {
	return fmt.Sprintf("feedback_%d", time.Now().UnixNano())
}

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

func sortFeedbackByPriorityAndDate(fbs []*Feedback) {
	sort.Slice(fbs, func(i, j int) bool {
		if fbs[i].Priority != fbs[j].Priority {
			return fbs[i].Priority > fbs[j].Priority
		}
		return fbs[i].CreatedAt.Before(fbs[j].CreatedAt)
	})
}

func filterFeedbackByStatus(feedbacks []*Feedback, statuses ...FeedbackStatus) []*Feedback {
	statusMap := make(map[FeedbackStatus]bool)
	for _, s := range statuses {
		statusMap[s] = true
	}

	var filtered []*Feedback
	for _, fb := range feedbacks {
		if statusMap[fb.Status] {
			filtered = append(filtered, fb)
		}
	}

	return filtered
}
