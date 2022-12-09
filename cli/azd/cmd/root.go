// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/cmd/middleware"
	"github.com/golobby/container/v3"

	// Importing for infrastructure provider plugin registrations

	_ "github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning/bicep"
	_ "github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning/terraform"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"

	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/internal/telemetry"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/spf13/cobra"
)

func NewRootCmd(staticHelp bool) *cobra.Command {
	prevDir := ""
	opts := &internal.GlobalCommandOptions{GenerateStaticHelp: staticHelp}

	cmd := &cobra.Command{
		Use:   "azd",
		Short: "Azure Developer CLI is a command-line interface for developers who build Azure solutions.",
		//nolint:lll
		Long: `Azure Developer CLI is a command-line interface for developers who build Azure solutions.

To begin working with Azure Developer CLI, run the ` + output.WithBackticks("azd up") + ` command by supplying a sample template in an empty directory:

	$ azd up –-template todo-nodejs-mongo

You can pick a template by running ` + output.WithBackticks("azd template list") + `and then supplying the repo name as a value to ` + output.WithBackticks("--template") + `.

The most common next commands are:

	$ azd pipeline config
	$ azd deploy
	$ azd monitor --overview

For more information, visit the Azure Developer CLI Dev Hub: https://aka.ms/azure-dev/devhub.`,
		Annotations: map[string]string{
			actions.AnnotationName: "azd",
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Cwd != "" {
				current, err := os.Getwd()

				if err != nil {
					return err
				}

				prevDir = current

				if err := os.Chdir(opts.Cwd); err != nil {
					return fmt.Errorf("failed to change directory to %s: %w", opts.Cwd, err)
				}
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			// This is just for cleanliness and making writing tests simpler since
			// we can just remove the entire project folder afterwards.
			// In practical execution, this wouldn't affect much, since the CLI is exiting.
			if prevDir != "" {
				return os.Chdir(prevDir)
			}

			return nil
		},
		SilenceUsage: true,
	}

	cmd.DisableAutoGenTag = true
	cmd.CompletionOptions.HiddenDefaultCmd = true
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Gets help for %s.", cmd.Name()))
	cmd.PersistentFlags().StringVarP(&opts.Cwd, "cwd", "C", "", "Sets the current working directory.")
	cmd.PersistentFlags().BoolVar(&opts.EnableDebugLogging, "debug", false, "Enables debugging and diagnostics logging.")
	cmd.PersistentFlags().
		BoolVar(
			&opts.NoPrompt,
			"no-prompt",
			false,
			"Accepts the default value instead of prompting, or it fails if there is no default.")
	cmd.SetHelpTemplate(
		fmt.Sprintf("%s\nPlease let us know how we are doing: https://aka.ms/azure-dev/hats\n", cmd.HelpTemplate()),
	)

	opts.EnableTelemetry = telemetry.IsTelemetryEnabled()

	cmd.AddCommand(configCmd(opts))
	cmd.AddCommand(envCmd(opts))
	cmd.AddCommand(infraCmd(opts))
	cmd.AddCommand(pipelineCmd(opts))
	cmd.AddCommand(telemetryCmd(opts))
	cmd.AddCommand(templatesCmd(opts))
	cmd.AddCommand(authCmd(opts))

	cmd.AddCommand(BuildCmd(opts, versionCmdDesign, newVersionAction, &actions.BuildOptions{DisableTelemetry: true}))
	cmd.AddCommand(BuildCmd(opts, showCmdDesign, newShowAction, nil))
	cmd.AddCommand(BuildCmd(opts, restoreCmdDesign, newRestoreAction, nil))
	cmd.AddCommand(BuildCmd(opts, loginCmdDesign, newLoginAction, nil))
	cmd.AddCommand(BuildCmd(opts, logoutCmdDesign, newLogoutAction, nil))
	cmd.AddCommand(BuildCmd(opts, monitorCmdDesign, newMonitorAction, nil))
	cmd.AddCommand(BuildCmd(opts, downCmdDesign, newInfraDeleteAction, nil))
	cmd.AddCommand(BuildCmd(opts, initCmdDesign, newInitAction, nil))
	cmd.AddCommand(BuildCmd(opts, upCmdDesign, newUpAction, nil))
	cmd.AddCommand(BuildCmd(opts, provisionCmdDesign, newInfraCreateAction, nil))
	cmd.AddCommand(BuildCmd(opts, deployCmdDesign, newDeployAction, nil))

	middleware.SetContainer(container.Global)
	middleware.Use("debug", middleware.NewDebugMiddleware)
	middleware.Use("telemetry", middleware.NewTelemetryMiddleware)

	return cmd
}

type commandDesignBuilder[F any] func(opts *internal.GlobalCommandOptions) (*cobra.Command, *F)

func BuildCmd[F any](
	opts *internal.GlobalCommandOptions,
	buildCommandDesign commandDesignBuilder[F],
	buildAction any, // IoC will validate that is a proper resolver function
	buildOptions *actions.BuildOptions) *cobra.Command {
	cmd, flags := buildCommandDesign(opts)
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Gets help for %s.", cmd.Name()))

	if buildOptions == nil {
		buildOptions = &actions.BuildOptions{}
	}

	actionName, err := createActionName(cmd)
	if err != nil {
		panic(err)
	}

	// Register action resolver and flag instances
	container.MustNamedSingletonLazy(container.Global, actionName, buildAction)
	registerInstance(container.Global, flags)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ctx = tools.WithInstalledCheckCache(ctx)

		// Azd components
		registerCommonDependencies(container.Global)

		// Register global instances
		registerInstance(container.Global, ctx)
		registerInstance(container.Global, buildOptions)
		registerInstance(container.Global, opts)
		registerInstance(container.Global, cmd)
		registerInstance(container.Global, args)

		var console input.Console
		err := container.Resolve(&console)
		if err != nil {
			return fmt.Errorf("failed resolving console : %w", err)
		}

		var action actions.Action
		err = container.NamedResolve(&action, actionName)
		if err != nil {
			return fmt.Errorf("failed resolving action '%s' : %w", actionName, err)
		}

		runOptions := middleware.Options{
			Name:    cmd.CommandPath(),
			Aliases: cmd.Aliases,
		}

		actionResult, err := middleware.RunAction(ctx, runOptions, action)
		// At this point, we know that there might be an error, so we can silence cobra from showing it after us.
		cmd.SilenceErrors = true

		// It is valid for a command to return a nil action result and error. If we have a result or an error, display it,
		// otherwise don't print anything.
		if actionResult != nil || err != nil {
			console.MessageUxItem(ctx, actions.ToUxItem(actionResult, err))
		}

		return err
	}

	return cmd
}

func createActionName(cmd *cobra.Command) (string, error) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	actionName, exists := cmd.Annotations[actions.AnnotationName]
	if !exists {
		return "", fmt.Errorf(
			"cobra command '%s' is missing required annotation '%s'",
			cmd.CommandPath(),
			actions.AnnotationName,
		)
	}

	actionName = strings.TrimSpace(actionName)
	actionName = strings.ReplaceAll(actionName, " ", "-")
	return strings.ToLower(fmt.Sprintf("%s-action", actionName)), nil
}
