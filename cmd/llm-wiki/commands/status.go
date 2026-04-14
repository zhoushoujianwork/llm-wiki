package commands

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"llm-wiki/internal/compiler"
	"llm-wiki/internal/source"
	"llm-wiki/internal/wiki"
)

func NewStatusCmd() *cobra.Command {
	var singleSource string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show which sources are stale and need recompilation",
		Run: func(cmd *cobra.Command, args []string) {
			m := source.NewManager(getSourcesDir())

			sources, err := m.List()
			if err != nil {
				fmt.Printf("Error listing sources: %v\n", err)
				return
			}

			if len(sources) == 0 {
				fmt.Println("No sources configured. Add one with 'llm-wiki source add <url>'.")
				return
			}

			c := compiler.NewCompiler()
			store := wiki.NewStore(getWikiDir())
			promptHash := c.PromptHash()

			totalStale := 0
			totalFresh := 0

			for _, src := range sources {
				if singleSource != "" && src.Name != singleSource {
					continue
				}

				docs, err := m.DiscoverDocuments(src)
				if err != nil {
					fmt.Printf("Source %s: error discovering documents: %v\n", src.Name, err)
					continue
				}

				entries := store.StatusEntries(src.Name, docs, promptHash)

				// Sort for stable output: stale first, then by path.
				sort.Slice(entries, func(i, j int) bool {
					if entries[i].Stale != entries[j].Stale {
						return entries[i].Stale
					}
					return entries[i].RelPath < entries[j].RelPath
				})

				staleCount := 0
				for _, e := range entries {
					if e.Stale {
						staleCount++
					}
				}

				fmt.Printf("\nSource: %s (%d documents, %d stale)\n", src.Name, len(entries), staleCount)

				for _, e := range entries {
					if e.Stale {
						fmt.Printf("  STALE  %s  [%s]\n", e.RelPath, e.Reason)
					} else {
						compiled := ""
						if !e.CompiledAt.IsZero() {
							compiled = "  compiled " + e.CompiledAt.Format("2006-01-02 15:04:05 UTC")
						}
						fmt.Printf("  ok     %s%s\n", e.RelPath, compiled)
					}
				}

				totalStale += staleCount
				totalFresh += len(entries) - staleCount
			}

			fmt.Printf("\nSummary: %d up to date, %d stale\n", totalFresh, totalStale)
			if totalStale > 0 {
				fmt.Println("Run 'llm-wiki compile' to recompile stale documents.")
			}
		},
	}

	cmd.Flags().StringVar(&singleSource, "source", "", "Show status for only this source")

	return cmd
}
