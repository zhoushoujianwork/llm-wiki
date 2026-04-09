package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/google/uuid"
)

// Source represents a document source (GitHub repo, local path, or URL)
type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`   // "github", "local", "url"
	URL       string `json:"url"`    // original URL or path
	Local     string `json:"local"`  // local path (for cloned repos)
	UseGitHub bool   `json:"use_github_api"` // use GitHub API instead of git clone
}

// Document represents a single document within a source
type Document struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Path     string `json:"path"`      // absolute path
	RelPath  string `json:"rel_path"`  // relative to source root
	Type     string `json:"type"`      // "markdown", "pdf", "text"
	Checksum string `json:"checksum"`  // for change detection
}

// Manager handles source discovery and management
type Manager struct {
	sourcesDir  string
	sourcesFile string
}

// NewManager creates a new source manager
func NewManager(sourcesDir string) *Manager {
	return &Manager{
		sourcesDir:  sourcesDir,
		sourcesFile: filepath.Join(sourcesDir, ".sources.json"),
	}
}

// Add adds a new source
func (m *Manager) Add(urlOrPath string, useGithubAPI bool) (*Source, error) {
	sources, _ := m.List()

	// Determine type
	var src Source
	if strings.HasPrefix(urlOrPath, "http://") || strings.HasPrefix(urlOrPath, "https://") {
		if strings.Contains(urlOrPath, "github.com") {
			src.Type = "github"
			src.UseGitHub = useGithubAPI
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

	// Clone/sync if GitHub (unless using GitHub API)
	if src.Type == "github" && !src.UseGitHub {
		localPath := filepath.Join(m.sourcesDir, src.Name)
		if err := m.cloneOrPull(urlOrPath, localPath); err != nil {
			return nil, fmt.Errorf("git clone/pull failed: %w", err)
		}
		src.Local = localPath
	} else if src.Type == "github" && src.UseGitHub {
		// GitHub API mode: store URL but don't clone
		src.Local = filepath.Join(m.sourcesDir, src.Name)
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
		if s.Type == "github" && !s.UseGitHub {
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
	if root == "" || src.UseGitHub {
		return nil, fmt.Errorf("GitHub API mode not yet implemented for document discovery")
	}

	var docs []Document
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
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
			return nil
		}

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
	os.MkdirAll(filepath.Dir(path), 0755)

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		// git pull
		return gitPull(path)
	}
	// git clone
	return gitClone(url, path)
}

func gitClone(url, path string) error {
	_, err := git.PlainClone(path, false, &git.CloneOptions{
		URL:      url,
		Depth:    100,
		Progress: os.Stdout,
	})
	return err
}

func gitPull(path string) error {
	r, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	err = w.Pull(&git.PullOptions{
		Force: true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}
	return nil
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
