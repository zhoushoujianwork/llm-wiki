// Package index provides knowledge indexing utilities.
package index

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"llm-wiki/internal/wiki"
)

// KnowledgeGraph maintains a mapping from concepts/entities to wiki pages with metadata.
type KnowledgeGraph struct {
	// concept -> page paths with confidence scores
	entries map[string][]KnowledgeEntry
	// page path -> associated concepts
	invertedIndex map[string][]string
	// page path -> page content hash (for change detection)
	contentHashes map[string]string
	// cached page data
	pageCache map[string]wiki.Page
	mu        sync.RWMutex
}

// KnowledgeEntry represents a concept-page association.
type KnowledgeEntry struct {
	Path       string  `json:"path"`
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	Confidence float64 `json:"confidence"` // How relevant this page is to the concept
	Snippet    string  `json:"snippet,omitempty"`
}

// NewKnowledgeGraph creates a new empty knowledge graph.
func NewKnowledgeGraph() *KnowledgeGraph {
	return &KnowledgeGraph{
		entries:       make(map[string][]KnowledgeEntry),
		invertedIndex: make(map[string][]string),
		contentHashes: make(map[string]string),
		pageCache:     make(map[string]wiki.Page),
	}
}

// Build constructs the knowledge graph from wiki pages.
func (kg *KnowledgeGraph) Build(ctx context.Context, store *wiki.Store) error {
	pages, err := store.AllPages()
	if err != nil {
		return fmt.Errorf("failed to load pages: %w", err)
	}

	kg.mu.Lock()
	defer kg.mu.Unlock()

	for _, page := range pages {
		kg.indexPage(page)
	}

	return nil
}

// indexPage extracts knowledge points and adds them to the graph.
func (kg *KnowledgeGraph) indexPage(page wiki.Page) {
	key := fmt.Sprintf("%s/%s", page.Namespace, page.Name)
	
	// Store page info in cache
	kg.pageCache[key] = page
	
	// Compute content hash for change detection
	hash := computeHash(page.Content)
	kg.contentHashes[key] = hash

	// Extract knowledge points from page title
	titleKey := strings.ToLower(strings.TrimSpace(page.Name))
	if titleKey != "" {
		entry := KnowledgeEntry{
			Path:       key,
			Name:       page.Name,
			Namespace:  page.Namespace,
			Confidence: 1.0,
			Snippet:    extractSnippet(page.Content, 150),
		}
		kg.entries[titleKey] = append(kg.entries[titleKey], entry)
		kg.invertedIndex[key] = append(kg.invertedIndex[key], titleKey)
	}

	// Extract knowledge points from headings and first paragraph
	extracted := extractKnowledgePoints(page.Content)
	for _, point := range extracted {
		pointKey := strings.ToLower(strings.TrimSpace(point))
		if pointKey != "" && pointKey != titleKey {
			confidence := calculateRelevanceScore(point, page.Content)
			entry := KnowledgeEntry{
				Path:       key,
				Name:       page.Name,
				Namespace:  page.Namespace,
				Confidence: confidence,
				Snippet:    extractSnippet(page.Content, 150),
			}
			
			// Avoid duplicates
			exists := false
			for _, e := range kg.entries[pointKey] {
				if e.Path == key {
					exists = true
					break
				}
			}
			if !exists {
				kg.entries[pointKey] = append(kg.entries[pointKey], entry)
				kg.invertedIndex[key] = append(kg.invertedIndex[key], pointKey)
			}
		}
	}

	// Index page links as relationships
	for _, link := range page.Links {
		linkLower := strings.ToLower(strings.TrimSpace(link))
		if linkLower != "" {
			// This is a cross-reference relationship
			entry := KnowledgeEntry{
				Path:       key,
				Name:       page.Name,
				Namespace:  page.Namespace,
				Confidence: 0.7,
				Snippet:    "",
			}
			kg.entries[linkLower] = append(kg.entries[linkLower], entry)
		}
	}
}

// GetRelatedPages returns pages related to a concept.
func (kg *KnowledgeGraph) GetRelatedPages(concept string) []KnowledgeEntry {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	conceptLower := strings.ToLower(concept)
	entries := kg.entries[conceptLower]
	
	// Sort by confidence
	sortByConfidence(entries)
	
	return entries
}

// GetAllConcepts returns all indexed concepts.
func (kg *KnowledgeGraph) GetAllConcepts() []string {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	concepts := make([]string, 0, len(kg.entries))
	for k := range kg.entries {
		concepts = append(concepts, k)
	}
	return concepts
}

// GetPageContent retrieves page content from cache.
func (kg *KnowledgeGraph) GetPageContent(path string) (wiki.Page, bool) {
	kg.mu.RLock()
	defer kg.mu.RUnlock()
	
	page, ok := kg.pageCache[path]
	return page, ok
}

