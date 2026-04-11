package commands

import (
	"fmt"
	"github.com/spf13/cobra"
	"llm-wiki/internal/source"
)

func NewSourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage document sources",
	}
	
	cmd.AddCommand(newSourceAddCmd())
	cmd.AddCommand(newSourceListCmd())
	cmd.AddCommand(newSourceSyncCmd())
	cmd.AddCommand(newSourceRemoveCmd())
	
	return cmd
}

func newSourceAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <url-or-path>",
		Short: "Add a source (GitHub repo, URL, or local path)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			m := source.NewManager(getSourcesDir())
			src, err := m.Add(args[0], false)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Added source: %s (%s)\n", src.Name, src.Type)
		},
	}
}

func newSourceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all sources",
		Run: func(cmd *cobra.Command, args []string) {
			m := source.NewManager(getSourcesDir())
			sources, err := m.List()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			if len(sources) == 0 {
				fmt.Println("No sources configured.")
				return
			}
			for _, s := range sources {
				fmt.Printf("- %s [%s] %s\n", s.Name, s.Type, s.URL)
			}
		},
	}
}

func newSourceSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync all GitHub repos",
		Run: func(cmd *cobra.Command, args []string) {
			m := source.NewManager(getSourcesDir())
			count, err := m.SyncAll()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Synced %d sources.\n", count)
		},
	}
}

func newSourceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a source",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			m := source.NewManager(getSourcesDir())
			if err := m.Remove(args[0]); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Removed source: %s\n", args[0])
		},
	}
}
