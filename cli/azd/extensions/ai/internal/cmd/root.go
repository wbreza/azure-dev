package cmd

import (
	"fmt"
	"os"

	azdext "github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/grpc"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "azd ai <group> [options]",
		Short:         "A CLI for managing AI models and services",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			fmt.Println("Start AI command")

			azdServerAddress := os.Getenv("AZD_SERVER")
			conn, err := grpc.NewClient(azdServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				return fmt.Errorf("failed to connect to server: %v", err)
			}

			promptClient := azdext.NewPromptServiceClient(conn)
			selectedSubscriptionReply, err := promptClient.PromptSubscription(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to prompt subscription: %v", err)
			}

			fmt.Println("Selected subscription: ", selectedSubscriptionReply.Subscription.Name)

			envClient := azdext.NewEnvironmentServiceClient(conn)
			currentEnvReply, err := envClient.GetCurrent(ctx, &azdext.Empty{})
			if err != nil {
				return fmt.Errorf("failed to get current environment: %v", err)
			}

			fmt.Println("Current environment: ", currentEnvReply.Environment.Name)

			values, err := envClient.GetValues(ctx, &azdext.GetEnvironmentRequest{Name: currentEnvReply.Environment.Name})
			if err != nil {
				return fmt.Errorf("failed to get environment values: %v", err)
			}

			for _, kv := range values.KeyValues {
				fmt.Printf("%s=%s\n", kv.Key, kv.Value)
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	return rootCmd
}
