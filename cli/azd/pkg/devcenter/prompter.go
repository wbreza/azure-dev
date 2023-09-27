package devcenter

import (
	"context"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"golang.org/x/exp/slices"
)

type Prompter struct {
	config          *Config
	console         input.Console
	manager         *Manager
	devCenterClient devcentersdk.DevCenterClient
}

func NewPrompter(
	config *Config,
	console input.Console,
	manager *Manager,
	devCenterClient devcentersdk.DevCenterClient,
) *Prompter {
	return &Prompter{
		config:          config,
		console:         console,
		manager:         manager,
		devCenterClient: devCenterClient,
	}
}

func (p *Prompter) PromptForValues(ctx context.Context) (*Config, error) {
	devCenterName := p.config.Name
	if devCenterName == "" {
		devCenter, err := p.PromptDevCenter(ctx)
		if err != nil {
			return nil, err
		}
		p.config.Name = devCenter.Name
		devCenterName = devCenter.Name
	}

	projectName := p.config.Project
	if projectName == "" {
		project, err := p.PromptProject(ctx, devCenterName)
		if err != nil {
			return nil, err
		}
		p.config.Project = project.Name
		projectName = project.Name
	}

	envDefinitionName := p.config.EnvironmentDefinition
	if envDefinitionName == "" {
		envDefinition, err := p.PromptEnvironmentDefinition(ctx, devCenterName, projectName)
		if err != nil {
			return nil, err
		}
		envDefinitionName = envDefinition.Name
		p.config.Catalog = envDefinition.CatalogName
		p.config.EnvironmentDefinition = envDefinitionName
	}

	return p.config, nil
}

func (p *Prompter) PromptDevCenter(ctx context.Context) (*devcentersdk.DevCenter, error) {
	devCenters := []*devcentersdk.DevCenter{}
	writeableProjects, err := p.manager.WritableProjects(ctx)
	if err != nil {
		return nil, err
	}

	for _, project := range writeableProjects {
		containsDevCenter := slices.ContainsFunc(devCenters, func(dc *devcentersdk.DevCenter) bool {
			return dc.ServiceUri == project.DevCenter.ServiceUri
		})

		if !containsDevCenter {
			devCenters = append(devCenters, project.DevCenter)
		}
	}

	slices.SortFunc(devCenters, func(x, y *devcentersdk.DevCenter) bool {
		return x.Name < y.Name
	})

	devCenterNames := []string{}
	for _, devCenter := range devCenters {
		devCenterNames = append(devCenterNames, devCenter.Name)
	}

	if len(devCenterNames) == 1 {
		return devCenters[0], nil
	}

	selected, err := p.console.Select(ctx, input.ConsoleOptions{
		Message: "Select a Dev Center:",
		Options: devCenterNames,
	})

	if err != nil {
		return nil, err
	}

	return devCenters[selected], nil
}

func (p *Prompter) PromptCatalog(
	ctx context.Context,
	devCenterName string,
	projectName string,
) (*devcentersdk.Catalog, error) {
	catalogsResponse, err := p.devCenterClient.
		DevCenterByName(devCenterName).
		ProjectByName(projectName).
		Catalogs().
		Get(ctx)

	if err != nil {
		return nil, err
	}

	catalogs := catalogsResponse.Value
	slices.SortFunc(catalogs, func(x, y *devcentersdk.Catalog) bool {
		return x.Name < y.Name
	})

	catalogNames := []string{}
	for _, catalog := range catalogs {
		catalogNames = append(catalogNames, catalog.Name)
	}

	if len(catalogNames) == 1 {
		return catalogs[0], nil
	}

	selected, err := p.console.Select(ctx, input.ConsoleOptions{
		Message: "Select a catalog:",
		Options: catalogNames,
	})

	if err != nil {
		return nil, err
	}

	return catalogs[selected], nil
}

