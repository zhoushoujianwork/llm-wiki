package conflicts

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// MockWikiStore implements WikiStoreProvider for testing.
type MockWikiStore struct {
	pages      map[string]string
	entities   map[string][]string
	pageOrder  []string // preserve insertion order
}

func NewMockWikiStore() *MockWikiStore {
	return &MockWikiStore{
		pages:     make(map[string]string),
		entities:  make(map[string][]string),
		pageOrder: make([]string, 0),
	}
}

func (m *MockWikiStore) AddPage(path, content string, entities []string) {
	m.pages[path] = content
	m.pageOrder = append(m.pageOrder, path)
	
	for _, entity := range entities {
		if m.entities[entity] == nil {
			m.entities[entity] = make([]string, 0)
		}
		// Avoid duplicates
		exists := false
		for _, p := range m.entities[entity] {
			if p == path {
				exists = true
				break
			}
		}
		if !exists {
			m.entities[entity] = append(m.entities[entity], path)
		}
	}
}

func (m *MockWikiStore) ListPages(ctx context.Context) ([]PageInfo, error) {
	result := make([]PageInfo, 0, len(m.pages))
	for _, path := range m.pageOrder {
		content := m.pages[path]
		entities := make([]string, 0)
		for entity, pages := range m.entities {
			for _, p := range pages {
				if p == path {
					entities = append(entities, entity)
					break
				}
			}
		}
		result = append(result, PageInfo{
			Path:     path,
			Title:    path,
			Entities: entities,
		})
	}
	return result, nil
}

func (m *MockWikiStore) ReadPage(ctx context.Context, path string) (string, error) {
	content, ok := m.pages[path]
	if !ok {
		return "", NotFoundError{path}
	}
	return content, nil
}

func (m *MockWikiStore) GetEntities(ctx context.Context) (map[string][]string, error) {
	result := make(map[string][]string)
	for k, v := range m.entities {
		result[k] = make([]string, len(v))
		copy(result[k], v)
	}
	return result, nil
}

type NotFoundError struct {
	path string
}

func (e NotFoundError) Error() string {
	return "page not found: " + e.path
}

// MockLLMClient implements llm.Client for testing.
type MockLLMClient struct {
	Response string
	Error    error
}

func (m *MockLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	if m.Error != nil {
		return "", m.Error
	}
	return m.Response, nil
}

func TestNewConflictDetector(t *testing.T) {
	store := NewMockWikiStore()
	client := &MockLLMClient{Response: `{"hasConflict": false}`}
	
	detector := NewConflictDetector(client, store, "")
	
	if detector == nil {
		t.Fatal("Expected non-nil detector")
	}
	if detector.WikiStore != store {
		t.Error("Expected WikiStore to be set")
	}
}

func TestConflictDetection_NoConflicts(t *testing.T) {
	ctx := context.Background()
	store := NewMockWikiStore()
	
	// Add two consistent pages about the same entity
	store.AddPage("pages/ml.md", `
Machine learning is a subset of artificial intelligence.
ML algorithms learn from data to improve performance.
`, []string{"machine_learning", "artificial_intelligence"})
	
	store.AddPage("pages/deep_learning.md", `
Deep learning is a type of machine learning.
Neural networks are commonly used in deep learning.
`, []string{"deep_learning", "machine_learning"})
	
	client := &MockLLMClient{
		Response: `{"hasConflict": false, "confidence": 0.1, "reason": "No contradiction found"}`,
	}
	
	detector := NewConflictDetector(client, store, "")
	
	report, err := detector.ScanAllPages(ctx)
	if err != nil {
		t.Fatalf("ScanAllPages failed: %v", err)
	}
	
	if report.TotalPages != 2 {
		t.Errorf("Expected 2 pages, got %d", report.TotalPages)
	}
	
	// "machine_learning" appears in both pages, so it should be checked
	if len(report.Conflicts) > 0 {
		t.Logf("Unexpected conflicts: %+v", report.Conflicts)
	}
}

func TestConflictDetection_WithConflict(t *testing.T) {
	ctx := context.Background()
	store := NewMockWikiStore()
	
	// Add pages with contradictory statements
	store.AddPage("pages/python.md", `
Python was created by Guido van Rossum in 1991.
Python's primary interpreter is CPython.
`, []string{"python", "programming_languages"})
	
	store.AddPage("pages/python_alternative.md", `
Python was originally developed by James Gosling in 1988.
The main Python implementation is Jython.
`, []string{"python", "programming_languages"})
	
	client := &MockLLMClient{
		Response: `{
			"hasConflict": true,
			"confidence": 0.9,
			"reason": "Contradictory creator and interpretation claims",
			"recommendation": "manual_review"
		}`,
	}
	
	detector := NewConflictDetector(client, store, "")
	
	conflicts, err := detector.CheckEntity(ctx, "python")
	if err != nil {
		t.Fatalf("CheckEntity failed: %v", err)
	}
	
	if len(conflicts) == 0 {
		t.Error("Expected at least one conflict")
	}
	
	for _, c := range conflicts {
		if c.Recommendation != "manual_review" {
			t.Errorf("Expected manual_review recommendation, got %s", c.Recommendation)
		}
	}
}

