// Package scheduler provides task scheduling and execution services for LLM Wiki maintenance.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-wiki/internal/conflicts"
	"llm-wiki/internal/quality"
	"llm-wiki/internal/wiki"
)

const defaultSchedulerCacheDir = ".scheduler_cache"

// TaskType defines the type of maintenance task.
type TaskType string

const (
	TaskTypeConflictCheck   TaskType = "conflict_check"
	TaskTypeQualityAudit    TaskType = "quality_audit"
	TaskTypeOutdatedUpdate  TaskType = "outdated_update"
	TaskTypeLinkValidation  TaskType = "link_validation"
	TaskTypeFullMaintenance TaskType = "full_maintenance"
)

// ScheduleFrequency defines how often a task runs.
type ScheduleFrequency string

const (
	FrequencyOnce      ScheduleFrequency = "once"
	FrequencyHourly    ScheduleFrequency = "hourly"
	FrequencyDaily     ScheduleFrequency = "daily"
	FrequencyWeekly    ScheduleFrequency = "weekly"
	FrequencyMonthly   ScheduleFrequency = "monthly"
)

// Task represents a scheduled maintenance task.
type Task struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        TaskType       `json:"type"`
	Schedule    ScheduleFrequency `json:"schedule"`
	NextRun     time.Time      `json:"next_run"`
	LastRun     *time.Time     `json:"last_run,omitempty"`
	Status      TaskStatus     `json:"status"`
	Description string         `json:"description"`
	Priority    int            `json:"priority"` // 1-10, higher is more urgent
	Enabled     bool           `json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TaskStatus indicates the current state of a task.
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

// TaskResult contains the outcome of a task execution.
type TaskResult struct {
	TaskID    string    `json:"task_id"`
	Status    TaskStatus `json:"status"`
	Error     string    `json:"error,omitempty"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
	RunAt     time.Time `json:"run_at"`
	Duration  string    `json:"duration"`
}

// Manager handles task scheduling and execution.
type Manager struct {
	tasks        map[string]*Task
	cacheDir     string
	taskHandlers map[TaskType]TaskHandler
	detector     *conflicts.ConflictDetector
	evaluator    *quality.QualityEvaluator
	wikiStore    *wiki.Store
	executor     *TaskExecutor
}

// TaskHandler defines an interface for task execution.
type TaskHandler func(ctx context.Context, task *Task) (*TaskResult, error)

// NewManager creates a new TaskManager instance.
func NewManager(cacheDir string) *Manager {
	return NewManagerWithDeps(cacheDir, nil, nil)
}

// NewManagerWithDeps creates a new TaskManager instance with optional dependencies.
func NewManagerWithDeps(cacheDir string, detector *conflicts.ConflictDetector, evaluator *quality.QualityEvaluator) *Manager {
	return NewManagerWithAllDeps(cacheDir, detector, evaluator, nil)
}

// NewManagerWithAllDeps creates a new TaskManager instance with all dependencies.
func NewManagerWithAllDeps(cacheDir string, detector *conflicts.ConflictDetector, evaluator *quality.QualityEvaluator, store *wiki.Store) *Manager {
	if cacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, ".llm-wiki", defaultSchedulerCacheDir)
	}

	m := &Manager{
		tasks:        make(map[string]*Task),
		cacheDir:     cacheDir,
		taskHandlers: make(map[TaskType]TaskHandler),
		executor:     NewTaskExecutor(),
	}

	// Register default handlers
	m.RegisterHandler(TaskTypeConflictCheck, m.handleConflictCheck)
	m.RegisterHandler(TaskTypeQualityAudit, m.handleQualityAudit)
	m.RegisterHandler(TaskTypeOutdatedUpdate, m.handleOutdatedUpdate)
	m.RegisterHandler(TaskTypeLinkValidation, m.handleLinkValidation)
	m.RegisterHandler(TaskTypeFullMaintenance, m.handleFullMaintenance)

	// Inject dependencies if provided
	if detector != nil {
		m.detector = detector
	}
	if evaluator != nil {
		m.evaluator = evaluator
	}
	if store != nil {
		m.wikiStore = store
	}

	// Load existing tasks from disk
	m.loadTasks()

	return m
}

// RegisterHandler adds a custom task handler.
func (m *Manager) RegisterHandler(taskType TaskType, handler TaskHandler) {
	m.taskHandlers[taskType] = handler
}

// AddTask creates a new scheduled task.
func (m *Manager) AddTask(task *Task) error {
	if task.ID == "" {
		task.ID = generateTaskID()
	}

	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now

	// Set initial next run time
	task.NextRun = m.calculateNextRun(task.Schedule)

	m.tasks[task.ID] = task

	return m.saveTasks()
}

