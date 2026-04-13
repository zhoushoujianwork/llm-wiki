package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"llm-wiki/internal/wiki"
)

// compiledDocument mirrors the internal/wiki type for JSON unmarshaling
type compiledDoc struct {
	Checksum   string    `json:"checksum"`
	Pages      []string  `json:"pages"`
	CompiledAt string    `json:"compiled_at"`
}

func NewInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <page>",
		Short: "Display compilation metadata for a wiki page",
		Long: `Display compilation metadata for a specific wiki page, including:
- Source file path and checksum
- Compilation timestamp
- Generation prompt hash (if available in future)
- Related pages and cross-references`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			pageName := args[0]
			
			// Load store to access index
			store := wiki.NewStore(getWikiDir())
			
			// Look up page paths from index
			paths, ok := store.GetEntities()[pageName]
			if !ok || len(paths) == 0 {
				fmt.Printf("❌ Page '%s' not found in wiki.\n", pageName)
				fmt.Println("\nAvailable pages:")
				listPages(store)
				return
			}
			
			fmt.Printf("📄 Page: %s\n\n", pageName)
			
			for _, path := range paths {
				displayPageMetadata(path, store)
			}
		},
	}
	
	return cmd
}

func displayPageMetadata(path string, store *wiki.Store) {
	// Read the page content
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("⚠️  Error reading page %s: %v\n\n", path, err)
		return
	}
	
	// Parse page info from path
	relPath, _ := filepath.Rel(getWikiDir(), path)
	parts := strings.Split(relPath, string(filepath.Separator))
	namespace := ""
	if len(parts) > 1 {
		namespace = parts[0]
	}
	pageTitle := strings.TrimSuffix(parts[len(parts)-1], ".md")
	
	// Get compilation metadata from state
	compiledState := loadCompiledState()
	
	fmt.Printf("┌─ Metadata\n")
	fmt.Printf("│ Path:        %s\n", relPath)
	fmt.Printf("│ Namespace:   %s\n", namespace)
	fmt.Printf("│ Title:       %s\n", pageTitle)
	
	// Look for source in compiled state
	foundSource := false
	for key, doc := range compiledState {
		for _, page := range doc.Pages {
			if page == relPath {
				fmt.Printf("│ Source:      %s\n", key)
				fmt.Printf("│ Checksum:    %s\n", truncateString(doc.Checksum, 16))
				fmt.Printf("│ Compiled:    %s\n", doc.CompiledAt)
				foundSource = true
				break
			}
		}
		if foundSource {
			break
		}
	}
	
	if !foundSource {
		fmt.Printf("│ Source:      Unknown (no compilation record)\n")
		fmt.Printf("│ Checksum:    -\n")
		fmt.Printf("│ Compiled:    -\n")
	}
	
	fmt.Printf("└─ End Metadata\n\n")
	
	// Display preview of content
	lines := strings.Split(string(content), "\n")
	var previewLines []string
	for i, line := range lines {
		if i >= 10 {
			previewLines = append(previewLines, "...")
			break
		}
		previewLines = append(previewLines, line)
	}
	
	fmt.Printf("┌─ Content Preview\n")
	fmt.Println(strings.Join(previewLines, "\n"))
	fmt.Printf("└─ End Preview\n")
}

func loadCompiledState() map[string]compiledDoc {
	compiledFile := filepath.Join(getWikiDir(), ".compiled.json")
	data, err := os.ReadFile(compiledFile)
	if err != nil {
		return make(map[string]compiledDoc)
	}
	
	var state map[string]compiledDoc
	if err := json.Unmarshal(data, &state); err != nil {
		return make(map[string]compiledDoc)
	}
	
	return state
}

func listPages(store *wiki.Store) {
	err := filepath.Walk(getWikiDir(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		
		relPath, _ := filepath.Rel(getWikiDir(), path)
		pageName := strings.TrimSuffix(filepath.Base(path), ".md")
		fmt.Printf("  • %s (%s)\n", pageName, relPath)
		return nil
	})
	
	if err != nil {
		fmt.Printf("Error listing pages: %v\n", err)
	}
}
