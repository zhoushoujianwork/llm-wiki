// Package conflicts provides conflict detection for wiki pages.
package conflicts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zhoushoujianwork/llm-wiki/internal/index"
	"github.com/zhoushoujianwork/llm-wiki/internal/llm"
	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

// LLMDetector uses LLM analysis to detect subtle contradictions.
type LLMDetector struct {
	client *llm.AnthropicClient
	store  *wiki.Store
	kg     *index.KnowledgeGraph
}

// NewLLMDetector creates a new LLM-powered conflict detector.
func NewLLMDetector(store *wiki.Store, kg *index.KnowledgeGraph) *LLMDetector {
	return &LLMDetector{
		client: llm.NewAnthropicClient(),
		store:  store,
		kg:     kg,
	}
}

// DetectContradictions uses LLM to identify semantic contradictions between pages.
func (d *LLMDetector) DetectContradictions(ctx context.Context, pagePairs [][2]wiki.Page) ([]Conflict, error) {
	var conflicts []Conflict

	for i, pair := range pagePairs {
		page1, page2 := pair[0], pair[1]

		conflict, err := d.analyzePair(ctx, page1, page2, i)
		if err != nil {
			// Log but continue - one failure shouldn't stop the whole scan
			fmt.Printf("DEBUG: Failed to analyze pair %d: %v\n", i, err)
			continue
		}

		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}

	return conflicts, nil
}

// analyzePair analyzes two pages for contradictions using LLM.
func (d *LLMDetector) analyzePair(ctx context.Context, page1, page2 wiki.Page, index int) (*Conflict, error) {
	// Check if API key is configured
	if d.client == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}

	// Build prompt for LLM
	prompt := d.buildContradictionPrompt(page1, page2, index)

	// Call LLM
	resp, err := d.client.Message(ctx, llm.MessageRequest{
		MaxTokens:   2048,
		Temperature: 0.3, // Lower temperature for more deterministic analysis
		User:        prompt,
	})

	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	conflict := d.parseLLMResponse(resp.Text, page1, page2, index)
	return conflict, nil
}

// buildContradictionPrompt constructs the LLM prompt for contradiction analysis.
func (d *LLMDetector) buildContradictionPrompt(page1, page2 wiki.Page, index int) string {
	prompt := fmt.Sprintf(`You are a wiki quality auditor. Analyze these two wiki pages for potential contradictions or conflicting information.

Page 1: %s/%s
Path: %s
Content:
%s

---

Page 2: %s/%s
Path: %s
Content:
%s

Your task:
1. Identify any factual or logical contradictions between these pages
2. Determine if they describe the same concept differently
3. Flag significant disagreements about facts, definitions, or claims

Respond with JSON in this exact format:
{
  "hasContradiction": boolean,
  "confidence": number (0-1),
  "contradictionType": "factual" | "definitional" | "temporal" | "contextual" | null,
  "description": "brief description of the contradiction",
  "evidence": [
    {
      "page": 1 or 2,
      "quote": "exact text from page",
      "explanation": "why this contradicts"
    }
  ],
  "recommendation": "how to resolve"
}

If no contradiction found, set hasContradiction to false and leave other fields as null or empty arrays.`,
		page1.Namespace, page1.Name, getFullPagePath(page1), truncateText(page1.Content, 3000),
		page2.Namespace, page2.Name, getFullPagePath(page2), truncateText(page2.Content, 3000),
	)

	return prompt
}

