package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/azdconfig"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/azdenv"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/azdprompt"
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

			defer conn.Close()

			promptClient := azdprompt.NewPromptServiceClient(conn)
			selectedSubscriptionReply, err := promptClient.PromptSubscription(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to prompt subscription: %v", err)
			}

			fmt.Println("Selected subscription: ", selectedSubscriptionReply.Subscription.Name)

			envClient := azdenv.NewEnvironmentServiceClient(conn)
			currentEnvReply, err := envClient.GetCurrent(ctx, &azdenv.EmptyResponse{})
			if err != nil {
				return fmt.Errorf("failed to get current environment: %v", err)
			}

			fmt.Println("Current environment: ", currentEnvReply.Environment.Name)

			userConfigClient := azdconfig.NewUserConfigServiceClient(conn)
			rootConfigReply, err := userConfigClient.Get(ctx, &azdconfig.GetRequest{Path: ""})
			if err != nil {
				return fmt.Errorf("failed to get root config: %v", err)
			}

			var rootConfig map[string]interface{}
			if err := json.Unmarshal(rootConfigReply.Value, &rootConfig); err != nil {
				return fmt.Errorf("failed to unmarshal root config: %v", err)
			}

			rootConfigJsonBytes, err := json.MarshalIndent(rootConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal root config: %v", err)
			}

			fmt.Println("Root config: \n", string(rootConfigJsonBytes))

			testGetString, err := userConfigClient.GetString(ctx, &azdconfig.GetStringRequest{Path: "extensions.ai.displayName"})
			if err != nil {
				return fmt.Errorf("failed to get string: %v", err)
			}

			fmt.Println("Test get string: ", testGetString.Value)

			values, err := envClient.GetValues(ctx, &azdenv.GetEnvironmentRequest{Name: currentEnvReply.Environment.Name})
			if err != nil {
				return fmt.Errorf("failed to get environment values: %v", err)
			}

			for _, kv := range values.KeyValues {
				fmt.Printf("%s=%s\n", kv.Key, kv.Value)
			}

			envConfigReply, err := envClient.GetConfigSection(ctx, &azdenv.GetConfigSectionRequest{Path: "ai"})
			if err != nil {
				return fmt.Errorf("failed to get environment config: %v", err)
			}

			var envConfig map[string]interface{}
			if err := json.Unmarshal(envConfigReply.Section, &envConfig); err != nil {
				return fmt.Errorf("failed to unmarshal environment config: %v", err)
			}

			jsonBytes, err := json.MarshalIndent(envConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal environment config: %v", err)
			}

			fmt.Println("Environment config: ", string(jsonBytes))

			return nil
		},
	}

	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	return rootCmd
}
