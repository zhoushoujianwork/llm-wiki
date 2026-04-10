package commands

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/zhoushoujianwork/llm-wiki/internal/query"
	"github.com/zhoushoujianwork/llm-wiki/internal/wiki"
)

func NewQueryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query <question>",
		Short: "Query the wiki",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			question := args[0]
			for i, a := range args[1:] {
				if i == 0 {
					question += " "
				}
				question += a
			}
			
			store := wiki.NewStore(getWikiDir())
			q := query.NewEngine(store)
			
			answer, err := q.Ask(context.Background(), question)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Println(answer)
		},
	}
}
