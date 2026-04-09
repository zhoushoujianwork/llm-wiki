package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/mitchellh/go-wordwrap"
)

// Source represents a document source (GitHub repo, local path, or URL)
type Source struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`   // "github", "local", "url"
	URL    string `json:"url"`    // original URL or path
	Local  string `json:"local"` // local path (for cloned repos)
}

// Document represents a single document within a source
type Document struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Path     string `json:"path"`       // absolute path
	RelPath  string `json:"rel_path"`   // relative to source root
	Type     string `json:"type"`       // "markdown", "pdf", "text"
	Content  string `json:"content"`   // raw content
	Checksum string `json:"checksum"`   // for change detection
}

// Manager handles source discovery and management
type Manager struct {
	sourcesDir string
	sourcesFile string
}

// NewManager creates a new source manager
func NewManager(sourcesDir string) *Manager {
	return &Manager{
		sourcesDir: sourcesDir,
		sourcesFile: filepath.Join(sourcesDir, ".sources.json"),
	}
}

// Add adds a new source
func (m *Manager) Add(urlOrPath string) (*Source, error) {
	sources, _ := m.List()

	// Determine type
	var src Source
	if strings.HasPrefix(urlOrPath, "http://") || strings.HasPrefix(urlOrPath, "https://") {
		if strings.Contains(urlOrPath, "github.com") {
			src.Type = "github"
		} else {
			src.Type = "url"
		}
		src.URL = urlOrPath
		src.Name = guessName(urlOrPath)
	} else {
		// Local path
		absPath, err := filepath.Abs(urlOrPath)
		if err != nil {
			return nil, err
		}
		src.Type = "local"
		src.URL = absPath
		src.Local = absPath
		src.Name = filepath.Base(absPath)
	}

	// Check for duplicates
	for _, s := range sources {
		if s.URL == src.URL {
			return &s, nil // already exists
		}
	}

	src.ID = uuid.New().String()

	// Clone/sync if GitHub
	if src.Type == "github" {
		localPath := filepath.Join(m.sourcesDir, src.Name)
		if err := m.cloneOrPull(urlOrPath, localPath); err != nil {
			return nil, fmt.Errorf("git clone/pull failed: %w", err)
		}
		src.Local = localPath
	}

	sources = append(sources, src)
	if err := m.saveSources(sources); err != nil {
		return nil, err
	}

	return &src, nil
}

// List returns all sources
func (m *Manager) List() ([]Source, error) {
	data, err := os.ReadFile(m.sourcesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sources []Source
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// Remove removes a source by name
func (m *Manager) Remove(name string) error {
	sources, err := m.List()
	if err != nil {
		return err
	}
	var remaining []Source
	for _, s := range sources {
		if s.Name != name {
			remaining = append(remaining, s)
		}
	}
	return m.saveSources(remaining)
}

// SyncAll pulls all GitHub repos
func (m *Manager) SyncAll() (int, error) {
	sources, err := m.List()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, s := range sources {
		if s.Type == "github" {
			if err := m.cloneOrPull(s.URL, s.Local); err != nil {
				fmt.Printf("Warning: failed to sync %s: %v\n", s.Name, err)
				continue
			}
			count++
		}
	}
	return count, nil
}

// DiscoverDocuments finds all documents in a source
func (m *Manager) DiscoverDocuments(src Source) ([]Document, error) {
	root := src.Local
	if root == "" {
		root = src.URL
	}

	var docs []Document
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		ext := strings.ToLower(filepath.Ext(path))

		var docType string
		switch ext {
		case ".md", ".markdown":
			docType = "markdown"
		case ".pdf":
			docType = "pdf"
		case ".txt", ".text":
			docType = "text"
		default:
			return nil // skip unsupported types
		}

		// Skip hidden files
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		doc := Document{
			ID:       uuid.New().String(),
			SourceID: src.ID,
			Path:     path,
			RelPath:  rel,
			Type:     docType,
		}
		docs = append(docs, doc)
		return nil
	})

	return docs, err
}

func (m *Manager) cloneOrPull(url, path string) error {
	// Check if already cloned
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		// git pull
		return runGit(path, "pull", "origin", "main")
	}
	// git clone
	return runGit(m.sourcesDir, "clone", url, path)
}

func (m *Manager) saveSources(sources []Source) error {
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(m.sourcesDir, 0755)
	return os.WriteFile(m.sourcesFile, data, 0644)
}

func guessName(urlOrPath string) string {
	parts := strings.Split(strings.TrimSuffix(urlOrPath, ".git"), "/")
	return parts[len(parts)-1]
}

// Simple git wrapper — uses system git binary
func runGit(dir string, args ...string) error {
	// Placeholder: will be replaced with go-git or exec
	_ = wordwrap.WrapString("placeholder", 80)
	return nil // TODO: implement
}