// HasChanged checks if a page has been modified since last build.
func (kg *KnowledgeGraph) HasChanged(path string, currentContent string) bool {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	currentHash := computeHash(currentContent)
	oldHash, exists := kg.contentHashes[path]
	return !exists || oldHash != currentHash
}

// PairingStrategy defines how to pair pages for comparison.
type PairingStrategy int

const (
	// BySimilarity pairs pages with similar content
	BySimilarity PairingStrategy = iota
	// ByLinkStructure pairs pages that reference each other
	ByLinkStructure
	// ByCommonConcept pairs pages sharing concepts
	ByCommonConcept
)

// PagePair represents two pages to compare.
type PagePair struct {
	Page1 wiki.Page
	Page2 wiki.Page
	Reason string // Why these were paired
}

// GeneratePairs generates page pairs based on strategy.
func (kg *KnowledgeGraph) GeneratePairs(strategy PairingStrategy) []PagePair {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	var pairs []PagePair
	
	switch strategy {
	case ByLinkStructure:
		pairs = kg.pairByLinks()
	case ByCommonConcept:
		pairs = kg.pairByConcepts()
	default:
		pairs = kg.pairBySimilarity()
	}

	return pairs
}

// pairByLinks pairs pages that have mutual or overlapping links.
func (kg *KnowledgeGraph) pairByLinks() []PagePair {
	var pairs []PagePair
	pageList := make([]wiki.Page, 0, len(kg.pageCache))
	
	for _, page := range kg.pageCache {
		pageList = append(pageList, page)
	}

	for i := 0; i < len(pageList); i++ {
		for j := i + 1; j < len(pageList); j++ {
			page1, page2 := pageList[i], pageList[j]
			
			// Check for mutual links
			hasLink1 := hasLinkTo(page1.Links, page2.Name)
			hasLink2 := hasLinkTo(page2.Links, page1.Name)
			
			// Check for overlapping linked pages
			overlapCount := countOverlap(page1.Links, page2.Links)
			
			if hasLink1 || hasLink2 || overlapCount > 2 {
				reason := "linked_pages"
				if hasLink1 && hasLink2 {
					reason = "mutual_links"
				} else if overlapCount > 2 {
					reason = fmt.Sprintf("shared_links(%d)", overlapCount)
				}
				pairs = append(pairs, PagePair{
					Page1:  page1,
					Page2:  page2,
					Reason: reason,
				})
			}
		}
	}

	return pairs
}

// pairByConcepts pairs pages that share significant concepts.
func (kg *KnowledgeGraph) pairByConcepts() []PagePair {
	var pairs []PagePair
	
	// Find concepts with multiple pages
	for concept, entries := range kg.entries {
		if len(entries) >= 2 {
			// Take top 3 most relevant pages per concept
			topEntries := entries
			if len(topEntries) > 3 {
				topEntries = topEntries[:3]
			}
			
			// Create all pairs
			for i := 0; i < len(topEntries); i++ {
				for j := i + 1; j < len(topEntries); j++ {
					page1Path := topEntries[i].Path
					page2Path := topEntries[j].Path
					
					page1, ok1 := kg.pageCache[page1Path]
					page2, ok2 := kg.pageCache[page2Path]
					
					if ok1 && ok2 {
						pairs = append(pairs, PagePair{
							Page1:  page1,
							Page2:  page2,
							Reason: fmt.Sprintf("shared_concept:%s", concept),
						})
					}
				}
			}
		}
	}

	// Deduplicate pairs
	seen := make(map[string]bool)
	var uniquePairs []PagePair
	for _, pair := range pairs {
		key := fmt.Sprintf("%s|%s", pair.Page1.Namespace+"/"+pair.Page1.Name, pair.Page2.Namespace+"/"+pair.Page2.Name)
		if !seen[key] {
			seen[key] = true
			uniquePairs = append(uniquePairs, pair)
		}
	}

	return uniquePairs
}

// pairBySimilarity pairs pages with similar content (using simple word overlap).
func (kg *KnowledgeGraph) pairBySimilarity() []PagePair {
	var pairs []PagePair
	pageList := make([]wiki.Page, 0, len(kg.pageCache))
	
	for _, page := range kg.pageCache {
		pageList = append(pageList, page)
	}

	for i := 0; i < len(pageList); i++ {
		for j := i + 1; j < len(pageList); j++ {
			page1, page2 := pageList[i], pageList[j]
			
			similarity := calculateJaccardSimilarity(page1.Content, page2.Content)
			
			if similarity > 0.3 { // Threshold for potential conflict
				pairs = append(pairs, PagePair{
					Page1:  page1,
					Page2:  page2,
					Reason: fmt.Sprintf("similar_content(%.0f%%)", similarity*100),
				})
			}
		}
	}

	return pairs
}

// save persists the knowledge graph to disk.
func (kg *KnowledgeGraph) Save(path string) error {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	data, err := json.MarshalIndent(kg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal knowledge graph: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// Load loads a persisted knowledge graph from disk.
func (kg *KnowledgeGraph) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read knowledge graph file: %w", err)
	}

	return json.Unmarshal(data, kg)
}

