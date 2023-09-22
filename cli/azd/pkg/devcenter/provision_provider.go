package devcenter

import (
	"context"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
)

const (
	ProvisionParametersConfigPath = "provision.parameters"

	DeploymentTagDevCenterName    = "AdeDevCenterName"
	DeploymentTagDevCenterProject = "AdeProjectName"
	DeploymentTagEnvironmentType  = "AdeEnvironmentTypeName"
	DeploymentTagEnvironmentName  = "AdeEnvironmentName"
)

type ProvisionProvider struct {
	console         input.Console
	env             *environment.Environment
	envManager      environment.Manager
	config          *Config
	devCenterClient devcentersdk.DevCenterClient
	manager         *Manager
	prompter        *Prompter
}

func NewDevCenterProvider(
	console input.Console,
	env *environment.Environment,
	envManager environment.Manager,
	config *Config,
	devCenterClient devcentersdk.DevCenterClient,
	manager *Manager,
	prompter *Prompter,
) provisioning.Provider {
	return &ProvisionProvider{
		console:         console,
		env:             env,
		envManager:      envManager,
		config:          config,
		devCenterClient: devCenterClient,
		manager:         manager,
		prompter:        prompter,
	}
}

func (p *ProvisionProvider) Name() string {
	return "Dev Center"
}

func (p *ProvisionProvider) Initialize(ctx context.Context, projectPath string, options provisioning.Options) error {
	return p.EnsureEnv(ctx)
}

func (p *ProvisionProvider) State(
	ctx context.Context,
	options *provisioning.StateOptions,
) (*provisioning.StateResult, error) {
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

	outputs, err := p.manager.Outputs(ctx, environment)
	if err != nil {
		return nil, fmt.Errorf("failed getting environment outputs: %w", err)
	}

	return &provisioning.StateResult{
		State: &provisioning.State{
			Outputs: outputs,
		},
	}, nil
}

func (p *ProvisionProvider) Deploy(ctx context.Context) (*provisioning.DeployResult, error) {
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

	outputs, err := p.manager.Outputs(ctx, environment)
	if err != nil {
		return nil, fmt.Errorf("failed getting environment outputs: %w", err)
	}

	result := &provisioning.DeployResult{
		Deployment: &provisioning.Deployment{
			Parameters: createInputParameters(envDef, paramValues),
			Outputs:    outputs,
		},
	}

	return result, nil
}

func (p *ProvisionProvider) Preview(ctx context.Context) (*provisioning.DeployPreviewResult, error) {
	return nil, fmt.Errorf("preview is not supported for devcenter")
}

func (p *ProvisionProvider) Destroy(
	ctx context.Context,
	options provisioning.DestroyOptions,
) (*provisioning.DestroyResult, error) {
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

	result := &provisioning.DestroyResult{}

	return result, nil
}

// EnsureEnv ensures that the environment is configured for the Dev Center provider.
// Require selection for devcenter, project, catalog, environment type, and environment definition
func (p *ProvisionProvider) EnsureEnv(ctx context.Context) error {
	currentConfig := *p.config
	updatedConfig, err := p.manager.Initialize(ctx)
	if err != nil {
		return err
	}

	envTypeName := p.config.EnvironmentType
	if envTypeName == "" {
		envTypeName, err = p.prompter.PromptEnvironmentType(ctx, updatedConfig.Name, updatedConfig.Project)
		if err != nil {
			return err
		}
		p.config.EnvironmentType = envTypeName
	}

	if currentConfig.Name == "" {
		if err := p.env.Config.Set(DevCenterNamePath, updatedConfig.Name); err != nil {
			return err
		}
	}

	if currentConfig.Project == "" {
		if err := p.env.Config.Set(DevCenterProjectPath, updatedConfig.Project); err != nil {
			return err
		}
	}

	if currentConfig.EnvironmentType == "" {
		if err := p.env.Config.Set(DevCenterEnvTypePath, updatedConfig.EnvironmentType); err != nil {
			return err
		}
	}

	if currentConfig.EnvironmentDefinition == "" {
		if err := p.env.Config.Set(DevCenterEnvDefinitionPath, updatedConfig.EnvironmentDefinition); err != nil {
			return err
		}
	}

	return nil
}

func mapBicepTypeToInterfaceType(s string) provisioning.ParameterType {
	switch s {
	case "String", "string", "secureString", "securestring":
		return provisioning.ParameterTypeString
	case "Bool", "bool":
		return provisioning.ParameterTypeBoolean
	case "Int", "int":
		return provisioning.ParameterTypeNumber
	case "Object", "object", "secureObject", "secureobject":
		return provisioning.ParameterTypeObject
	case "Array", "array":
		return provisioning.ParameterTypeArray
	default:
		panic(fmt.Sprintf("unexpected bicep type: '%s'", s))
	}
}

// Creates a normalized view of the azure output parameters and resolves inconsistencies in the output parameter name
// casings.
func createOutputParameters(
	deploymentOutputs map[string]azapi.AzCliDeploymentOutput,
) map[string]provisioning.OutputParameter {
	outputParams := map[string]provisioning.OutputParameter{}

	for key, azureParam := range deploymentOutputs {
		// To support BYOI (bring your own infrastructure) scenarios we will default to UPPER when canonical casing
		// is not found in the parameters file to workaround strange azure behavior with OUTPUT values that look
		// like `azurE_RESOURCE_GROUP`
		paramName := strings.ToUpper(key)

		outputParams[paramName] = provisioning.OutputParameter{
			Type:  mapBicepTypeToInterfaceType(azureParam.Type),
			Value: azureParam.Value,
		}
	}

	return outputParams
}

func createInputParameters(
	environmentDefinition *devcentersdk.EnvironmentDefinition,
	parameterValues map[string]any,
) map[string]provisioning.InputParameter {
	inputParams := map[string]provisioning.InputParameter{}

	for _, param := range environmentDefinition.Parameters {
		inputParams[param.Name] = provisioning.InputParameter{
			Type:         string(param.Type),
			DefaultValue: param.Default,
			Value:        parameterValues[param.Name],
		}
	}

	return inputParams
}
