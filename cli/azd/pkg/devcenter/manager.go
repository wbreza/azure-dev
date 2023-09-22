package devcenter

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/infra"
	"github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"golang.org/x/exp/slices"
)

type Manager struct {
	config               *Config
	prompter             *Prompter
	deploymentsService   azapi.Deployments
	deploymentOperations azapi.DeploymentOperations
}

func NewManager(
	config *Config,
	prompter *Prompter,
	deploymentsService azapi.Deployments,
	deploymentOperations azapi.DeploymentOperations,
) *Manager {
	return &Manager{
		config:               config,
		prompter:             prompter,
		deploymentsService:   deploymentsService,
		deploymentOperations: deploymentOperations,
	}
}

func (m *Manager) Initialize(ctx context.Context) (*Config, error) {
	devCenterName := m.config.Name
	var err error

	if devCenterName == "" {
		devCenterName, err = m.prompter.PromptDevCenter(ctx)
		if err != nil {
			return nil, err
		}
		m.config.Name = devCenterName
	}

	projectName := m.config.Project
	if projectName == "" {
		projectName, err = m.prompter.PromptProject(ctx, devCenterName)
		if err != nil {
			return nil, err
		}
		m.config.Project = projectName
	}

	envDefinitionName := m.config.EnvironmentDefinition
	if envDefinitionName == "" {
		envDefinition, err := m.prompter.PromptEnvironmentDefinition(ctx, devCenterName, projectName)
		if err != nil {
			return nil, err
		}
		envDefinitionName = envDefinition.Name
		m.config.Catalog = envDefinition.CatalogName
		m.config.EnvironmentDefinition = envDefinitionName
	}

	return m.config, nil
}

// getEnvironmentOutputs gets the outputs for the latest deployment of the specified environment
// Right now this will retrieve the outputs from the latest azure deployment
// Long term this will call into ADE Outputs API
func (m *Manager) Outputs(
	ctx context.Context,
	env *devcentersdk.Environment,
) (map[string]provisioning.OutputParameter, error) {
	resourceGroupId, err := devcentersdk.NewResourceGroupId(env.ResourceGroupId)
	if err != nil {
		return nil, fmt.Errorf("failed parsing resource group id: %w", err)
	}

	scope := infra.NewResourceGroupScope(
		m.deploymentsService,
		m.deploymentOperations,
		resourceGroupId.SubscriptionId,
		resourceGroupId.Name,
	)

	deployments, err := scope.ListDeployments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed listing deployments: %w", err)
	}

	slices.SortFunc(deployments, func(x, y *armresources.DeploymentExtended) bool {
		return x.Properties.Timestamp.After(*y.Properties.Timestamp)
	})

	latestDeploymentIndex := slices.IndexFunc(deployments, func(d *armresources.DeploymentExtended) bool {
		tagDevCenterName, devCenterOk := d.Tags[DeploymentTagDevCenterName]
		tagProjectName, projectOk := d.Tags[DeploymentTagDevCenterProject]
		tagEnvTypeName, envTypeOk := d.Tags[DeploymentTagEnvironmentType]
		tagEnvName, envOk := d.Tags[DeploymentTagEnvironmentName]

		if !devCenterOk || !projectOk || !envTypeOk || !envOk {
			return false
		}

		if *tagDevCenterName == m.config.Name ||
			*tagProjectName == m.config.Project ||
			*tagEnvTypeName == m.config.EnvironmentType ||
			*tagEnvName == env.Name {
			return true
		}

		return false
	})

	if latestDeploymentIndex == -1 {
		return nil, fmt.Errorf("failed to find latest deployment")
	}

	latestDeployment := deployments[latestDeploymentIndex]
	outputs := createOutputParameters(azapi.CreateDeploymentOutput(latestDeployment.Properties.Outputs))

	// Set up AZURE_SUBSCRIPTION_ID and AZURE_RESOURCE_GROUP environment variables
	// These are required for azd deploy to work as expected
	if _, exists := outputs[environment.SubscriptionIdEnvVarName]; !exists {
		outputs[environment.SubscriptionIdEnvVarName] = provisioning.OutputParameter{
			Type:  provisioning.ParameterTypeString,
			Value: resourceGroupId.SubscriptionId,
		}
	}

	if _, exists := outputs[environment.ResourceGroupEnvVarName]; !exists {
		outputs[environment.ResourceGroupEnvVarName] = provisioning.OutputParameter{
			Type:  provisioning.ParameterTypeString,
			Value: resourceGroupId.Name,
		}
	}

	return outputs, nil
}
