package commands

import (
	"fmt"
	"github.com/spf13/cobra"
	"llm-wiki/internal/compiler"
	"llm-wiki/internal/source"
	"llm-wiki/internal/wiki"
)

func NewCompileCmd() *cobra.Command {
	var full bool
	var singleSource string
	
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile sources into wiki pages",
		Run: func(cmd *cobra.Command, args []string) {
			m := source.NewManager(getSourcesDir())
			
			sources, err := m.List()
			if err != nil {
				fmt.Printf("Error listing sources: %v\n", err)
				return
			}
			
			if len(sources) == 0 {
				fmt.Println("No sources to compile. Add one with 'llm-wiki source add <url>'.")
				return
			}
			
			c := compiler.NewCompiler()
			store := wiki.NewStore(getWikiDir())
			
			for _, src := range sources {
				if singleSource != "" && src.Name != singleSource {
					continue
				}
				fmt.Printf("Compiling source: %s...\n", src.Name)
				docs, err := m.DiscoverDocuments(src)
				if err != nil {
					fmt.Printf("  Error discovering documents: %v\n", err)
					continue
				}
				fmt.Printf("  Found %d documents\n", len(docs))
				
				for _, doc := range docs {
					pages, err := c.CompileDocument(doc)
					if err != nil {
						fmt.Printf("  Error compiling %s: %v\n", doc.RelPath, err)
						continue
					}
					if err := store.StoreDocumentPages(src.Name, doc, pages); err != nil {
						fmt.Printf("  Error saving pages for %s: %v\n", doc.RelPath, err)
					} else {
						fmt.Printf("  ✓ %s → %d pages\n", doc.RelPath, len(pages))
					}
				}
			}
			fmt.Println("Compilation complete.")
		},
	}
	
	cmd.Flags().BoolVar(&full, "full", false, "Full recompilation (ignore cache)")
	cmd.Flags().StringVar(&singleSource, "source", "", "Compile only this source")
	
	return cmd
}
