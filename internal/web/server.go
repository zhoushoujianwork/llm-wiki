package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strings"

	"llm-wiki/internal/wiki"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the local web UI server.
type Server struct {
	store *wiki.Store
	mux   *http.ServeMux
	tmpls map[string]*template.Template
}

// NewServer creates a new web server backed by the given wiki store.
func NewServer(store *wiki.Store) (*Server, error) {
	funcMap := template.FuncMap{
		"renderLinks": renderWikiLinks,
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
	}

	pages := []string{"index.html", "page.html", "search.html"}
	tmpls := make(map[string]*template.Template, len(pages))
	for _, name := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/base.html", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
		}
		tmpls[name] = t
	}

	s := &Server{store: store, mux: http.NewServeMux(), tmpls: tmpls}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/page/", s.handlePage)
	s.mux.HandleFunc("/search", s.handleSearch)
	s.mux.HandleFunc("/api/search", s.handleAPISearch)
	return s, nil
}

// Serve starts the HTTP server on the given address and blocks until ctx is done.
func (s *Server) Serve(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.mux}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpls[name].ExecuteTemplate(&buf, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// --- handlers ---

type indexData struct {
	Namespaces []namespaceInfo
}

type namespaceInfo struct {
	Name  string
	Pages []wiki.Page
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	pages, err := s.store.AllPages()
	if err != nil {
		http.Error(w, "failed to load pages", http.StatusInternalServerError)
		return
	}

	nsMap := make(map[string][]wiki.Page)
	for _, p := range pages {
		nsMap[p.Namespace] = append(nsMap[p.Namespace], p)
	}

	var data indexData
	for ns, ps := range nsMap {
		data.Namespaces = append(data.Namespaces, namespaceInfo{Name: ns, Pages: ps})
	}

	s.renderTemplate(w, "index.html", data)
}

type pageData struct {
	Namespace string
	Name      string
	HTML      template.HTML
	Links     []string
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	// /page/{namespace}/{name}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/page/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	ns, name := parts[0], parts[1]

	pages, err := s.store.AllPages()
	if err != nil {
		http.Error(w, "failed to load pages", http.StatusInternalServerError)
		return
	}

	for _, p := range pages {
		if p.Namespace == ns && p.Name == name {
			data := pageData{
				Namespace: p.Namespace,
				Name:      p.Name,
				HTML:      renderWikiLinks(p.Content),
				Links:     p.Links,
			}
			s.renderTemplate(w, "page.html", data)
			return
		}
	}
	http.NotFound(w, r)
}

type searchData struct {
	Query   string
	Results []wiki.Page
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data := searchData{Query: q}
	if q != "" {
		results, err := s.store.FindRelevantPages(q)
		if err == nil {
			data.Results = results
		}
	}
	s.renderTemplate(w, "search.html", data)
}

func (s *Server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var results []wiki.Page
	if q != "" {
		var err error
		results, err = s.store.FindRelevantPages(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// --- helpers ---

var wikiLinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// renderWikiLinks converts [[Page Name]] to clickable HTML links.
func renderWikiLinks(content string) template.HTML {
	escaped := template.HTMLEscapeString(content)
	result := wikiLinkRe.ReplaceAllStringFunc(escaped, func(match string) string {
		inner := match[2 : len(match)-2]
		// inner may be "namespace/page" or just "page"
		href := "/search?q=" + template.URLQueryEscaper(inner)
		return fmt.Sprintf(`<a href="%s">%s</a>`, href, inner)
	})
	// Wrap newlines in <br> for basic readability
	result = strings.ReplaceAll(result, "\n", "<br>\n")
	return template.HTML(result)
}