// GetTask retrieves a task by ID.
func (m *Manager) GetTask(taskID string) (*Task, error) {
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return task, nil
}

// ListTasks returns all registered tasks.
func (m *Manager) ListTasks() []*Task {
	tasks := make([]*Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// RemoveTask deletes a task by ID.
func (m *Manager) RemoveTask(taskID string) error {
	if _, ok := m.tasks[taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	delete(m.tasks, taskID)
	return m.saveTasks()
}

// EnableTask enables a task.
func (m *Manager) EnableTask(taskID string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}
	task.Enabled = true
	task.UpdatedAt = time.Now()
	return m.saveTasks()
}

// DisableTask disables a task.
func (m *Manager) DisableTask(taskID string) error {
	task, err := m.GetTask(taskID)
	if err != nil {
		return err
	}
	task.Enabled = false
	task.UpdatedAt = time.Now()
	return m.saveTasks()
}

// ExecuteTasks executes all pending tasks.
func (m *Manager) ExecuteTasks(ctx context.Context) ([]*TaskResult, error) {
	var results []*TaskResult
	now := time.Now()

	for _, task := range m.tasks {
		if !task.Enabled {
			continue
		}

		if task.NextRun.After(now) {
			continue
		}

		result, err := m.executor.Execute(ctx, task, m.taskHandlers)
		if err != nil {
			result = &TaskResult{
				TaskID: task.ID,
				Status: StatusFailed,
				Error:  err.Error(),
				RunAt:  now,
			}
		}

		results = append(results, result)

		// Update task status
		if task.LastRun == nil {
			task.LastRun = &now
		}
		task.NextRun = m.calculateNextRun(task.Schedule)
		task.UpdatedAt = now
	}

	return results, m.saveTasks()
}

// ExecuteNextTask executes only the next due task.
func (m *Manager) ExecuteNextTask(ctx context.Context) (*TaskResult, error) {
	var earliestTask *Task
	var earliestTime time.Time

	now := time.Now()
	for _, task := range m.tasks {
		if !task.Enabled || task.NextRun.After(now) {
			continue
		}
		if earliestTask == nil || task.NextRun.Before(earliestTime) {
			earliestTask = task
			earliestTime = task.NextRun
		}
	}

	if earliestTask == nil {
		return nil, fmt.Errorf("no tasks due for execution")
	}

	result, err := m.executor.Execute(ctx, earliestTask, m.taskHandlers)
	if err != nil {
		return &TaskResult{
			TaskID: earliestTask.ID,
			Status: StatusFailed,
			Error:  err.Error(),
			RunAt:  now,
		}, nil
	}

	// Update task
	now = time.Now()
	if earliestTask.LastRun == nil {
		earliestTask.LastRun = &now
	}
	earliestTask.NextRun = m.calculateNextRun(earliestTask.Schedule)
	earliestTask.UpdatedAt = now

	return result, m.saveTasks()
}

// handleConflictCheck executes conflict detection as a task.
func (m *Manager) handleConflictCheck(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	result := &TaskResult{
		TaskID: task.ID,
		Status: StatusCompleted,
		RunAt:  time.Now(),
		Metrics: make(map[string]interface{}),
	}

	if m.detector == nil {
		result.Status = StatusFailed
		result.Error = "conflict detector not configured"
		return result, fmt.Errorf("conflict detector not configured")
	}

	report, err := m.detector.ScanAllPages(ctx)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		return result, err
	}

	// Cache results
	if err := m.detector.CacheResults(report); err != nil {
		// Don't fail the task, just log
		fmt.Printf("⚠ Could not cache conflict results: %v\n", err)
	}

	result.Duration = time.Since(start).String()
	result.Metrics["pages_scanned"] = report.TotalPages
	result.Metrics["entities_checked"] = report.TotalEntities
	result.Metrics["conflicts_found"] = len(report.Conflicts)
	result.Metrics["high_confidence"] = report.Summary.HighConfidence
	result.Metrics["medium_confidence"] = report.Summary.MediumConfidence
	result.Metrics["low_confidence"] = report.Summary.LowConfidence

	return result, nil
}

// handleQualityAudit executes quality audit as a task.
func (m *Manager) handleQualityAudit(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	result := &TaskResult{
		TaskID: task.ID,
		Status: StatusCompleted,
		RunAt:  time.Now(),
		Metrics: make(map[string]interface{}),
	}

	if m.evaluator == nil {
		result.Status = StatusFailed
		result.Error = "quality evaluator not configured"
		return result, fmt.Errorf("quality evaluator not configured")
	}

	report, err := m.evaluator.EvaluateAllPages(ctx)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		return result, err
	}

	// Cache results
	if err := m.evaluator.CacheResults(report); err != nil {
		// Don't fail the task, just log
		fmt.Printf("⚠ Could not cache quality results: %v\n", err)
	}

	result.Duration = time.Since(start).String()
	result.Metrics["pages_evaluated"] = report.PagesEvaluated
	result.Metrics["average_score"] = report.AverageScore
	result.Metrics["excellent"] = report.QualityDist.Excellent
	result.Metrics["good"] = report.QualityDist.Good
	result.Metrics["fair"] = report.QualityDist.Fair
	result.Metrics["poor"] = report.QualityDist.Poor
	result.Metrics["needs_review"] = report.Summary.NeedsReviewCount

	return result, nil
}

