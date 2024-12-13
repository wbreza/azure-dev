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
			fmt.Println("Hello from Go!")
			fmt.Println()

			azdClient, err := azdext.NewAzdClient(azdext.WithAddress(os.Getenv("AZD_SERVER")))
			if err != nil {
				return fmt.Errorf("failed to create azd client: %w", err)
			}

			defer azdClient.Close()

			deploymentContextReply, err := azdClient.Deployment().GetDeploymentContext(ctx, &azdext.EmptyRequest{})
			if err != nil {
				return fmt.Errorf("failed to get deployment context: %w", err)
			}

			azureContext := deploymentContextReply.AzureContext

			nameReply, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Required:    true,
					Message:     "What is your name?",
					HelpMessage: "This is a help message",
					Hint:        "This is a hint",
				},
			})
			if err != nil {
				return fmt.Errorf("failed to prompt: %w", err)
			}

			fmt.Println("Hello, ", nameReply.Value)

			selectedSubscriptionReply, err := azdClient.Prompt().PromptSubscription(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to prompt subscription: %w", err)
			}

			azureContext.Scope.SubscriptionId = selectedSubscriptionReply.Subscription.Id

			fmt.Println("Selected subscription: ", selectedSubscriptionReply.Subscription.Name)

			selectedLocationReply, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: azureContext,
			})
			if err != nil {
				return fmt.Errorf("failed to prompt location: %w", err)
			}

			azureContext.Scope.Location = selectedLocationReply.Location.Name

			fmt.Println("Selected location: ", selectedLocationReply.Location.Name)

			selectedResourceGroupReply, err := azdClient.Prompt().
				PromptResourceGroup(ctx, &azdext.PromptResourceGroupRequest{
					AzureContext: azureContext,
				})
			if err != nil {
				return fmt.Errorf("failed to prompt resource group: %w", err)
			}

			azureContext.Scope.ResourceGroup = selectedResourceGroupReply.ResourceGroup.Name

			fmt.Println("Selected resource group: ", selectedResourceGroupReply.ResourceGroup.Name)

			aiServicePrompt, err := azdClient.Prompt().PromptSubscriptionResource(ctx, &azdext.PromptSubscriptionResourceRequest{
				AzureContext: azureContext,
				Options: &azdext.PromptResourceOptions{
					ResourceType:            "Microsoft.CognitiveServices/accounts",
					Kinds:                   []string{"OpenAI", "AIServices", "CognitiveServices"},
					ResourceTypeDisplayName: "Azure AI Service",
				},
			})
			if err != nil {
				return fmt.Errorf("failed to prompt AI service: %w", err)
			}

			fmt.Println("Selected AI service: ", aiServicePrompt.Resource.Name)

			projectReply, err := azdClient.Project().Get(ctx, &azdext.EmptyRequest{})
			if err != nil {
				return fmt.Errorf("failed to get project: %w", err)
			}

			fmt.Println("Project: ", projectReply.Project.Name)

			currentEnvReply, err := azdClient.Environment().GetCurrent(ctx, &azdext.EmptyRequest{})
			if err != nil {
				return fmt.Errorf("failed to get current environment: %w", err)
			}

			fmt.Println("Current environment: ", currentEnvReply.Environment.Name)

			rootConfigReply, err := azdClient.UserConfig().Get(ctx, &azdext.GetRequest{Path: ""})
			if err != nil {
				return fmt.Errorf("failed to get root config: %w", err)
			}

			var rootConfig map[string]interface{}
			if err := json.Unmarshal(rootConfigReply.Value, &rootConfig); err != nil {
				return fmt.Errorf("failed to unmarshal root config: %w", err)
			}

			rootConfigJsonBytes, err := json.MarshalIndent(rootConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal root config: %w", err)
			}

			fmt.Println("Root config: \n", string(rootConfigJsonBytes))

			testGetString, err := azdClient.UserConfig().
				GetString(ctx, &azdext.GetStringRequest{Path: "extensions.ai.displayName"})
			if err != nil {
				return fmt.Errorf("failed to get string: %w", err)
			}

			fmt.Println("Test get string: ", testGetString.Value)

			values, err := azdClient.Environment().
				GetValues(ctx, &azdext.GetEnvironmentRequest{Name: currentEnvReply.Environment.Name})
			if err != nil {
				return fmt.Errorf("failed to get environment values: %w", err)
			}

			for _, kv := range values.KeyValues {
				fmt.Printf("%s=%s\n", kv.Key, kv.Value)
			}

			envConfigReply, err := azdClient.Environment().GetConfigSection(ctx, &azdext.GetConfigSectionRequest{Path: "ai"})
			if err != nil {
				return fmt.Errorf("failed to get environment config: %w", err)
			}

			if envConfigReply.Found {
				var envConfig map[string]interface{}
				if err := json.Unmarshal(envConfigReply.Section, &envConfig); err != nil {
					return fmt.Errorf("failed to unmarshal environment config: %w", err)
				}

				jsonBytes, err := json.MarshalIndent(envConfig, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal environment config: %w", err)
				}

				fmt.Println("Environment config: ", string(jsonBytes))
			} else {
				fmt.Println("Environment config not found")
			}

			return nil
		},
	}

	//rootCmd.AddCommand(newChatCommand())
	// rootCmd.AddCommand(newModelCommand())
	// rootCmd.AddCommand(newServiceCommand())
	// rootCmd.AddCommand(newChatCommand())
	// rootCmd.AddCommand(newDocumentCommand())
	// rootCmd.AddCommand(newEmbeddingCommand())
	// rootCmd.AddCommand(newIndexCommand())
	// rootCmd.AddCommand(newEvaluateCommand())
	rootCmd.AddCommand(newVersionCommand())

	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	return rootCmd
}
