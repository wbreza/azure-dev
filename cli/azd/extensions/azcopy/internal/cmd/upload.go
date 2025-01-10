package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
)

type uploadFlags struct {
	recursive bool
	source    string
	dest      string
}

func newUploadCommand() *cobra.Command {
	flags := uploadFlags{}

	uploadCmd := &cobra.Command{
		Use:           "upload [options]",
		Short:         "A CLI for managing AI models and services",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			azdSever := os.Getenv("AZD_SERVER")
			azdAccessToken := os.Getenv("AZD_ACCESS_TOKEN")
			ctx := metadata.AppendToOutgoingContext(cmd.Context(), "authorization", azdAccessToken)

			azdClient, err := azdext.NewAzdClient(azdext.WithAddress(azdSever))
			if err != nil {
				return fmt.Errorf("failed to create azd client: %w", err)
			}

			azdClient.Project()

			defer azdClient.Close()

			azureContext := azdext.AzureContext{
				Scope:     &azdext.AzureScope{},
				Resources: []string{},
			}

			if err := ensureAzureScope(ctx, &azureContext, azdClient); err != nil {
				return fmt.Errorf("failed determine azure scope: %w", err)
			}

			promptResourceResponse, err := azdClient.
				Prompt().
				PromptSubscriptionResource(ctx, &azdext.PromptSubscriptionResourceRequest{
					AzureContext: &azureContext,
					Options: &azdext.PromptResourceOptions{
						ResourceType:            "Microsoft.Storage/storageAccounts",
						ResourceTypeDisplayName: "Azure Storage Account",
					},
				})

			if err != nil {
				return fmt.Errorf("failed to prompt Azure Storage Account: %w", err)
			}

			storageAccountName := promptResourceResponse.Resource.Name
			storageAccountUrl := fmt.Sprintf("https://%s.blob.core.windows.net", storageAccountName)

			absSourcePath, err := filepath.Abs(flags.source)
			if err != nil {
				return fmt.Errorf("failed to get absolute path: %w", err)
			}

			azcopyArgs := []string{
				"copy",
				absSourcePath,
				fmt.Sprintf("%s/%s", storageAccountUrl, flags.dest),
			}

			if flags.recursive {
				azcopyArgs = append(azcopyArgs, "--recursive")
			}

			azcopyCmd := exec.Command("azcopy", azcopyArgs...)
			azcopyCmd.Stdout = os.Stdout
			azcopyCmd.Stderr = os.Stderr

			err = azcopyCmd.Run()
			if err != nil {
				return fmt.Errorf("failed to run azcopy: %w", err)
			}

			return nil

		},
	}

	uploadCmd.Flags().BoolVarP(&flags.recursive, "recursive", "r", false, "Recursively upload files")
	uploadCmd.Flags().StringVarP(&flags.source, "source", "s", "", "Source path to upload")
	uploadCmd.Flags().StringVarP(&flags.dest, "dest", "d", "", "Destination path to upload")

	return uploadCmd
}

func ensureAzureScope(ctx context.Context, azureContext *azdext.AzureContext, azdClient *azdext.AzdClient) error {
	// Check if we have an azd environment context
	azdEnvResponse, err := azdClient.Environment().GetCurrent(ctx, &azdext.EmptyRequest{})
	if err != nil {
		// When we have a valid environment, check if we also have a deployment context
		deploymentContextResponse, err := azdClient.Deployment().GetDeploymentContext(ctx, &azdext.EmptyRequest{})
		if err == nil {
			azureContext.Resources = deploymentContextResponse.AzureContext.Resources
		}
	} else {
		// Attempt to pull azure context from the environment variables
		envValuesResponse, err := azdClient.
			Environment().
			GetValues(ctx, &azdext.GetEnvironmentRequest{
				Name: azdEnvResponse.Environment.Name,
			})

		if err != nil {
			return fmt.Errorf("failed to get environment values: %w", err)
		}

		subscriptionId, ok := getValue(envValuesResponse.KeyValues, "AZURE_SUBSCRIPTION_ID")
		if ok {
			azureContext.Scope.SubscriptionId = subscriptionId
		}

		resourceGroup, ok := getValue(envValuesResponse.KeyValues, "AZURE_RESOURCE_GROUP")
		if ok {
			azureContext.Scope.ResourceGroup = resourceGroup
		}

		location, ok := getValue(envValuesResponse.KeyValues, "AZURE_LOCATION")
		if ok {
			azureContext.Scope.Location = location
		}

		tenantId, ok := getValue(envValuesResponse.KeyValues, "AZURE_TENANT_ID")
		if ok {
			azureContext.Scope.TenantId = tenantId
		}
	}

	// If we're still missing values, prompt the user for more context

	if azureContext.Scope.SubscriptionId == "" {
		// Prompt for subscription
		selectedSubscriptionResponse, err := azdClient.Prompt().PromptSubscription(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to prompt subscription: %w", err)
		}

		azureContext.Scope.SubscriptionId = selectedSubscriptionResponse.Subscription.Id
		azureContext.Scope.TenantId = selectedSubscriptionResponse.Subscription.TenantId
	}

	if azureContext.Scope.Location == "" {
		// Prompt for location
		locationRequest := &azdext.PromptLocationRequest{
			AzureContext: azureContext,
		}
		selectedLocationResponse, err := azdClient.Prompt().PromptLocation(ctx, locationRequest)
		if err != nil {
			return fmt.Errorf("failed to prompt location: %w", err)
		}

		azureContext.Scope.Location = selectedLocationResponse.Location.Name
	}

	if azureContext.Scope.ResourceGroup == "" {
		// Prompt for resource group
		resourceGroupRequest := &azdext.PromptResourceGroupRequest{
			AzureContext: azureContext,
		}
		selectedResourceGroupResponse, err := azdClient.Prompt().PromptResourceGroup(ctx, resourceGroupRequest)
		if err != nil {
			return fmt.Errorf("failed to prompt resource group: %w", err)
		}

		azureContext.Scope.ResourceGroup = selectedResourceGroupResponse.ResourceGroup.Name
	}

	return nil
}

func getValue(values []*azdext.KeyValue, key string) (string, bool) {
	for _, value := range values {
		if strings.ToUpper(value.Key) == strings.ToUpper(key) {
			return value.Value, true
		}
	}

	return "", false
}
