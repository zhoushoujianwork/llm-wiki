package source

import (
	"bufio"
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

	// Create gitignore matcher at root
	giMatcher, err := newGitignoreMatcher(root, nil)
	if err != nil {
		// If we can't load gitignore, continue without it
		giMatcher = nil
	}

	var docs []Document
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)

		if info.IsDir() {
			// Skip descending into .git directory
			if rel == ".git" {
				return filepath.SkipDir
			}

			// Skip descending into known build/output directories (hardcoded)
			if isIgnoredDir(rel) {
				return filepath.SkipDir
			}

			// Check if this directory has a .gitignore (may affect children)
			giPath := filepath.Join(path, ".gitignore")
			if _, err := os.Stat(giPath); err == nil && giMatcher != nil {
				// Create child matcher for this subdirectory
				subRel, _ := filepath.Rel(root, path)
				childMatcher, err := giMatcher.Child(subRel)
				if err == nil {
					giMatcher = childMatcher
				}
			}

			// Use gitignore to check if directory should be ignored
			if giMatcher != nil && giMatcher.IsIgnored(filepath.ToSlash(rel), true) {
				return filepath.SkipDir
			}

			return nil
		}

		// Skip files in ignored paths, hidden files, or by gitignore rules
		docType := detectDocumentType(rel)
		if docType == "" || isHiddenPath(rel) || isInIgnoredPath(rel) {
			return nil
		}

		// Use gitignore to check if file should be ignored
		if giMatcher != nil && giMatcher.IsIgnored(filepath.ToSlash(rel), false) {
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

// gitignoreMatcher holds parsed .gitignore rules for a directory tree.
type gitignoreMatcher struct {
	rules     []gitignorePattern // patterns from this directory's .gitignore
	parent    *gitignoreMatcher  // parent directory's matcher
	dir       string             // the directory this matcher is for
}

type gitignorePattern struct {
	pattern  string // the pattern (without leading !)
	negate  bool   // true if pattern is negated (!pattern)
	dirOnly bool   // true if pattern ends with /
}

// newGitignoreMatcher creates a gitignore matcher for the given root directory.
// It loads .gitignore from root and merges with parent matcher's rules.
func newGitignoreMatcher(root string, parent *gitignoreMatcher) (*gitignoreMatcher, error) {
	m := &gitignoreMatcher{
		rules:  make([]gitignorePattern, 0),
		parent: parent,
		dir:    root,
	}

	// Load .gitignore from this directory
	giPath := filepath.Join(root, ".gitignore")
	if err := m.loadFile(giPath); err != nil {
		// .gitignore might not exist, that's fine
	}

	// Also load .git/info/exclude (local git excludes, lower priority than .gitignore)
	excludePath := filepath.Join(root, ".git", "info", "exclude")
	if err := m.loadFile(excludePath); err != nil {
		// might not exist
	}

	return m, nil
}

// loadFile loads gitignore patterns from a file.
func (m *gitignoreMatcher) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle negation
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}

		// Directory-only pattern (ends with /)
		dirOnly := false
		if strings.HasSuffix(line, "/") {
			dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}

		// Remove trailing whitespace for non-negated patterns
		line = strings.TrimRight(line, " ")

		// Skip empty lines after processing
		if line == "" {
			continue
		}

		m.rules = append(m.rules, gitignorePattern{
			pattern:  line,
			negate:   negate,
			dirOnly:  dirOnly,
		})
	}
	return scanner.Err()
}

// Child returns a new matcher for a subdirectory, inheriting parent rules.
func (m *gitignoreMatcher) Child(subdir string) (*gitignoreMatcher, error) {
	childPath := filepath.Join(m.dir, subdir)
	return newGitignoreMatcher(childPath, m)
}

// IsIgnored returns true if the given relative path (to the matcher's root) should be ignored.
// It checks all patterns in the matcher chain (parent to child), with later rules taking precedence.
// For files, it also checks whether any parent directory is ignored (gitignore behavior).
func (m *gitignoreMatcher) IsIgnored(relPath string, isDir bool) bool {
	// Check if the path itself is ignored
	if m.isIgnoredByRule(relPath, isDir) {
		return true
	}

	// For files, also check if any parent directory is ignored
	// (if a directory is ignored, all its contents are ignored)
	if !isDir {
		parts := strings.Split(filepath.ToSlash(relPath), "/")
		for i := len(parts) - 1; i > 0; i-- {
			parent := strings.Join(parts[:i], "/")
			if m.isIgnoredByRule(parent, true) {
				return true
			}
		}
	}

	return false
}

