package devcenter

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/devcenter"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/infra"
	. "github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"golang.org/x/exp/slices"
)

const (
	ProvisionParametersConfigPath = "provision.parameters"

	DeploymentTagDevCenterName    = "AdeDevCenterName"
	DeploymentTagDevCenterProject = "AdeProjectName"
	DeploymentTagEnvironmentType  = "AdeEnvironmentTypeName"
	DeploymentTagEnvironmentName  = "AdeEnvironmentName"
)

type DevCenterProvider struct {
	console              input.Console
	env                  *environment.Environment
	envManager           environment.Manager
	config               *devcenter.Config
	devCenterClient      devcentersdk.DevCenterClient
	deploymentsService   azapi.Deployments
	deploymentOperations azapi.DeploymentOperations
	prompter             *Prompter
}

func NewDevCenterProvider(
	console input.Console,
	env *environment.Environment,
	envManager environment.Manager,
	config *devcenter.Config,
	devCenterClient devcentersdk.DevCenterClient,
	deploymentsService azapi.Deployments,
	deploymentOperations azapi.DeploymentOperations,
	prompter *Prompter,
) Provider {
	return &DevCenterProvider{
		console:              console,
		env:                  env,
		envManager:           envManager,
		config:               config,
		devCenterClient:      devCenterClient,
		deploymentsService:   deploymentsService,
		deploymentOperations: deploymentOperations,
		prompter:             prompter,
	}
}

func (p *DevCenterProvider) Name() string {
	return "Dev Center"
}

func (p *DevCenterProvider) Initialize(ctx context.Context, projectPath string, options Options) error {
	return p.EnsureEnv(ctx)
}

func (p *DevCenterProvider) State(ctx context.Context, options *StateOptions) (*StateResult, error) {
	if !p.config.IsValid() {
		return nil, fmt.Errorf("invalid devcenter configuration")
	}

	envName := p.env.GetEnvName()
	environment, err := p.devCenterClient.
		DevCenterByName(p.config.Name).
		ProjectByName(p.config.Project).
		EnvironmentByName(envName).
		Get(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed getting environment: %w", err)
	}

	outputs, err := p.getEnvironmentOutputs(ctx, environment)
	if err != nil {
		return nil, fmt.Errorf("failed getting environment outputs: %w", err)
	}

	return &StateResult{
		State: &State{
			Outputs: outputs,
		},
	}, nil
}

