package cmd

import (
	"fmt"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/spf13/cobra"
)

func authCmd(rootOptions *internal.GlobalCommandOptions) *cobra.Command {
	root := &cobra.Command{
		Use:    "auth",
		Hidden: true,
		Annotations: map[string]string{
			actions.AnnotationName: "auth",
		},
	}

	root.Flags().BoolP("help", "h", false, fmt.Sprintf("Gets help for %s.", root.Name()))

	root.AddCommand(BuildCmd(rootOptions, authTokenCmdDesign, newAuthTokenAction, nil))

	return root
}
