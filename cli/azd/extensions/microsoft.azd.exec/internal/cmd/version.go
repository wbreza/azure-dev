// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Populated at build time
	Version   = "0.1.0" // Default value for development builds
	Commit    = "none"
	BuildDate = "unknown"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display the version of the exec extension.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("azd exec extension\nVersion: %s\nCommit: %s\nBuild Date: %s\n", Version, Commit, BuildDate)
		},
	}
}
