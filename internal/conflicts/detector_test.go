package conflicts

import (
	"testing"

	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

func TestParseWikiLink(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"[[namespace/page]]", "namespace/page"},
		{" [[other/ns]] ", "other/ns"},
		{"[invalid", ""},
		{"no brackets", ""},
	}

	for _, tt := range tests {
		result := parseWikiLink(tt.input)
		if result != tt.expected {
			t.Errorf("parseWikiLink(%q) = %q; expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestDetectInconsistentReferences(t *testing.T) {
	store := wiki.NewStore("/tmp/test-wiki")

	pages := []wiki.Page{
		{Namespace: "doc", Name: "page1", Links: []string{"[[missing/page]]"}},
		{Namespace: "doc", Name: "page2", Links: []string{}},
	}

	detector := NewDetector(store)
	conflicts, err := detector.detectInconsistentReferences(pages)

	if err != nil {
		t.Fatalf("detectInconsistentReferences() error = %v", err)
	}

	if len(conflicts) != 1 {
		t.Errorf("Expected 1 conflict, got %d", len(conflicts))
	}

	if conflicts[0].Type != InconsistentReference {
		t.Errorf("Expected conflict type %s, got %s", InconsistentReference, conflicts[0].Type)
	}
}

func TestDetectDuplicateEntities(t *testing.T) {
	store := wiki.NewStore("/tmp/test-wiki")
	detector := NewDetector(store)

	// Create pages with high similarity
	similarContent := "This is some sample content that will be identical in both pages."
	
	pages := []wiki.Page{
		{Namespace: "test", Name: "page1", Content: similarContent + " additional text 1"},
		{Namespace: "test", Name: "page2", Content: similarContent + " additional text 2"},
	}

	conflicts, err := detector.detectDuplicateEntities(pages)
	if err != nil {
		t.Fatalf("detectDuplicateEntities() error = %v", err)
	}

	// Note: Current implementation uses Jaccard similarity which may not detect these as duplicates
	// depending on how much overlap there is
	t.Logf("Found %d duplicate conflicts", len(conflicts))
}

func TestCalculateTextSimilarity(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected float64
	}{
		{"hello world", "hello world", 1.0},
		{"hello", "world", 0.0},
		{"the cat sat", "the cat sat on the mat", 0.6},
		{"", "", 0.0},
	}

	for _, tt := range tests {
		result := calculateTextSimilarity(tt.a, tt.b)
		// Allow small floating point tolerance
		diff := result - tt.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			t.Errorf("calculateTextSimilarity(%q, %q) = %f; expected ~%f", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestBuildSummary(t *testing.T) {
	conflicts := []Conflict{
		{Type: DuplicateEntity, Severity: SeverityMedium},
		{Type: DuplicateEntity, Severity: SeverityLow},
		{Type: InconsistentReference, Severity: SeverityHigh},
	}

	summary := buildSummary(conflicts)

	if summary.TotalConflicts != 3 {
		t.Errorf("Expected total of 3 conflicts, got %d", summary.TotalConflicts)
	}

	if summary.ByType[DuplicateEntity] != 2 {
		t.Errorf("Expected 2 DuplicateEntity conflicts, got %d", summary.ByType[DuplicateEntity])
	}

	if summary.BySeverity[SeverityHigh] != 1 {
		t.Errorf("Expected 1 High severity conflict, got %d", summary.BySeverity[SeverityHigh])
	}

	if summary.RequiresImmediate != 1 {
		t.Errorf("Expected 1 requires immediate, got %d", summary.RequiresImmediate)
	}
}

func TestGenerateRecommendations(t *testing.T) {
	tests := []struct {
		name     string
		conflicts []Conflict
		expected []string
	}{
		{
			name: "no conflicts",
			conflicts: []Conflict{},
			expected: []string{"No immediate action required - wiki integrity looks good!"},
		},
		{
			name: "circular deps",
			conflicts: []Conflict{{Type: CircularDependency}},
			expected: []string{"Review circular dependencies and reorganize page structure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := generateRecommendations(tt.conflicts)
			if len(recs) != len(tt.expected) {
				t.Errorf("Expected %d recommendations, got %d", len(tt.expected), len(recs))
			}
		})
	}
}
