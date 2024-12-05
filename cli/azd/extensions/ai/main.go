package main

import (
	"context"
	"os"

	"github.com/azure/azure-dev/cli/azd/extensions/ai/internal/cmd"
	"github.com/fatih/color"
)

func main() {
	// Execute the root command
	ctx := context.Background()
	rootCmd := cmd.NewRootCommand()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		color.Red("Error: %v", err)
		os.Exit(1)
	}
}