// handleOutdatedUpdate checks for potentially outdated content.
func (m *Manager) handleOutdatedUpdate(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	result := &TaskResult{
		TaskID: task.ID,
		Status: StatusCompleted,
		RunAt:  time.Now(),
		Metrics: make(map[string]interface{}),
	}

	if m.wikiStore == nil {
		result.Metrics["pages_reviewed"] = 0
		result.Metrics["flagged_outdated"] = 0
		return result, nil
	}

	// Get all pages and check their modification times
	pages, err := m.wikiStore.ListPages()
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		return result, err
	}

	outdatedThreshold := 30 * 24 * time.Hour // 30 days
	flaggedCount := 0

	for _, path := range pages {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > outdatedThreshold {
			flaggedCount++
		}
	}

	result.Duration = time.Since(start).String()
	result.Metrics["pages_reviewed"] = len(pages)
	result.Metrics["flagged_outdated"] = flaggedCount

	return result, nil
}

// handleLinkValidation validates internal links.
func (m *Manager) handleLinkValidation(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	result := &TaskResult{
		TaskID: task.ID,
		Status: StatusCompleted,
		RunAt:  time.Now(),
		Metrics: make(map[string]interface{}),
	}

	if m.wikiStore == nil {
		result.Metrics["links_checked"] = 0
		result.Metrics["broken_links"] = 0
		return result, nil
	}

	// Get all pages
	pages, err := m.wikiStore.ListPages()
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		return result, err
	}

	// Build index of existing pages
	existingPages := make(map[string]bool)
	for _, path := range pages {
		basename := strings.TrimSuffix(filepath.Base(path), ".md")
		existingPages[basename] = true
	}

	// Also use the store's entity index
	entityIndex := m.wikiStore.GetEntities()
	for pageName := range entityIndex {
		existingPages[pageName] = true
	}

	// Scan all pages for broken links
	totalLinks := 0
	brokenLinks := 0
	brokenLinkDetails := []string{}

	for _, path := range pages {
		content, err := m.wikiStore.ReadPage(path)
		if err != nil {
			continue
		}

		// Extract [[link]] patterns
		links := extractWikiLinks(content)
		for _, link := range links {
			totalLinks++
			linkBase := strings.TrimSpace(link)
			if !existingPages[linkBase] {
				brokenLinks++
				if len(brokenLinkDetails) < 10 { // Limit details
					brokenLinkDetails = append(brokenLinkDetails, fmt.Sprintf("%s -> %s", filepath.Base(path), link))
				}
			}
		}
	}

	result.Duration = time.Since(start).String()
	result.Metrics["links_checked"] = totalLinks
	result.Metrics["broken_links"] = brokenLinks
	result.Metrics["broken_link_samples"] = brokenLinkDetails

	return result, nil
}

// handleFullMaintenance runs all maintenance checks.
func (m *Manager) handleFullMaintenance(ctx context.Context, task *Task) (*TaskResult, error) {
	start := time.Now()
	result := &TaskResult{
		TaskID: task.ID,
		Status: StatusCompleted,
		RunAt:  time.Now(),
		Metrics: make(map[string]interface{}),
	}

	// Run all sub-tasks sequentially
	handlers := []TaskType{
		TaskTypeConflictCheck,
		TaskTypeQualityAudit,
		TaskTypeOutdatedUpdate,
		TaskTypeLinkValidation,
	}

	var totalConflicts int
	var totalNeedsReview int

	for _, handler := range handlers {
		if h, ok := m.taskHandlers[handler]; ok {
			subResult, err := h(ctx, task)
			if err == nil && subResult != nil {
				// Aggregate metrics
				if metrics := subResult.Metrics; metrics != nil {
					switch handler {
					case TaskTypeConflictCheck:
						if conflicts, ok := metrics["conflicts_found"].(int); ok {
							totalConflicts += conflicts
						}
					case TaskTypeQualityAudit:
						if needsReview, ok := metrics["needs_review"].(int); ok {
							totalNeedsReview += needsReview
						}
					}
				}
			}
		}
	}

	result.Metrics["total_conflicts"] = totalConflicts
	result.Metrics["total_needs_review"] = totalNeedsReview
	result.Duration = time.Since(start).String()

	return result, nil
}

