// Package conflicts provides conflict detection for wiki pages.
package conflicts

import (
	"time"
)

// ConflictType represents the kind of conflict detected.
type ConflictType string

const (
	// ContradictoryContent - Two pages make contradictory claims about the same topic
	ContradictoryContent ConflictType = "contradictory_content"

	// DuplicateEntity - Same entity described differently across pages
	DuplicateEntity ConflictType = "duplicate_entity"

	// InconsistentReference - Page references another page that doesn't exist
	InconsistentReference ConflictType = "inconsistent_reference"

	// CircularDependency - Pages form a circular reference chain
	CircularDependency ConflictType = "circular_dependency"

	// OutdatedInformation - Page content appears outdated based on compilation date
	OutdatedInformation ConflictType = "outdated_information"
)

// SeverityLevel indicates how critical a conflict is.
type SeverityLevel string

const (
	SeverityLow    SeverityLevel = "low"
	SeverityMedium SeverityLevel = "medium"
	SeverityHigh   SeverityLevel = "high"
)

// Conflict represents a detected issue in the wiki.
type Conflict struct {
	ID            string          `json:"id"`
	Type          ConflictType    `json:"type"`
	Severity      SeverityLevel   `json:"severity"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Pages         []ConflictPage  `json:"pages"`
	Evidence      []EvidenceItem  `json:"evidence"`
	CreatedAt     time.Time       `json:"created_at"`
	Confidence    float64         `json:"confidence"` // 0.0 to 1.0
	Resolved      bool            `json:"resolved"`
	Resolution    string          `json:"resolution,omitempty"`
}

// ConflictPage identifies a specific page involved in a conflict.
type ConflictPage struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Snippet   string `json:"snippet,omitempty"`
}

// EvidenceItem contains supporting evidence for a conflict.
type EvidenceItem struct {
	Type        string            `json:"type"` // "quote", "comparison", "analysis"
	Description string            `json:"description"`
	Text        string            `json:"text,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Report represents a complete conflict detection report.
type Report struct {
	GeneratedAt     time.Time  `json:"generated_at"`
	TotalPages      int        `json:"total_pages"`
	Conflicts       []Conflict `json:"conflicts"`
	Summary         Summary    `json:"summary"`
	Recommendations []string   `json:"recommendations"`
}

// Summary provides high-level statistics about the report.
type Summary struct {
	TotalConflicts  int                  `json:"total_conflicts"`
	ByType          map[ConflictType]int `json:"by_type"`
	BySeverity      map[SeverityLevel]int `json:"by_severity"`
	HighestPriority ConflictType         `json:"highest_priority"`
	RequiresImmediate int               `json:"requires_immediate"` // High severity count
}