// isIgnoredByRule checks if a path matches any rule (without checking parent directories).
func (m *gitignoreMatcher) isIgnoredByRule(relPath string, isDir bool) bool {
	var rules []gitignorePattern

	// Collect rules from parent chain (parent rules first, then current)
	var chain []*gitignoreMatcher
	for cur := m; cur != nil; cur = cur.parent {
		chain = append([]*gitignoreMatcher{cur}, chain...)
	}
	for _, matcher := range chain {
		rules = append(rules, matcher.rules...)
	}

	matched := false
	for _, rule := range rules {
		if rule.dirOnly && !isDir {
			continue
		}
		if matchPattern(relPath, rule.pattern) {
			matched = !rule.negate
		}
	}
	return matched
}

// matchPattern checks if a path matches a gitignore pattern.
// This implements a subset of gitignore pattern matching.
func matchPattern(path, pattern string) bool {
	// Normalize path separators
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	// Handle anchored patterns (starting with /)
	anchored := false
	if strings.HasPrefix(pattern, "/") {
		anchored = true
		pattern = pattern[1:]
	}

	// Handle trailing /**
	hasTrailingGlob := false
	if strings.HasSuffix(pattern, "/**") {
		hasTrailingGlob = true
		pattern = strings.TrimSuffix(pattern, "/**")
	}

	// Handle ** in the middle of patterns
	// Split by /** and check each segment
	segments := strings.Split(pattern, "**")

	if len(segments) == 1 {
		// No ** in pattern, do exact or glob matching
		if anchored {
			// Anchored: pattern must match from the beginning of the path
			if hasTrailingGlob {
				return strings.HasPrefix(path, segments[0])
			}
			return path == segments[0]
		}
		// Not anchored: pattern can match anywhere in the path
		if hasTrailingGlob {
			return strings.HasPrefix(path, segments[0]) || strings.Contains(path, "/"+segments[0])
		}
		pat := segments[0]
		// Git: a pattern with no "/" matches any path component (e.g. managed_components/ ignores nested files).
		if !strings.Contains(pat, "/") && !strings.ContainsAny(pat, "*?[]") {
			for _, seg := range strings.Split(path, "/") {
				if seg == pat {
					return true
				}
			}
		}
		return matchGlob(path, pat)
	}

	// Pattern contains **
	// Check if path starts with the prefix before first **
	prefix := segments[0]
	if prefix != "" {
		if anchored {
			if !strings.HasPrefix(path, prefix) {
				return false
			}
			path = path[len(prefix):]
		} else {
			idx := strings.Index(path, prefix)
			if idx == -1 {
				return false
			}
			path = path[idx:]
		}
	}

	// Check remaining segments
	for i := 1; i < len(segments); i++ {
		segment := segments[i]
		if segment == "" {
			continue
		}
		// Find this segment in path (can span directories)
		if hasTrailingGlob && i == len(segments)-1 {
			// Last segment with trailing /** matches rest of path
			return true
		}
		idx := strings.Index(path, segment)
		if idx == -1 {
			return false
		}
		path = path[idx+len(segment):]
	}

	return true
}

// matchGlob does simple glob matching with * (matches anything except /).
func matchGlob(path, pattern string) bool {
	// Simple case: no wildcards
	if !strings.ContainsAny(pattern, "*?") {
		return path == pattern
	}

	// Convert pattern to regex
	re := ""
	for _, c := range pattern {
		switch c {
		case '*':
			re += "[^/]*"
		case '?':
			re += "[^/]"
		case '.', '(', ')', '+', '{', '}', '|', '^', '$', '\\', '[', ']':
			re += "\\" + string(c)
		default:
			re += string(c)
		}
	}

	// Simple regex matching for the path
	// Match full path or just filename portion
	matched, _ := matchRegexp("^" + re + "$", path)
	if matched {
		return true
	}
	matched2, _ := matchRegexp("^.*/" + re + "$", path)
	return matched2
}

func matchRegexp(pattern, text string) (bool, error) {
	// Simple regexp implementation for our specific case
	// We just check if pattern matches text exactly
	pi := 0
	ti := 0
	starIdx := -1
	tStarIdx := -1

	for pi < len(pattern) && ti < len(text) {
		pc := pattern[pi]
		tc := text[ti]

		if pc == '*' {
			starIdx = pi
			tStarIdx = ti
			pi++
		} else if pc == tc || pc == '?' {
			pi++
			ti++
		} else if starIdx != -1 {
			pi = starIdx + 1
			ti = tStarIdx + 1
			tStarIdx = ti
		} else {
			return false, nil
		}
	}

	// Handle trailing *
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}

	return pi == len(pattern) && ti == len(text), nil
}

// isIgnoredDir returns true for directory names that should never be traversed.
func isIgnoredDir(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "node_modules" || part == "dist" || part == "build" ||
			part == ".git" || part == "vendor" || part == "__pycache__" ||
			part == ".vite" || part == ".next" || part == ".nuxt" ||
			part == "target" || part == ".cache" || part == ".turbo" ||
			part == "managed_components" || part == ".pio" {
			return true
		}
	}
	return false
}

// isInIgnoredPath returns true if any path component is an ignored directory.
func isInIgnoredPath(path string) bool {
	return isIgnoredDir(path)
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