// parseLLMResponse parses the LLM's JSON response into a Conflict object.
func (d *LLMDetector) parseLLMResponse(resp string, page1, page2 wiki.Page, index int) *Conflict {
	var result struct {
		HasContradiction bool     `json:"hasContradiction"`
		Confidence       float64  `json:"confidence"`
		ContradictionType *string  `json:"contradictionType"`
		Description      string   `json:"description"`
		Evidence         []struct {
			Page       int    `json:"page"`
			Quote      string `json:"quote"`
			Explanation string `json:"explanation"`
		} `json:"evidence"`
		Recommendation string `json:"recommendation"`
	}

	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		// If JSON parsing fails, try to extract useful info from plain text
		if strings.Contains(strings.ToLower(resp), "contradiction") && 
	           !strings.Contains(strings.ToLower(resp), "no contradiction") {
			// Found indication of contradiction even without proper JSON
			return &Conflict{
				ID:          fmt.Sprintf("llm-contradiction-%d", index),
				Type:        ContradictoryContent,
				Severity:    SeverityMedium,
				Title:       "Potential contradiction detected by LLM",
				Description: "LLM analysis suggests these pages may contain contradictory information",
				Pages: []ConflictPage{
					{Namespace: page1.Namespace, Name: page1.Name, Path: getFullPagePath(page1)},
					{Namespace: page2.Namespace, Name: page2.Name, Path: getFullPagePath(page2)},
				},
				Evidence: []EvidenceItem{
					{Type: "analysis", Description: "LLM detected potential contradiction", Text: resp[:min(len(resp), 500)]},
				},
				CreatedAt:  time.Now(),
				Confidence: 0.5, // Lower confidence when we can't parse structured response
				Resolved:   false,
			}
		}
		return nil
	}

	if !result.HasContradiction {
		return nil
	}

	// Convert evidence
	evidenceItems := make([]EvidenceItem, len(result.Evidence))
	for i, e := range result.Evidence {
		pageNum := "Page 1"
		if e.Page == 2 {
			pageNum = "Page 2"
		}
		evidenceItems[i] = EvidenceItem{
			Type:        "comparison",
			Description: e.Explanation,
			Text:        e.Quote,
			Metadata:    map[string]string{"source_page": pageNum},
		}
	}

	// Determine severity based on contradiction type
	severity := SeverityMedium
	if result.ContradictionType != nil {
		switch *result.ContradictionType {
		case "factual":
			severity = SeverityHigh
		case "definitional":
			severity = SeverityMedium
		case "temporal":
			severity = SeverityLow
		}
	}

	return &Conflict{
		ID:          fmt.Sprintf("llm-contradiction-%d", index),
		Type:        ContradictoryContent,
		Severity:    severity,
		Title:       fmt.Sprintf("Contradiction: %s", result.Description),
		Description: result.Description,
		Pages: []ConflictPage{
			{Namespace: page1.Namespace, Name: page1.Name, Path: getFullPagePath(page1)},
			{Namespace: page2.Namespace, Name: page2.Name, Path: getFullPagePath(page2)},
		},
		Evidence:      evidenceItems,
		CreatedAt:     time.Now(),
		Confidence:    result.Confidence,
		Resolved:      false,
		Resolution:    result.Recommendation,
	}
}

// truncateText truncates text to maxLength while trying to preserve paragraph boundaries.
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	truncated := text[:maxLength]
	lastNewline := strings.LastIndex(truncated, "\n\n")
	if lastNewline > maxLength*8/10 {
		truncated = truncated[:lastNewline]
	}

	return truncated + "... [truncated]"
}

// Detector implements static analysis-based conflict detection.
type Detector struct {
	store *wiki.Store
	kg    *index.KnowledgeGraph
}

// NewDetector creates a new static-analysis conflict detector.
func NewDetector(store *wiki.Store) *Detector {
	return &Detector{store: store, kg: nil}
}

// NewDetectorWithKG creates a detector with an existing knowledge graph.
func NewDetectorWithKG(store *wiki.Store, kg *index.KnowledgeGraph) *Detector {
	return &Detector{store: store, kg: kg}
}

