package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"llm-wiki/internal/mergeconcepts"
)

func NewMergeConceptsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge-concepts",
		Short: "Merge same-concept pages across namespaces into _concepts/",
		Long: `Scan all wiki namespaces for pages with matching or semantically similar titles.
Uses LLM to determine if two pages describe the same concept, then merges them
into a global _concepts/ namespace with back-references to source pages.

Original namespace pages are preserved unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			wikiDir := getWikiDir()
			if wikiDir == "" {
				return fmt.Errorf("wiki directory not configured")
			}

			store := createWikiStore(wikiDir)
			client := createLLMClient()
			merger := mergeconcepts.NewMerger(store, client)

			fmt.Println("Scanning namespaces for cross-namespace concepts...")
			results, err := merger.Run(context.Background())
			if err != nil {
				return fmt.Errorf("merge-concepts failed: %w", err)
			}

			merged := 0
			skipped := 0
			for _, r := range results {
				if r.Skipped {
					fmt.Printf("  skip  %s — %s\n", r.ConceptName, r.SkipReason)
					skipped++
				} else {
					fmt.Printf("  ✓  %s → _concepts/%s (sources: %v)\n", r.ConceptName, r.ConceptName, r.SourcePages)
					merged++
				}
			}

			fmt.Printf("\nDone. %d concepts merged, %d skipped.\n", merged, skipped)
			return nil
		},
	}
	return cmd
}