// Helper functions

func extractSnippet(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	var snippet strings.Builder
	
	for i, line := range lines {
		if i >= 3 {
			break
		}
		line = strings.TrimSpace(line)
		if line != "" {
			if snippet.Len() > 0 {
				snippet.WriteString(" ")
			}
			if len(snippet.String())+len(line) > maxLen {
				snippet.WriteString(line[:maxLen-snippet.Len()-3])
				snippet.WriteString("...")
				break
			}
			snippet.WriteString(line)
		}
	}
	
	return snippet.String()
}

func extractKnowledgePoints(content string) []string {
	var points []string
	
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Skip empty lines and markdown headers
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Take short phrases (< 60 chars) as potential knowledge points
		if len(line) < 60 && len(line) > 5 {
			points = append(points, line)
		}
	}
	
	return points
}

func calculateRelevanceScore(term, content string) float64 {
	termLower := strings.ToLower(term)
	contentLower := strings.ToLower(content)
	
	// Exact match gets highest score
	if strings.Contains(contentLower, termLower) {
		return 0.9
	}
	
	// Word boundary match
	if strings.Contains(" "+contentLower+" ", " "+termLower+" ") {
		return 0.7
	}
	
	// Prefix/suffix match
	if strings.HasPrefix(contentLower, termLower) || strings.HasSuffix(contentLower, termLower) {
		return 0.5
	}
	
	return 0.3
}

func calculateJaccardSimilarity(a, b string) float64 {
	setA := getWordSet(a)
	setB := getWordSet(b)
	
	if len(setA) == 0 || len(setB) == 0 {
		return 0
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

func getWordSet(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	set := make(map[string]bool)
	for _, w := range words {
		// Clean punctuation
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if len(w) > 2 {
			set[w] = true
		}
	}
	return set
}

func hasLink(links []string, targetName string) bool {
	targetLower := strings.ToLower(targetName)
	for _, link := range links {
		linkClean := strings.ToLower(strings.TrimSpace(link))
		// Strip wiki link syntax
		if strings.HasPrefix(linkClean, "[[") && strings.HasSuffix(linkClean, "]]") {
			linkClean = linkClean[2 : len(linkClean)-2]
		}
		// Take just the name part (before #)
		if idx := strings.Index(linkClean, "#"); idx >= 0 {
			linkClean = linkClean[:idx]
		}
		if linkClean == targetLower {
			return true
		}
	}
	return false
}

// hasLinkTo is an alias for hasLink.
func hasLinkTo(links []string, targetName string) bool {
	return hasLink(links, targetName)
}

func countOverlap(links1, links2 []string) int {
	set1 := make(map[string]bool)
	for _, l := range links1 {
		l = strings.ToLower(strings.TrimSpace(l))
		if strings.HasPrefix(l, "[[") && strings.HasSuffix(l, "]]") {
			l = l[2 : len(l)-2]
		}
		if idx := strings.Index(l, "#"); idx >= 0 {
			l = l[:idx]
		}
		set1[l] = true
	}
	
	count := 0
	for _, l := range links2 {
		l = strings.ToLower(strings.TrimSpace(l))
		if strings.HasPrefix(l, "[[") && strings.HasSuffix(l, "]]") {
			l = l[2 : len(l)-2]
		}
		if idx := strings.Index(l, "#"); idx >= 0 {
			l = l[:idx]
		}
		if set1[l] {
			count++
		}
	}
	return count
}

func computeHash(content string) string {
	h := 0
	for _, c := range content {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("%x", h&0xFFFFFFFF)
}

func sortByConfidence(entries []KnowledgeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Confidence > entries[j].Confidence
	})
}

// BuildKnowledgeGraph creates a new knowledge graph and builds it from wiki store.
func BuildKnowledgeGraph(ctx context.Context, store *wiki.Store, indexPath string) (*KnowledgeGraph, error) {
	kg := NewKnowledgeGraph()
	
	if err := kg.Build(ctx, store); err != nil {
		return nil, err
	}
	
	if indexPath != "" {
		if err := kg.Save(indexPath); err != nil {
			return nil, fmt.Errorf("failed to save knowledge graph: %w", err)
		}
	}
	
	return kg, nil
}

// LoadOrCreateKnowledgeGraph loads from disk if exists, otherwise builds fresh.
func LoadOrCreateKnowledgeGraph(ctx context.Context, store *wiki.Store, indexPath string) (*KnowledgeGraph, error) {
	if indexPath != "" {
		if _, err := os.Stat(indexPath); err == nil {
			kg := NewKnowledgeGraph()
			if err := kg.Load(indexPath); err == nil {
				return kg, nil
			}
		}
	}
	
	return BuildKnowledgeGraph(ctx, store, indexPath)
}