// DetectAll runs all available conflict detection methods.
func (d *Detector) DetectAll(ctx context.Context) (*Report, error) {
	pages, err := d.store.AllPages()
	if err != nil {
		return &Report{
			GeneratedAt:     time.Now(),
			TotalPages:      0,
			Conflicts:       []Conflict{},
			Summary:         Summary{},
			Recommendations: []string{"Unable to scan wiki directory"},
		}, nil
	}
	if len(pages) == 0 {
		return &Report{
			GeneratedAt:     time.Now(),
			TotalPages:      0,
			Conflicts:       []Conflict{},
			Summary:         Summary{},
			Recommendations: []string{},
		}, nil
	}

	allConflicts := make([]Conflict, 0)

	// Run static analysis
	refConflicts, _ := d.detectInconsistentReferences(pages)
	allConflicts = append(allConflicts, refConflicts...)

	dupConflicts, _ := d.detectDuplicateEntities(pages)
	allConflicts = append(allConflicts, dupConflicts...)

	circConflicts, _ := d.detectCircularDependencies(pages)
	allConflicts = append(allConflicts, circConflicts...)

	// Run LLM-powered contradiction detection if knowledge graph is available
	if d.kg != nil {
		llmConflicts, err := d.detectSemanticContradictions(ctx, pages)
		if err != nil {
			fmt.Printf("Warning: LLM contradiction detection failed: %v\n", err)
		} else {
			allConflicts = append(allConflicts, llmConflicts...)
		}
	}

	report := &Report{
		GeneratedAt:     time.Now(),
		TotalPages:      len(pages),
		Conflicts:       allConflicts,
		Summary:         buildSummary(allConflicts),
		Recommendations: generateRecommendations(allConflicts),
	}

	return report, nil
}

// detectSemanticContradictions uses LLM to find semantic contradictions.
func (d *Detector) detectSemanticContradictions(ctx context.Context, pages []wiki.Page) ([]Conflict, error) {
	// Generate page pairs for comparison
	pairs := d.kg.GeneratePairs(index.ByLinkStructure)
	
	// Also add pairs by common concepts
	conceptPairs := d.kg.GeneratePairs(index.ByCommonConcept)
	pairs = append(pairs, conceptPairs...)

	// Convert to format expected by LLMDetector
	pagePairs := make([][2]wiki.Page, len(pairs))
	for i, pair := range pairs {
		pagePairs[i] = [2]wiki.Page{pair.Page1, pair.Page2}
	}

	// Use LLM detector
	llmDetector := NewLLMDetector(d.store, d.kg)
	conflicts, err := llmDetector.DetectContradictions(ctx, pagePairs)
	
	return conflicts, err
}

// detectInconsistentReferences finds broken or inconsistent page links.
func (d *Detector) detectInconsistentReferences(pages []wiki.Page) ([]Conflict, error) {
	var conflicts []Conflict
	pageMap := make(map[string]bool)

	for _, page := range pages {
		key := ""
		if page.Namespace != "" {
			key = fmt.Sprintf("%s/%s", page.Namespace, page.Name)
		} else {
			key = page.Name
		}
		pageMap[key] = true
	}

	for _, page := range pages {
		for _, link := range page.Links {
			target := parseWikiLink(link)
			if target != "" {
				keyWithNS := ""
				found := false
				if page.Namespace != "" {
					keyWithNS = fmt.Sprintf("%s/%s", page.Namespace, target)
					if pageMap[keyWithNS] {
						found = true
					}
				}
				if !found && !pageMap[target] {
					conflict := Conflict{
						ID:          fmt.Sprintf("broken-ref-%s-%s", page.Namespace, page.Name),
						Type:        InconsistentReference,
						Severity:    SeverityMedium,
						Title:       fmt.Sprintf("Broken link from %s/%s", page.Namespace, page.Name),
						Description: fmt.Sprintf("Page references non-existent page: %s", target),
						Pages: []ConflictPage{
							{Namespace: page.Namespace, Name: page.Name, Path: fmt.Sprintf("%s/%s.md", page.Namespace, page.Name)},
						},
						Evidence: []EvidenceItem{
							{Type: "comparison", Description: "Non-existent reference", Text: link},
						},
						CreatedAt:  time.Now(),
						Confidence: 0.95,
						Resolved:   false,
					}
					conflicts = append(conflicts, conflict)
				}
			}
		}
	}

	return conflicts, nil
}

