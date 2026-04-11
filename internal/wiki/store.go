package wiki

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"llm-wiki/internal/source"
)

const compiledStateFilename = ".compiled.json"

// safeWalkSkip returns filepath.SkipDir if the directory should be skipped.
func safeWalkSkip(path string, info os.FileInfo, err error) (bool, error) {
	if err != nil {
		// Skip permission denied or non-existent paths
		return false, nil
	}
	if info.IsDir() {
		// Skip hidden directories (starting with .)
		parts := strings.Split(path, string(filepath.Separator))
		if len(parts) > 0 && strings.HasPrefix(parts[len(parts)-1], ".") {
			return false, filepath.SkipDir
		}
	}
	return false, nil
}

// Page represents a wiki page.
type Page struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Content   string   `json:"content"`
	Links     []string `json:"links"`
	Tags      []string `json:"tags,omitempty"`
}

// Index maps concepts and entities to page paths.
type Index struct {
	Entries map[string][]string `json:"entries"`
}

type compiledState struct {
	Documents map[string]compiledDocument `json:"documents"`
}

type compiledDocument struct {
	Checksum   string    `json:"checksum"`
	Pages      []string  `json:"pages"`
	CompiledAt time.Time `json:"compiled_at"`
}

// Store handles wiki page persistence.
type Store struct {
	rootDir       string
	index         *Index
	compiledFile  string
	compiledState compiledState
}

// NewStore creates a new wiki store.
func NewStore(rootDir string) *Store {
	store := &Store{
		rootDir:      rootDir,
		compiledFile: filepath.Join(rootDir, compiledStateFilename),
		index: &Index{
			Entries: make(map[string][]string),
		},
		compiledState: compiledState{
			Documents: make(map[string]compiledDocument),
		},
	}
	_ = store.loadCompiledState()
	return store
}

// WritePage writes a page to the wiki and returns the resulting path.
func (s *Store) WritePage(namespace string, page Page) (string, error) {
	nsDir := filepath.Join(s.rootDir, namespace)
	if err := os.MkdirAll(nsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create namespace dir: %w", err)
	}

	filename := s.uniqueFilename(nsDir, fmt.Sprintf("%s.md", page.Name))
	path := filepath.Join(nsDir, filename)

	if err := os.WriteFile(path, []byte(page.Content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write page: %w", err)
	}

	s.index.Entries[page.Name] = append(s.index.Entries[page.Name], path)
	return path, nil
}

// StoreDocumentPages replaces the generated pages for a document and persists its checksum.
func (s *Store) StoreDocumentPages(namespace string, doc source.Document, pages []Page) error {
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return fmt.Errorf("failed to create wiki root: %w", err)
	}

	key := s.documentKey(namespace, doc.RelPath)
	if previous, ok := s.compiledState.Documents[key]; ok {
		for _, pagePath := range previous.Pages {
			_ = os.Remove(filepath.Join(s.rootDir, pagePath))
		}
	}

	writtenPages := make([]string, 0, len(pages))
	for _, page := range pages {
		pagePath, err := s.WritePage(namespace, page)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(s.rootDir, pagePath)
		if err != nil {
			return fmt.Errorf("failed to compute page path: %w", err)
		}
		writtenPages = append(writtenPages, filepath.ToSlash(relPath))
	}

	s.compiledState.Documents[key] = compiledDocument{
		Checksum:   doc.Checksum,
		Pages:      writtenPages,
		CompiledAt: time.Now().UTC(),
	}

	return s.saveCompiledState()
}

// NeedsCompilation reports whether a document should be recompiled.
func (s *Store) NeedsCompilation(namespace string, doc source.Document) bool {
	if strings.TrimSpace(doc.Checksum) == "" {
		return true
	}

	entry, ok := s.compiledState.Documents[s.documentKey(namespace, doc.RelPath)]
	if !ok {
		return true
	}

	return entry.Checksum != doc.Checksum
}

// RebuildIndex rebuilds the concept → page index.
func (s *Store) RebuildIndex() error {
	s.index.Entries = make(map[string][]string)

	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if skip, err := safeWalkSkip(path, info, err); skip {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if err != nil {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		pageName := strings.TrimSuffix(info.Name(), ".md")
		s.index.Entries[pageName] = append(s.index.Entries[pageName], path)

		for _, link := range extractLinks(string(content)) {
			s.index.Entries[link] = append(s.index.Entries[link], path)
		}

		return nil
	})

	return err
}

// FindRelevantPages finds pages relevant to a query.
func (s *Store) FindRelevantPages(query string) ([]Page, error) {
	queryLower := strings.ToLower(query)
	var results []Page

	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if skip, err := safeWalkSkip(path, info, err); skip {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if err != nil {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if strings.Contains(strings.ToLower(string(content)), queryLower) {
			namespace, _ := filepath.Rel(s.rootDir, filepath.Dir(path))
			results = append(results, Page{
				Namespace: filepath.ToSlash(namespace),
				Name:      strings.TrimSuffix(info.Name(), ".md"),
				Content:   string(content),
			})
		}

		return nil
	})

	return results, err
}

// AllPages loads every wiki page in storage.
func (s *Store) AllPages() ([]Page, error) {
	var pages []Page

	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if skip, err := safeWalkSkip(path, info, err); skip {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if err != nil {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		namespace, _ := filepath.Rel(s.rootDir, filepath.Dir(path))
		pages = append(pages, Page{
			Namespace: filepath.ToSlash(namespace),
			Name:      strings.TrimSuffix(info.Name(), ".md"),
			Content:   string(content),
			Links:     extractLinks(string(content)),
		})
		return nil
	})

	return pages, err
}

// ListPages returns a list of all wiki page paths.
func (s *Store) ListPages() ([]string, error) {
	var pages []string
	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if skip, err := safeWalkSkip(path, info, err); skip {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		pages = append(pages, path)
		return nil
	})
	return pages, err
}

// ReadPage reads the content of a wiki page.
func (s *Store) ReadPage(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetEntities returns the concept → page index.
func (s *Store) GetEntities() map[string][]string {
	return s.index.Entries
}

func (s *Store) documentKey(namespace, relPath string) string {
	return namespace + "::" + filepath.ToSlash(relPath)
}

func (s *Store) loadCompiledState() error {
	data, err := os.ReadFile(s.compiledFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state compiledState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Documents == nil {
		state.Documents = make(map[string]compiledDocument)
	}

	s.compiledState = state
	return nil
}

func (s *Store) saveCompiledState() error {
	data, err := json.MarshalIndent(s.compiledState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode compiled state: %w", err)
	}
	if err := os.WriteFile(s.compiledFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write compiled state: %w", err)
	}
	return nil
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
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return filepath.Base(candidate)
		}
		i++
	}
}

func extractLinks(content string) []string {
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
