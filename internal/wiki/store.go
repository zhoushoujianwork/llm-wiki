package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Page represents a wiki page
type Page struct {
	Namespace string   `json:"namespace"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Content  string   `json:"content"`
	Links    []string `json:"links"`
}

// Index maps concepts/entities to page paths
type Index struct {
	Entries map[string][]string `json:"entries"` // concept → page paths
}

// Store handles wiki page persistence
type Store struct {
	rootDir string
	index   *Index
}

// NewStore creates a new wiki store
func NewStore(rootDir string) *Store {
	return &Store{
		rootDir: rootDir,
		index: &Index{
			Entries: make(map[string][]string),
		},
	}
}

// WritePage writes a page to the wiki
func (s *Store) WritePage(namespace string, page Page) error {
	nsDir := filepath.Join(s.rootDir, namespace)
	if err := os.MkdirAll(nsDir, 0755); err != nil {
		return fmt.Errorf("failed to create namespace dir: %w", err)
	}

	filename := fmt.Sprintf("%s.md", page.Name)
	// Ensure unique filename
	filename = s.uniqueFilename(nsDir, filename)
	path := filepath.Join(nsDir, filename)

	if err := os.WriteFile(path, []byte(page.Content), 0644); err != nil {
		return fmt.Errorf("failed to write page: %w", err)
	}

	// Update index
	s.index.Entries[page.Name] = append(s.index.Entries[page.Name], path)

	return nil
}

// NeedsCompilation checks if a document needs compilation
func (s *Store) NeedsCompilation(sourceName, docPath string) bool {
	// TODO: Check mtime comparison
	// For now, always compile
	return true
}

// RebuildIndex rebuilds the concept → page index
func (s *Store) RebuildIndex() error {
	s.index.Entries = make(map[string][]string)

	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Extract links from content
		links := extractLinks(string(content))
		for _, link := range links {
			s.index.Entries[link] = append(s.index.Entries[link], path)
		}

		return nil
	})

	return err
}

// FindRelevantPages finds pages relevant to a query
func (s *Store) FindRelevantPages(query string) ([]Page, error) {
	// Simple implementation: search all pages for query terms
	queryLower := strings.ToLower(query)
	var results []Page

	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if strings.Contains(strings.ToLower(string(content)), queryLower) {
			rel, _ := filepath.Rel(s.rootDir, filepath.Dir(path))
			results = append(results, Page{
				Namespace: rel,
				Name:      strings.TrimSuffix(info.Name(), ".md"),
				Content:   string(content),
			})
		}

		return nil
	})

	return results, err
}

func (s *Store) uniqueFilename(dir, filename string) string {
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return filename
	}

	base := strings.TrimSuffix(filename, ".md")
	ext := ".md"
	i := 1
	for {
		path = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return filepath.Base(path)
		}
		i++
	}
}

func extractLinks(content string) []string {
	// Simple wiki-style link extraction: [[Page Name]]
	var links []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			link := strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]")
			links = append(links, link)
		}
	}
	return links
}
