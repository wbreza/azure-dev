package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
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

			azdClient, err := azdext.NewAzdClient(os.Getenv("AZD_SERVER"))
			if err != nil {
				return fmt.Errorf("failed to create azd client: %v", err)
			}

			defer azdClient.Close()

			azureContext := &azdext.AzureContext{
				Scope: &azdext.AzureScope{},
			}

			selectedSubscriptionReply, err := azdClient.Prompt().PromptSubscription(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to prompt subscription: %v", err)
			}

			azureContext.Scope.SubscriptionId = selectedSubscriptionReply.Subscription.Id

			fmt.Println("Selected subscription: ", selectedSubscriptionReply.Subscription.Name)

			selectedLocationReply, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: azureContext,
			})
			if err != nil {
				return fmt.Errorf("failed to prompt location: %v", err)
			}

			azureContext.Scope.Location = selectedLocationReply.Location.Name

			fmt.Println("Selected location: ", selectedLocationReply.Location.Name)

			selectedResourceGroupReply, err := azdClient.Prompt().PromptResourceGroup(ctx, &azdext.PromptResourceGroupRequest{
				AzureContext: azureContext,
			})
			if err != nil {
				return fmt.Errorf("failed to prompt resource group: %v", err)
			}

			azureContext.Scope.ResourceGroup = selectedResourceGroupReply.ResourceGroup.Name

			fmt.Println("Selected resource group: ", selectedResourceGroupReply.ResourceGroup.Name)

			projectReply, err := azdClient.Project().Get(ctx, &azdext.EmptyRequest{})
			if err != nil {
				return fmt.Errorf("failed to get project: %v", err)
			}

			fmt.Println("Project: ", projectReply.Project.Name)

			currentEnvReply, err := azdClient.Environment().GetCurrent(ctx, &azdext.EmptyResponse{})
			if err != nil {
				return fmt.Errorf("failed to get current environment: %v", err)
			}

			fmt.Println("Current environment: ", currentEnvReply.Environment.Name)

			rootConfigReply, err := azdClient.UserConfig().Get(ctx, &azdext.GetRequest{Path: ""})
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

			testGetString, err := azdClient.UserConfig().GetString(ctx, &azdext.GetStringRequest{Path: "extensions.ai.displayName"})
			if err != nil {
				return fmt.Errorf("failed to get string: %v", err)
			}

			fmt.Println("Test get string: ", testGetString.Value)

			values, err := azdClient.Environment().GetValues(ctx, &azdext.GetEnvironmentRequest{Name: currentEnvReply.Environment.Name})
			if err != nil {
				return fmt.Errorf("failed to get environment values: %v", err)
			}

			for _, kv := range values.KeyValues {
				fmt.Printf("%s=%s\n", kv.Key, kv.Value)
			}

			envConfigReply, err := azdClient.Environment().GetConfigSection(ctx, &azdext.GetConfigSectionRequest{Path: "ai"})
			if err != nil {
				return fmt.Errorf("failed to get environment config: %v", err)
			}

			if envConfigReply.Found {
				var envConfig map[string]interface{}
				if err := json.Unmarshal(envConfigReply.Section, &envConfig); err != nil {
					return fmt.Errorf("failed to unmarshal environment config: %v", err)
				}

				jsonBytes, err := json.MarshalIndent(envConfig, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal environment config: %v", err)
				}

				fmt.Println("Environment config: ", string(jsonBytes))
			} else {
				fmt.Println("Environment config not found")
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	return rootCmd
}
