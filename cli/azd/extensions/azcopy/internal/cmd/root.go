package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "azcopy [options]",
		Short:         "Runs azcopy commmands from within azd",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newUploadCommand())
	rootCmd.AddCommand(newVersionCommand())

	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	return rootCmd
}
