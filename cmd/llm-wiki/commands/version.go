package commands

import (
	"fmt"
	"github.com/spf13/cobra"
)

var Version = "dev"

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("llm-wiki version %s\n", Version)
		},
	}
}