func (p *DevCenterProvider) Deploy(ctx context.Context) (*DeployResult, error) {
	if !p.config.IsValid() {
		return nil, fmt.Errorf("invalid devcenter configuration")
	}

	envDef, err := p.devCenterClient.
		DevCenterByName(p.config.Name).
		ProjectByName(p.config.Project).
		CatalogByName(p.config.Catalog).
		EnvironmentDefinitionByName(p.config.EnvironmentDefinition).
		Get(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed getting environment definition: %w", err)
	}

	paramValues, err := p.prompter.PromptParameters(ctx, p.env, envDef)
	if err != nil {
		return nil, fmt.Errorf("failed prompting for parameters: %w", err)
	}

	for key, value := range paramValues {
		path := fmt.Sprintf("%s.%s", ProvisionParametersConfigPath, key)
		if err := p.env.Config.Set(path, value); err != nil {
			return nil, fmt.Errorf("failed setting config value %s: %w", path, err)
		}
	}

	if err := p.envManager.Save(ctx, p.env); err != nil {
		return nil, fmt.Errorf("failed saving environment: %w", err)
	}

	envName := p.env.GetEnvName()
	spinnerMessage := fmt.Sprintf("Creating devcenter environment %s", output.WithHighLightFormat(envName))

	envSpec := devcentersdk.EnvironmentSpec{
		CatalogName:               p.config.Catalog,
		EnvironmentType:           p.config.EnvironmentType,
		EnvironmentDefinitionName: p.config.EnvironmentDefinition,
		Parameters:                paramValues,
	}

	p.console.ShowSpinner(ctx, spinnerMessage, input.Step)

	poller, err := p.devCenterClient.
		DevCenterByName(p.config.Name).
		ProjectByName(p.config.Project).
		EnvironmentByName(envName).
		BeginPut(ctx, envSpec)

	if err != nil {
		p.console.StopSpinner(ctx, spinnerMessage, input.StepFailed)
		return nil, fmt.Errorf("failed creating environment: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		p.console.StopSpinner(ctx, spinnerMessage, input.StepFailed)
		return nil, fmt.Errorf("failed creating environment: %w", err)
	}

	p.console.StopSpinner(ctx, spinnerMessage, input.StepDone)

	environment, err := p.devCenterClient.
		DevCenterByName(p.config.Name).
		ProjectByName(p.config.Project).
		EnvironmentByName(envName).
		Get(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed getting environment: %w", err)
	}

	outputs, err := p.getEnvironmentOutputs(ctx, environment)
	if err != nil {
		return nil, fmt.Errorf("failed getting environment outputs: %w", err)
	}

	result := &DeployResult{
		Deployment: &Deployment{
			Parameters: createInputParameters(envDef, paramValues),
			Outputs:    outputs,
		},
	}

	return result, nil
}

func (p *DevCenterProvider) Preview(ctx context.Context) (*DeployPreviewResult, error) {
	return nil, fmt.Errorf("preview is not supported for devcenter")
}

func (p *DevCenterProvider) Destroy(ctx context.Context, options DestroyOptions) (*DestroyResult, error) {
	if !p.config.IsValid() {
		return nil, fmt.Errorf("invalid devcenter configuration")
	}

	envName := p.env.GetEnvName()
	spinnerMessage := fmt.Sprintf("Deleting devcenter environment %s", output.WithHighLightFormat(envName))

	if !options.Force() {
		warningMessage := output.WithWarningFormat(
			"WARNING: This will delete the following Dev Center environment and all of its resources:\n",
		)
		p.console.Message(ctx, warningMessage)

		p.console.Message(ctx, fmt.Sprintf("Dev Center: %s", output.WithHighLightFormat(p.config.Name)))
		p.console.Message(ctx, fmt.Sprintf("Project: %s", output.WithHighLightFormat(p.config.Project)))
		p.console.Message(ctx, fmt.Sprintf("Environment Type: %s", output.WithHighLightFormat(p.config.EnvironmentType)))
		p.console.Message(ctx,
			fmt.Sprintf("Environment Definition: %s", output.WithHighLightFormat(p.config.EnvironmentDefinition)),
		)
		p.console.Message(ctx, fmt.Sprintf("Environment: %s\n", output.WithHighLightFormat(envName)))

		confirm, err := p.console.Confirm(ctx, input.ConsoleOptions{
			Message:      "Are you sure you want to continue?",
			DefaultValue: false,
		})

		if err != nil {
			p.console.Message(ctx, "")
			p.console.ShowSpinner(ctx, spinnerMessage, input.Step)
			p.console.StopSpinner(ctx, spinnerMessage, input.StepFailed)
			return nil, fmt.Errorf("destroy operation interrupted: %w", err)
		}

		p.console.Message(ctx, "\n")

		if !confirm {
			p.console.ShowSpinner(ctx, spinnerMessage, input.Step)
			p.console.StopSpinner(ctx, spinnerMessage, input.StepSkipped)
			return nil, fmt.Errorf("destroy operation cancelled")
		}
	}

	p.console.ShowSpinner(ctx, spinnerMessage, input.Step)

	poller, err := p.devCenterClient.
		DevCenterByName(p.config.Name).
		ProjectByName(p.config.Project).
		EnvironmentByName(envName).
		BeginDelete(ctx)

	if err != nil {
		p.console.StopSpinner(ctx, spinnerMessage, input.StepFailed)
		return nil, fmt.Errorf("failed deleting environment: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		p.console.StopSpinner(ctx, spinnerMessage, input.StepFailed)
		return nil, fmt.Errorf("failed deleting environment: %w", err)
	}

	p.console.StopSpinner(ctx, spinnerMessage, input.StepDone)

	result := &DestroyResult{}

	return result, nil
}

// EnsureEnv ensures that the environment is configured for the Dev Center provider.
// Require selection for devcenter, project, catalog, environment type, and environment definition
func (p *DevCenterProvider) EnsureEnv(ctx context.Context) error {
	devCenterName := p.config.Name
	var err error

	if devCenterName == "" {
		devCenterName, err = p.prompter.PromptDevCenter(ctx)
		if err != nil {
			return err
		}
		p.config.Name = devCenterName
		if err := p.env.Config.Set(devcenter.DevCenterNamePath, devCenterName); err != nil {
			return err
		}
	}

	projectName := p.config.Project
	if projectName == "" {
		projectName, err = p.prompter.PromptProject(ctx, devCenterName)
		if err != nil {
			return err
		}
		p.config.Project = projectName
		if err := p.env.Config.Set(devcenter.DevCenterProjectPath, projectName); err != nil {
			return err
		}
	}

	catalogName := p.config.Catalog
	if catalogName == "" {
		catalogName, err = p.prompter.PromptCatalog(ctx, devCenterName, projectName)
		if err != nil {
			return err
		}
		p.config.Catalog = catalogName
		if err := p.env.Config.Set(devcenter.DevCenterCatalogPath, catalogName); err != nil {
			return err
		}
	}

	envTypeName := p.config.EnvironmentType
	if envTypeName == "" {
		envTypeName, err = p.prompter.PromptEnvironmentType(ctx, devCenterName, projectName)
		if err != nil {
			return err
		}
		p.config.EnvironmentType = envTypeName
		if err := p.env.Config.Set(devcenter.DevCenterEnvTypePath, envTypeName); err != nil {
			return err
		}
	}

	envDefinitionName := p.config.EnvironmentDefinition
	if envDefinitionName == "" {
		envDefinitionName, err = p.prompter.PromptEnvironmentDefinition(ctx, devCenterName, projectName)
		if err != nil {
			return err
		}
		p.config.EnvironmentDefinition = envDefinitionName
		if err := p.env.Config.Set(devcenter.DevCenterEnvDefinitionPath, envDefinitionName); err != nil {
			return err
		}
	}

	return nil
}

// getEnvironmentOutputs gets the outputs for the latest deployment of the specified environment
// Right now this will retrieve the outputs from the latest azure deployment
// Long term this will call into ADE Outputs API
func (p *DevCenterProvider) getEnvironmentOutputs(
	ctx context.Context,
	env *devcentersdk.Environment,
) (map[string]OutputParameter, error) {
	resourceGroupId, err := devcentersdk.NewResourceGroupId(env.ResourceGroupId)
	if err != nil {
		return nil, fmt.Errorf("failed parsing resource group id: %w", err)
	}

	scope := infra.NewResourceGroupScope(
		p.deploymentsService,
		p.deploymentOperations,
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

		if *tagDevCenterName == p.config.Name ||
			*tagProjectName == p.config.Project ||
			*tagEnvTypeName == p.config.EnvironmentType ||
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
		outputs[environment.SubscriptionIdEnvVarName] = OutputParameter{
			Type:  ParameterTypeString,
			Value: resourceGroupId.SubscriptionId,
		}
	}

	if _, exists := outputs[environment.ResourceGroupEnvVarName]; !exists {
		outputs[environment.ResourceGroupEnvVarName] = OutputParameter{
			Type:  ParameterTypeString,
			Value: resourceGroupId.Name,
		}
	}

	return outputs, nil
}

func mapBicepTypeToInterfaceType(s string) ParameterType {
	switch s {
	case "String", "string", "secureString", "securestring":
		return ParameterTypeString
	case "Bool", "bool":
		return ParameterTypeBoolean
	case "Int", "int":
		return ParameterTypeNumber
	case "Object", "object", "secureObject", "secureobject":
		return ParameterTypeObject
	case "Array", "array":
		return ParameterTypeArray
	default:
		panic(fmt.Sprintf("unexpected bicep type: '%s'", s))
	}
}

// Creates a normalized view of the azure output parameters and resolves inconsistencies in the output parameter name
// casings.
func createOutputParameters(deploymentOutputs map[string]azapi.AzCliDeploymentOutput) map[string]OutputParameter {
	outputParams := map[string]OutputParameter{}

	for key, azureParam := range deploymentOutputs {
		// To support BYOI (bring your own infrastructure) scenarios we will default to UPPER when canonical casing
		// is not found in the parameters file to workaround strange azure behavior with OUTPUT values that look
		// like `azurE_RESOURCE_GROUP`
		paramName := strings.ToUpper(key)

		outputParams[paramName] = OutputParameter{
			Type:  mapBicepTypeToInterfaceType(azureParam.Type),
			Value: azureParam.Value,
		}
	}

	return outputParams
}

func createInputParameters(
	environmentDefinition *devcentersdk.EnvironmentDefinition,
	parameterValues map[string]any,
) map[string]InputParameter {
	inputParams := map[string]InputParameter{}

	for _, param := range environmentDefinition.Parameters {
		inputParams[param.Name] = InputParameter{
			Type:         string(param.Type),
			DefaultValue: param.Default,
			Value:        parameterValues[param.Name],
		}
	}

	return inputParams
}