func (p *Prompter) PromptProject(ctx context.Context, devCenterName string) (*devcentersdk.Project, error) {
	writeableProjects, err := p.manager.WritableProjects(ctx)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(writeableProjects, func(x, y *devcentersdk.Project) bool {
		return x.Name < y.Name
	})

	projectNames := []string{}
	for _, project := range writeableProjects {
		if strings.EqualFold(devCenterName, project.DevCenter.Name) {
			projectNames = append(projectNames, project.Name)
		}
	}

	if len(projectNames) == 1 {
		return writeableProjects[0], nil
	}

	selected, err := p.console.Select(ctx, input.ConsoleOptions{
		Message: "Select a project:",
		Options: projectNames,
	})

	if err != nil {
		return nil, err
	}

	return writeableProjects[selected], nil
}

func (p *Prompter) PromptEnvironmentType(
	ctx context.Context,
	devCenterName string,
	projectName string,
) (*devcentersdk.EnvironmentType, error) {
	envTypesResponse, err := p.devCenterClient.
		DevCenterByName(devCenterName).
		ProjectByName(projectName).
		EnvironmentTypes().
		Get(ctx)

	if err != nil {
		return nil, err
	}

	envTypes := envTypesResponse.Value
	slices.SortFunc(envTypes, func(x, y *devcentersdk.EnvironmentType) bool {
		return x.Name < y.Name
	})

	envTypeNames := []string{}
	for _, envType := range envTypesResponse.Value {
		envTypeNames = append(envTypeNames, envType.Name)
	}

	if len(envTypeNames) == 1 {
		return envTypes[0], nil
	}

	selected, err := p.console.Select(ctx, input.ConsoleOptions{
		Message: "Select an environment type:",
		Options: envTypeNames,
	})

	if err != nil {
		return nil, err
	}

	return envTypes[selected], nil
}

func (p *Prompter) PromptEnvironmentDefinition(
	ctx context.Context,
	devCenterName, projectName string,
) (*devcentersdk.EnvironmentDefinition, error) {
	envDefinitionsResponse, err := p.devCenterClient.
		DevCenterByName(devCenterName).
		ProjectByName(projectName).
		EnvironmentDefinitions().
		Get(ctx)

	if err != nil {
		return nil, err
	}

	environmentDefinitions := envDefinitionsResponse.Value
	slices.SortFunc(environmentDefinitions, func(x, y *devcentersdk.EnvironmentDefinition) bool {
		return x.Name < y.Name
	})

	envDefinitionNames := []string{}
	for _, envDefinition := range environmentDefinitions {
		envDefinitionNames = append(envDefinitionNames, envDefinition.Name)
	}

	selected, err := p.console.Select(ctx, input.ConsoleOptions{
		Message: "Select an environment definition:",
		Options: envDefinitionNames,
	})

	if err != nil {
		return nil, err
	}

	return environmentDefinitions[selected], nil
}

// Prompts the user for values defined within the environment definition parameters
func (p *Prompter) PromptParameters(
	ctx context.Context,
	env *environment.Environment,
	envDef *devcentersdk.EnvironmentDefinition,
) (map[string]any, error) {
	paramValues := map[string]any{}

	for _, param := range envDef.Parameters {
		if param.Name == "environmentName" {
			paramValues[param.Name] = env.GetEnvName()
			continue
		}

		if param.Name == "repoUrl" {
			paramValues[param.Name] = param.Allowed[0]
			continue
		}

		paramPath := fmt.Sprintf("%s.%s", ProvisionParametersConfigPath, param.Name)
		paramValue, exists := env.Config.Get(paramPath)
		if !exists {
			promptOptions := input.ConsoleOptions{
				DefaultValue: param.Default,
				Options:      param.Allowed,
				Message:      fmt.Sprintf("Enter a value for %s", param.Name),
				Help:         param.Description,
			}

			if param.Allowed != nil && len(param.Allowed) > 0 {
				selectedIndex, err := p.console.Select(ctx, promptOptions)

				if err != nil {
					return nil, err
				}

				paramValue = param.Allowed[selectedIndex]
			} else {
				promptValue, err := p.console.Prompt(ctx, promptOptions)
				if err != nil {
					return nil, err
				}
				paramValue = promptValue
			}
		}

		paramValues[param.Id] = paramValue
	}

	return paramValues, nil
}
