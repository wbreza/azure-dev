// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newExecCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "<command> [args...]",
		Short:              "Execute a command with azd environment variables loaded.",
		Long:               "Execute arbitrary scripts and commands with azd environment variables loaded into the subprocess.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if a command was provided
			if len(args) == 0 {
				return fmt.Errorf("no command specified\n\nUsage: azd exec <command> [args...]")
			}

			// Create a new context that includes the AZD access token
			ctx := azdext.WithAccessToken(cmd.Context())

			// Create a new AZD client
			azdClient, err := azdext.NewAzdClient()
			if err != nil {
				return fmt.Errorf("failed to create azd client: %w", err)
			}
			defer azdClient.Close()

			// Get the current environment
			currentEnvResponse, err := azdClient.Environment().GetCurrent(ctx, &azdext.EmptyRequest{})
			if err != nil {
				return fmt.Errorf(
					"no azd environment found\n\nRun %s to create a new environment",
					color.CyanString("azd env new"),
				)
			}

			// Get environment values
			envValuesResponse, err := azdClient.Environment().GetValues(ctx, &azdext.GetEnvironmentRequest{
				Name: currentEnvResponse.Environment.Name,
			})
			if err != nil {
				return fmt.Errorf("failed to retrieve environment values: %w", err)
			}

			// Build the environment variable slice
			env := buildEnvironment(envValuesResponse.KeyValues)

			// Execute the command
			exitCode, err := executeCommand(ctx, args[0], args[1:], env)
			if err != nil {
				return err
			}

			// Exit with the same code as the subprocess
			if exitCode != 0 {
				os.Exit(exitCode)
			}

			return nil
		},
	}

	return cmd
} // buildEnvironment creates an environment variable slice by merging the current process environment
// with azd environment values. Azd values take precedence over existing environment variables.
func buildEnvironment(azdValues []*azdext.KeyValue) []string {
	// Start with a copy of the current environment
	envMap := make(map[string]string)

	for _, envVar := range os.Environ() {
		// Parse KEY=VALUE format
		for i := 0; i < len(envVar); i++ {
			if envVar[i] == '=' {
				key := envVar[:i]
				value := envVar[i+1:]
				envMap[key] = value
				break
			}
		}
	}

	// Override/add azd environment values
	for _, pair := range azdValues {
		envMap[pair.Key] = pair.Value
	}

	// Convert map back to slice format
	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeCommand runs the specified command with the provided arguments and environment variables.
// It returns the exit code and any error that occurred during execution.
func executeCommand(ctx context.Context, command string, args []string, env []string) (int, error) {
	// Create the command
	cmd := exec.CommandContext(ctx, command, args...)

	// Set the environment
	cmd.Env = env

	// Forward stdin, stdout, and stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		// Check if it's an exit error to get the exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		// Other errors (e.g., command not found)
		return 1, fmt.Errorf("failed to execute command: %w", err)
	}

	return 0, nil
}