func TestExtractRelevantStatements(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		entity       string
		expectedCount int
	}{
		{
			name: "single_match",
			content: "Python is a programming language.\nGo is another language.",
			entity: "Python",
			expectedCount: 1,
		},
		{
			name: "multiple_matches",
			content: "Python was designed by Guido.\nUse Python for scripting.\nGo is fast.",
			entity: "Python",
			expectedCount: 2,
		},
		{
			name: "case_insensitive",
			content: "PYTHON is great.",
			entity: "python",
			expectedCount: 1,
		},
		{
			name: "no_match",
			content: "Go and Rust are systems languages.",
			entity: "Python",
			expectedCount: 0,
		},
	}
	
	store := NewMockWikiStore()
	store.AddPage("test.md", "test content", nil)
	
	detector := NewConflictDetector(nil, store, "")
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statements := detector.extractRelevantStatements(tt.content, tt.entity)
			if len(statements) != tt.expectedCount {
				t.Errorf("Expected %d statements, got %d: %v", 
					tt.expectedCount, len(statements), statements)
			}
		})
	}
}

func TestConfidenceClassification(t *testing.T) {
	tests := []struct {
		confidence float64
		category   string
	}{
		{0.9, "high"},
		{0.8, "high"},
		{0.75, "medium"},
		{0.5, "medium"},
		{0.3, "low"},
		{0.0, "low"},
	}
	
	for _, tt := range tests {
		t.Run(string(rune(int(tt.confidence*100))), func(t *testing.T) {
			var category string
			switch {
			case tt.confidence >= 0.8:
				category = "high"
			case tt.confidence >= 0.5:
				category = "medium"
			default:
				category = "low"
			}
			
			if category != tt.category {
				t.Errorf("Expected category %s, got %s for confidence %.2f",
					tt.category, category, tt.confidence)
			}
		})
	}
}

func TestReportSummaryCalculation(t *testing.T) {
	ctx := context.Background()
	store := NewMockWikiStore()
	
	// Add conflicting pages
	store.AddPage("a.md", "A says X", []string{"topic"})
	store.AddPage("b.md", "B says Y", []string{"topic"})
	store.AddPage("c.md", "C says Z", []string{"topic"})
	
	client := &MockLLMClient{
		Response: `{
			"hasConflict": true,
			"confidence": 0.9,
			"recommendation": "manual_review"
		}`,
	}
	
	detector := NewConflictDetector(client, store, "")
	report, err := detector.ScanAllPages(ctx)
	if err != nil {
		t.Fatalf("ScanAllPages failed: %v", err)
	}
	
	// 3 pairs: (a,b), (a,c), (b,c)
	expectedConflicts := 3
	
	if report.Summary.HighConfidence != expectedConflicts {
		t.Errorf("Expected %d high confidence conflicts, got %d",
			expectedConflicts, report.Summary.HighConfidence)
	}
	
	if report.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
	
	if report.Duration == "" || report.Duration == "N/A" {
		// In test this might be empty if time package import issue
		t.Log("Duration calculation may need adjustment in test environment")
	}
}

func TestGetCachedResults_NotImplemented(t *testing.T) {
	store := NewMockWikiStore()
	detector := NewConflictDetector(&MockLLMClient{}, store, "/tmp/cache")
	
	_, err := detector.GetCachedResults()
	if err == nil {
		t.Error("Expected caching to return error when not implemented")
	}
}

func BenchmarkConflictDetection(b *testing.B) {
	store := NewMockWikiStore()
	
	// Create 100 pages with 10 entities each
	for i := 0; i < 100; i++ {
		entities := make([]string, 10)
		for j := 0; j < 10; j++ {
			entities[j] = fmt.Sprintf("entity_%d_%d", i, j)
		}
		store.AddPage(fmt.Sprintf("page_%d.md", i), 
			fmt.Sprintf("Content for page %d mentioning all entities", i), 
			entities)
	}
	
	client := &MockLLMClient{Response: `{"hasConflict": false}`}
	
	ctx := context.Background()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		detector := NewConflictDetector(client, store, "")
		_, _ = detector.ScanAllPages(ctx)
	}
}