// detectDuplicateEntities finds pages that might be duplicate entries.
func (d *Detector) detectDuplicateEntities(pages []wiki.Page) ([]Conflict, error) {
	var conflicts []Conflict

	similarPairs := calculateIntraGroupSimilarity(pages)

	const similarityThreshold = 0.8
	for _, pair := range similarPairs {
		if pair.similarity > similarityThreshold {
			conflict := Conflict{
				ID:          fmt.Sprintf("dup-entity-%s-%s", pair.page1.Namespace, pair.page2.Namespace),
				Type:        DuplicateEntity,
				Severity:    SeverityMedium,
				Title:       "Potential duplicate entities",
				Description: fmt.Sprintf("Pages %s/%s and %s/%s have high content similarity (%.0f%%)",
					pair.page1.Namespace, pair.page1.Name,
					pair.page2.Namespace, pair.page2.Name,
					pair.similarity*100),
				Pages: []ConflictPage{
					{Namespace: pair.page1.Namespace, Name: pair.page1.Name, Path: fmt.Sprintf("%s/%s.md", pair.page1.Namespace, pair.page1.Name)},
					{Namespace: pair.page2.Namespace, Name: pair.page2.Name, Path: fmt.Sprintf("%s/%s.md", pair.page2.Namespace, pair.page2.Name)},
				},
				Evidence: []EvidenceItem{
					{Type: "analysis", Description: "High similarity score", Metadata: map[string]string{"similarity": fmt.Sprintf("%.2f", pair.similarity)}},
				},
				CreatedAt:  time.Now(),
				Confidence: 0.7,
				Resolved:   false,
			}
			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts, nil
}

// detectCircularDependencies finds circular reference chains.
func (d *Detector) detectCircularDependencies(pages []wiki.Page) ([]Conflict, error) {
	var conflicts []Conflict
	pageMap := make(map[string]wiki.Page)
	graph := make(map[string][]string)

	for _, page := range pages {
		key := fmt.Sprintf("%s/%s", page.Namespace, page.Name)
		pageMap[key] = page
		graph[key] = []string{}
	}

	for _, page := range pages {
		key := fmt.Sprintf("%s/%s", page.Namespace, page.Name)
		for _, link := range page.Links {
			target := parseWikiLink(link)
			if target != "" {
				graph[key] = append(graph[key], target)
			}
		}
	}

	found := make(map[string]bool)
	for start := range graph {
		if found[start] {
			continue
		}

		cyclePath := []string{start}
		visited := make(map[string]bool)
		visited[start] = true

		var dfs func(current string, path []string) bool
		dfs = func(current string, path []string) bool {
			for _, neighbor := range graph[current] {
				if visited[neighbor] {
					cycleStart := findCycleStart(path, neighbor)
					if cycleStart >= 0 {
						path = path[cycleStart:]
						pathStr := strings.Join(path, " -> ") + " -> " + neighbor
						conflict := Conflict{
							ID:          fmt.Sprintf("circular-dep-%s", hashString(pathStr)),
							Type:        CircularDependency,
							Severity:    SeverityLow,
							Title:       "Circular dependency detected",
							Description: fmt.Sprintf("Page chain forms a circle: %s", pathStr),
							Pages:       extractPagesFromPath(path),
							Evidence: []EvidenceItem{
								{Type: "analysis", Description: "Circular reference chain", Text: pathStr},
							},
							CreatedAt:  time.Now(),
							Confidence: 0.9,
							Resolved:   false,
						}
						conflicts = append(conflicts, conflict)
						return true
					}
				} else if graph[neighbor] != nil {
					visited[neighbor] = true
					path = append(path, neighbor)
					if dfs(neighbor, path) {
						return true
					}
					path = path[:len(path)-1]
				}
			}
			return false
		}

		if dfs(start, cyclePath) {
			found[start] = true
		}
	}

	return conflicts, nil
}

// Helper functions

func parseWikiLink(link string) string {
	link = strings.TrimSpace(link)
	// Handle [[wiki-link]] format
	if strings.HasPrefix(link, "[[") && strings.HasSuffix(link, "]]") {
		content := link[2 : len(link)-2]
		if idx := strings.Index(content, "#"); idx >= 0 {
			content = content[:idx]
		}
		return content
	}
	// Also accept plain wiki links (non-bracketed)
	if len(link) > 0 && !strings.ContainsAny(link, "[]()") {
		return link
	}
	return ""
}

func calculateIntraGroupSimilarity(pages []wiki.Page) []SimilarityPair {
	var pairs []SimilarityPair
	for i := 0; i < len(pages); i++ {
		for j := i + 1; j < len(pages); j++ {
			sim := calculateTextSimilarity(pages[i].Content, pages[j].Content)
			pairs = append(pairs, SimilarityPair{
				page1:      pages[i],
				page2:      pages[j],
				similarity: sim,
			})
		}
	}
	return pairs
}

func calculateTextSimilarity(a, b string) float64 {
	wordsA := wordCount(a)
	wordsB := wordCount(b)

	if wordsA == 0 || wordsB == 0 {
		return 0
	}

	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, w := range splitWords(a) {
		setA[strings.ToLower(w)] = true
	}

	for _, w := range splitWords(b) {
		setB[strings.ToLower(w)] = true
	}

	intersection := 0
	for k := range setA {
		if setB[k] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection

	return float64(intersection) / float64(union)
}

func wordCount(text string) int {
	count := 0
	inWord := false
	for _, c := range text {
		if c == ' ' || c == '\n' {
			if inWord {
				count++
				inWord = false
			}
		} else {
			inWord = true
		}
	}
	if inWord {
		count++
	}
	return count
}

func splitWords(text string) []string {
	var words []string
	current := strings.Builder{}
	for _, c := range text {
		if c == ' ' || c == '\n' || c == '\t' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func findCycleStart(path []string, target string) int {
	for i, node := range path {
		if node == target {
			return i
		}
	}
	return -1
}

func extractPagesFromPath(path []string) []ConflictPage {
	var pages []ConflictPage
	for _, node := range path {
		parts := strings.Split(node, "/")
		if len(parts) == 2 {
			pages = append(pages, ConflictPage{
				Namespace: parts[0],
				Name:      parts[1],
				Path:      fmt.Sprintf("%s/%s.md", parts[0], parts[1]),
			})
		}
	}
	return pages
}

func hashString(s string) string {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("%x", h&0xFFFFFFFF)
}

// groupByNamespaceAndSimilarity groups pages for analysis.
func groupByNamespaceAndSimilarity(pages []wiki.Page) [][]wiki.Page {
	groups := make(map[string][]wiki.Page)
	for _, page := range pages {
		key := fmt.Sprintf("%s_%s", page.Namespace, strings.ToLower(page.Name[:min(20, len(page.Name))]))
		groups[key] = append(groups[key], page)
	}
	var result [][]wiki.Page
	for _, group := range groups {
		result = append(result, group)
	}
	return result
}

// SimilarityPair represents a pair of pages with calculated similarity.
type SimilarityPair struct {
	page1      wiki.Page
	page2      wiki.Page
	similarity float64
}

// buildSummary creates summary statistics from conflicts.
func buildSummary(conflicts []Conflict) Summary {
	summary := Summary{
		TotalConflicts:  len(conflicts),
		ByType:          make(map[ConflictType]int),
		BySeverity:      make(map[SeverityLevel]int),
		HighestPriority: "",
	}

	for _, c := range conflicts {
		summary.ByType[c.Type]++
		summary.BySeverity[c.Severity]++

		if c.Severity == SeverityHigh {
			summary.HighestPriority = c.Type
			summary.RequiresImmediate++
		}
	}

	return summary
}

// generateRecommendations provides actionable recommendations based on conflicts.
func generateRecommendations(conflicts []Conflict) []string {
	var recs []string

	byType := make(map[ConflictType]int)
	for _, c := range conflicts {
		byType[c.Type]++
	}

	if byType[CircularDependency] > 0 {
		recs = append(recs, "Review circular dependencies and reorganize page structure")
	}
	if byType[InconsistentReference] > 0 {
		recs = append(recs, "Fix broken references to non-existent pages")
	}
	if byType[DuplicateEntity] > 0 {
		recs = append(recs, "Consider merging or distinguishing duplicate pages")
	}
	if byType[ContradictoryContent] > 0 {
		recs = append(recs, "Investigate and resolve contradictory information between pages")
	}

	if len(recs) == 0 {
		recs = append(recs, "No immediate action required - wiki integrity looks good!")
	}

	return recs
}

// Helper functions for conflict detection

func getFullPagePath(page wiki.Page) string {
	if page.Namespace != "" {
		return fmt.Sprintf("%s/%s.md", page.Namespace, page.Name)
	}
	return fmt.Sprintf("%s.md", page.Name)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
