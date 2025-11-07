// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	// The root command IS the exec command
	rootCmd := newExecCommand()

	// Update the Use field to match the expected pattern
	rootCmd.Use = "azd exec <command> [args...]"
	rootCmd.Short = "Execute commands with azd environment variables loaded."
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.CompletionOptions = cobra.CompletionOptions{
		DisableDefaultCmd: true,
	}

	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	// Add version as a subcommand
	rootCmd.AddCommand(newVersionCommand())

	return rootCmd
}
