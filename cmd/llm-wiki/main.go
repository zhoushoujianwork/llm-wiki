package main

import (
	"os"

	"github.com/zhoushoujianwork/llm-wiki/cmd/llm-wiki/commands"
)

func main() {
	cmd := commands.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}