// calculateNextRun calculates when the next execution should occur.
func (m *Manager) calculateNextRun(schedule ScheduleFrequency) time.Time {
	now := time.Now()

	switch schedule {
	case FrequencyOnce:
		return now.Add(24 * time.Hour) // Run once after 24h
	case FrequencyHourly:
		return now.Add(time.Hour)
	case FrequencyDaily:
		return now.Add(24 * time.Hour)
	case FrequencyWeekly:
		return now.Add(7 * 24 * time.Hour)
	case FrequencyMonthly:
		return now.Add(30 * 24 * time.Hour)
	default:
		return now.Add(time.Hour)
	}
}

// saveTasks persists tasks to disk.
func (m *Manager) saveTasks() error {
	if err := os.MkdirAll(m.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	tasksFile := filepath.Join(m.cacheDir, "tasks.json")

	data, err := json.MarshalIndent(m.tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize tasks: %w", err)
	}

	if err := os.WriteFile(tasksFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tasks: %w", err)
	}

	return nil
}

// loadTasks loads tasks from disk.
func (m *Manager) loadTasks() error {
	tasksFile := filepath.Join(m.cacheDir, "tasks.json")

	data, err := os.ReadFile(tasksFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing tasks - create default tasks
			defaultTasks := CreateDefaultTasks()
			for _, task := range defaultTasks {
				m.tasks[task.ID] = task
			}
			return m.saveTasks()
		}
		return fmt.Errorf("failed to read tasks file: %w", err)
	}

	var tasksMap map[string]*Task
	if err := json.Unmarshal(data, &tasksMap); err != nil {
		return fmt.Errorf("failed to parse tasks: %w", err)
	}

	m.tasks = tasksMap
	return nil
}

// generateTaskID creates a unique task ID.
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

// Default Tasks for quick setup

// CreateDefaultTasks returns a list of recommended default tasks.
func CreateDefaultTasks() []*Task {
	now := time.Now()
	return []*Task{
		{
			ID:          "default_conflict_check",
			Name:        "Daily Conflict Detection",
			Type:        TaskTypeConflictCheck,
			Schedule:    FrequencyDaily,
			NextRun:     now.Add(time.Hour),
			Status:      StatusPending,
			Description: "Automatically detect conflicts between wiki pages about the same entity",
			Priority:    8,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "default_quality_audit",
			Name:        "Weekly Quality Audit",
			Type:        TaskTypeQualityAudit,
			Schedule:    FrequencyWeekly,
			NextRun:     now.Add(7 * 24 * time.Hour),
			Status:      StatusPending,
			Description: "Evaluate wiki page quality and identify pages needing improvement",
			Priority:    6,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "default_link_validation",
			Name:        "Monthly Link Validation",
			Type:        TaskTypeLinkValidation,
			Schedule:    FrequencyMonthly,
			NextRun:     now.Add(30 * 24 * time.Hour),
			Status:      StatusPending,
			Description: "Check all internal links and report broken references",
			Priority:    4,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
}

// TaskExecutor handles actual task execution with timeout and logging.
type TaskExecutor struct {
	timeout time.Duration
}

// NewTaskExecutor creates a new TaskExecutor.
func NewTaskExecutor() *TaskExecutor {
	return &TaskExecutor{timeout: 30 * time.Minute}
}

// Execute runs a single task with timeout enforcement.
func (e *TaskExecutor) Execute(ctx context.Context, task *Task, handlers map[TaskType]TaskHandler) (*TaskResult, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	handler, ok := handlers[task.Type]
	if !ok {
		return &TaskResult{
			TaskID: task.ID,
			Status: StatusFailed,
			Error:  fmt.Sprintf("no handler registered for task type: %s", task.Type),
			RunAt:  start,
		}, fmt.Errorf("no handler for task type %s", task.Type)
	}

	return handler(ctx, task)
}

// truncateAndEllipsis truncates a string with ellipsis.
func truncateAndEllipsis(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// min returns minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// extractWikiLinks extracts [[link]] patterns from markdown content.
func extractWikiLinks(content string) []string {
	var links []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		start := strings.Index(line, "[[")
		for start != -1 {
			end := strings.Index(line[start+2:], "]]")
			if end == -1 {
				break
			}
			link := strings.TrimSpace(line[start+2 : start+2+end])
			if link != "" {
				links = append(links, link)
			}
			nextStart := strings.Index(line[start+2+end+2:], "[[")
			if nextStart == -1 {
				break
			}
			start = start + 2 + end + 2 + nextStart
		}
	}
	return links
}
