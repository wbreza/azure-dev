package devcenter

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/devcenter"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	. "github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
)

type DevCenterProvider struct {
	console         input.Console
	env             *environment.Environment
	envManager      environment.Manager
	config          *devcenter.Config
	devCenterClient devcentersdk.DevCenterClient
	prompter        *Prompter
}

func NewDevCenterProvider(
	console input.Console,
	env *environment.Environment,
	envManager environment.Manager,
	config *devcenter.Config,
	devCenterClient devcentersdk.DevCenterClient,
	prompter *Prompter,
) Provider {
	return &DevCenterProvider{
		console:         console,
		env:             env,
		envManager:      envManager,
		config:          config,
		devCenterClient: devCenterClient,
		prompter:        prompter,
	}
}

func (p *DevCenterProvider) Name() string {
	return "Dev Center"
}

func (p *DevCenterProvider) Initialize(ctx context.Context, projectPath string, options Options) error {
	return p.EnsureEnv(ctx)
}

func (p *DevCenterProvider) State(ctx context.Context, options *StateOptions) (*StateResult, error) {
	result := &StateResult{
		State: &State{},
	}

	return result, nil
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
		path := fmt.Sprintf("provision.%s", key)
		if err := p.env.Config.Set(path, value); err != nil {
			return nil, fmt.Errorf("failed setting config value %s: %w", path, err)
		}
	}

	if err := p.envManager.Save(ctx, p.env); err != nil {
		return nil, fmt.Errorf("failed saving environment: %w", err)
	}

	envName := p.env.GetEnvName()

	envSpec := devcentersdk.EnvironmentSpec{
		CatalogName:               p.config.Catalog,
		EnvironmentType:           p.config.EnvironmentType,
		EnvironmentDefinitionName: p.config.EnvironmentDefinition,
		Parameters:                paramValues,
	}

	spinnerMessage := fmt.Sprintf("Creating devcenter environment %s", envName)
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

	result := &DeployResult{
		Deployment: &Deployment{},
	}

	return result, nil
}

func (p *DevCenterProvider) Preview(ctx context.Context) (*DeployPreviewResult, error) {
	result := &DeployPreviewResult{
		Preview: &DeploymentPreview{},
	}

	return result, nil
}

func (p *DevCenterProvider) Destroy(ctx context.Context, options DestroyOptions) (*DestroyResult, error) {
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
		p.env.Config.Set(devcenter.DevCenterNamePath, devCenterName)
	}

	projectName := p.config.Project
	if projectName == "" {
		projectName, err = p.prompter.PromptProject(ctx, devCenterName)
		if err != nil {
			return err
		}
		p.config.Project = projectName
		p.env.Config.Set(devcenter.DevCenterProjectPath, projectName)
	}

	catalogName := p.config.Catalog
	if catalogName == "" {
		catalogName, err = p.prompter.PromptCatalog(ctx, devCenterName, projectName)
		if err != nil {
			return err
		}
		p.config.Catalog = catalogName
		p.env.Config.Set(devcenter.DevCenterCatalogPath, catalogName)
	}

	envTypeName := p.config.EnvironmentType
	if envTypeName == "" {
		envTypeName, err = p.prompter.PromptEnvironmentType(ctx, devCenterName, projectName)
		if err != nil {
			return err
		}
		p.config.EnvironmentType = envTypeName
		p.env.Config.Set(devcenter.DevCenterEnvTypePath, envTypeName)
	}

	envDefinitionName := p.config.EnvironmentDefinition
	if envDefinitionName == "" {
		envDefinitionName, err = p.prompter.PromptEnvironmentDefinition(ctx, devCenterName, projectName)
		if err != nil {
			return err
		}
		p.config.EnvironmentDefinition = envDefinitionName
		p.env.Config.Set(devcenter.DevCenterEnvDefinitionPath, envDefinitionName)
	}

	if err := p.envManager.Save(ctx, p.env); err != nil {
		return fmt.Errorf("failed saving environment: %w", err)
	}

	return nil
}
