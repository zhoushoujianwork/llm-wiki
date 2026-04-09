package source

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/google/uuid"
)

const githubAPIBaseURL = "https://api.github.com"

// Source represents a document source (GitHub repo, local path, or URL).
type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`           // "github", "local", "url"
	URL       string `json:"url"`            // original URL or path
	Local     string `json:"local"`          // local path (for cloned repos)
	UseGitHub bool   `json:"use_github_api"` // use GitHub API instead of git clone
}

// Document represents a single document within a source.
type Document struct {
	ID       string    `json:"id"`
	SourceID string    `json:"source_id"`
	Path     string    `json:"path"`     // absolute path or virtual path
	RelPath  string    `json:"rel_path"` // relative to source root
	Type     string    `json:"type"`     // "markdown", "pdf", "text"
	Checksum string    `json:"checksum"` // for change detection
	Content  []byte    `json:"-"`
	ModTime  time.Time `json:"-"`
}

type githubRepository struct {
	DefaultBranch string    `json:"default_branch"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type githubTreeResponse struct {
	Tree []githubTreeEntry `json:"tree"`
}

type githubTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type githubContentResponse struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type githubCommitEnvelope struct {
	Commit githubCommit `json:"commit"`
}

type githubCommit struct {
	Committer githubCommitPerson `json:"committer"`
	Author    githubCommitPerson `json:"author"`
}

type githubCommitPerson struct {
	Date time.Time `json:"date"`
}

// Manager handles source discovery and management.
type Manager struct {
	sourcesDir  string
	sourcesFile string
	httpClient  *http.Client
}

// NewManager creates a new source manager.
func NewManager(sourcesDir string) *Manager {
	return &Manager{
		sourcesDir:  sourcesDir,
		sourcesFile: filepath.Join(sourcesDir, ".sources.json"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Add adds a new source.
func (m *Manager) Add(urlOrPath string, useGithubAPI bool) (*Source, error) {
	sources, _ := m.List()

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
		absPath, err := filepath.Abs(urlOrPath)
		if err != nil {
			return nil, err
		}
		src.Type = "local"
		src.URL = absPath
		src.Local = absPath
		src.Name = filepath.Base(absPath)
	}

	for _, s := range sources {
		if s.URL == src.URL {
			return &s, nil
		}
	}

	src.ID = uuid.New().String()

	if src.Type == "github" && !src.UseGitHub {
		localPath := filepath.Join(m.sourcesDir, src.Name)
		if err := m.cloneOrPull(urlOrPath, localPath); err != nil {
			return nil, fmt.Errorf("git clone/pull failed: %w", err)
		}
		src.Local = localPath
	} else if src.Type == "github" && src.UseGitHub {
		src.Local = filepath.Join(m.sourcesDir, src.Name)
	}

	sources = append(sources, src)
	if err := m.saveSources(sources); err != nil {
		return nil, err
	}

	return &src, nil
}

// List returns all sources.
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

// Remove removes a source by name.
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

// SyncAll pulls all GitHub repos that use clone mode.
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

// DiscoverDocuments finds all documents in a source.
func (m *Manager) DiscoverDocuments(src Source) ([]Document, error) {
	if src.UseGitHub {
		return m.discoverGitHubDocuments(src)
	}

	root := src.Local
	if root == "" {
		return nil, fmt.Errorf("source %s has no local root", src.Name)
	}

	var docs []Document
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		docType := detectDocumentType(rel)
		if docType == "" || isHiddenPath(rel) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		docs = append(docs, Document{
			ID:       uuid.New().String(),
			SourceID: src.ID,
			Path:     path,
			RelPath:  filepath.ToSlash(rel),
			Type:     docType,
			Checksum: checksumForContent(content, info.ModTime()),
			Content:  content,
			ModTime:  info.ModTime(),
		})
		return nil
	})

	return docs, err
}

