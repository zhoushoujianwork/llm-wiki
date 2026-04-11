package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"github.com/spf13/cobra"
	"llm-wiki/internal/query"
	"llm-wiki/internal/wiki"
)

func NewAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask",
		Short: "Interactive query mode",
		Run: func(cmd *cobra.Command, args []string) {
			store := wiki.NewStore(getWikiDir())
			q := query.NewEngine(store)
			
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("LLM Wiki Interactive Mode")
			fmt.Println("Type your question and press Enter. Ctrl+C to exit.")
			fmt.Println()
			
			for {
				fmt.Print("> ")
				input, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				question := strings.TrimSpace(input)
				if question == "" {
					continue
				}
				if question == "exit" || question == "quit" {
					break
				}
				
				answer, err := q.Ask(context.Background(), question)
				if err != nil {
					fmt.Printf("Error: %v\n\n", err)
					continue
				}
				fmt.Printf("\n%s\n\n", answer)
			}
		},
	}
}