func (m *Manager) discoverGitHubDocuments(src Source) ([]Document, error) {
	owner, repo, err := parseGitHubRepo(src.URL)
	if err != nil {
		return nil, err
	}

	repository, err := m.fetchGitHubRepository(owner, repo)
	if err != nil {
		return nil, err
	}

	tree, err := m.fetchGitHubTree(owner, repo, repository.DefaultBranch)
	if err != nil {
		return nil, err
	}

	var docs []Document
	for _, entry := range tree.Tree {
		if entry.Type != "blob" || isHiddenPath(entry.Path) {
			continue
		}

		docType := detectDocumentType(entry.Path)
		if docType == "" {
			continue
		}

		content, err := m.fetchGitHubFileContent(owner, repo, repository.DefaultBranch, entry.Path)
		if err != nil {
			return nil, fmt.Errorf("fetching %s from GitHub API: %w", entry.Path, err)
		}

		modTime, err := m.fetchGitHubFileModTime(owner, repo, repository.DefaultBranch, entry.Path)
		if err != nil {
			modTime = repository.UpdatedAt
		}

		docs = append(docs, Document{
			ID:       uuid.New().String(),
			SourceID: src.ID,
			Path:     fmt.Sprintf("github://%s/%s/%s", owner, repo, entry.Path),
			RelPath:  entry.Path,
			Type:     docType,
			Checksum: checksumForContent(content, modTime),
			Content:  content,
			ModTime:  modTime,
		})
	}

	return docs, nil
}

func (m *Manager) cloneOrPull(url, path string) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)

	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return gitPull(path)
	}
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

	err = w.Pull(&git.PullOptions{Force: true})
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
	_ = os.MkdirAll(m.sourcesDir, 0o755)
	return os.WriteFile(m.sourcesFile, data, 0o644)
}

func (m *Manager) fetchGitHubRepository(owner, repo string) (*githubRepository, error) {
	var repository githubRepository
	if err := m.githubGET(fmt.Sprintf("/repos/%s/%s", owner, repo), &repository); err != nil {
		return nil, fmt.Errorf("fetch repository metadata: %w", err)
	}
	return &repository, nil
}

func (m *Manager) fetchGitHubTree(owner, repo, ref string) (*githubTreeResponse, error) {
	var tree githubTreeResponse
	if err := m.githubGET(fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, url.PathEscape(ref)), &tree); err != nil {
		return nil, fmt.Errorf("fetch repository tree: %w", err)
	}
	return &tree, nil
}

func (m *Manager) fetchGitHubFileContent(owner, repo, ref, filePath string) ([]byte, error) {
	var payload githubContentResponse
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, escapeGitHubPath(filePath), url.QueryEscape(ref))
	if err := m.githubGET(endpoint, &payload); err != nil {
		return nil, err
	}

	if payload.Encoding != "base64" {
		return nil, fmt.Errorf("unsupported GitHub content encoding %q", payload.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decode GitHub content: %w", err)
	}
	return decoded, nil
}

func (m *Manager) fetchGitHubFileModTime(owner, repo, ref, filePath string) (time.Time, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?path=%s&sha=%s&per_page=1", owner, repo, url.QueryEscape(filePath), url.QueryEscape(ref))
	var commits []githubCommitEnvelope
	if err := m.githubGET(endpoint, &commits); err != nil {
		return time.Time{}, err
	}
	if len(commits) == 0 {
		return time.Time{}, fmt.Errorf("no commits found for %s", filePath)
	}
	if !commits[0].Commit.Committer.Date.IsZero() {
		return commits[0].Commit.Committer.Date, nil
	}
	if !commits[0].Commit.Author.Date.IsZero() {
		return commits[0].Commit.Author.Date, nil
	}
	return time.Time{}, fmt.Errorf("commit metadata missing date for %s", filePath)
}

func (m *Manager) githubGET(path string, target any) error {
	req, err := http.NewRequest(http.MethodGet, githubAPIBaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "llm-wiki")
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode GitHub API response: %w", err)
	}
	return nil
}

func parseGitHubRepo(rawURL string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parse GitHub URL: %w", err)
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub repo URL: %s", rawURL)
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), nil
}

func detectDocumentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "markdown"
	case ".pdf":
		return "pdf"
	case ".txt", ".text":
		return "text"
	default:
		return ""
	}
}

func isHiddenPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func escapeGitHubPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func guessName(urlOrPath string) string {
	parts := strings.Split(strings.TrimSuffix(urlOrPath, ".git"), "/")
	return parts[len(parts)-1]
}

func checksumForContent(content []byte, modTime time.Time) string {
	withTimestamp := append([]byte{}, content...)
	withTimestamp = append(withTimestamp, []byte(modTime.UTC().Format(time.RFC3339Nano))...)
	sum := sha256.Sum256(withTimestamp)
	return hex.EncodeToString(sum[:])
}
