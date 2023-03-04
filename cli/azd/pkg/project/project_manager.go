package project

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/telemetry"
	"github.com/azure/azure-dev/cli/azd/internal/telemetry/fields"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/environment/azdcontext"
	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/ext"
	"github.com/azure/azure-dev/cli/azd/pkg/infra"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/azcli"
	"gopkg.in/yaml.v3"
)

const (
	projectSchemaAnnotation = "# yaml-language-server: $schema=" +
		"https://raw.githubusercontent.com/Azure/azure-dev/main/schemas/v1.0/azure.yaml.json"
)

type ProjectManager interface {
	Initialize(ctx context.Context, projectConfig *ProjectConfig) error
	Parse(ctx context.Context, yamlContent string) (*ProjectConfig, error)
	Load(ctx context.Context, projectPath string) (*ProjectConfig, error)
	Save(ctx context.Context, projectConfig *ProjectConfig, projectFilePath string) error
	GetResourceGroupName(ctx context.Context, projectConfig *ProjectConfig) (string, error)
}

type projectManager struct {
	*ext.EventDispatcher[ServiceLifecycleEventArgs]

	azdContext     *azdcontext.AzdContext
	env            *environment.Environment
	commandRunner  exec.CommandRunner
	azCli          azcli.AzCli
	console        input.Console
	accountManager account.Manager
	serviceManager *serviceManager
}

// Saves the current instance back to the azure.yaml file
func (pm *projectManager) Save(ctx context.Context, projectConfig *ProjectConfig, projectFilePath string) error {
	projectBytes, err := yaml.Marshal(projectConfig)
	if err != nil {
		return fmt.Errorf("marshalling project yaml: %w", err)
	}

	projectFileContents := bytes.NewBufferString(projectSchemaAnnotation + "\n\n")
	_, err = projectFileContents.Write(projectBytes)
	if err != nil {
		return fmt.Errorf("preparing new project file contents: %w", err)
	}

	err = os.WriteFile(projectFilePath, projectFileContents.Bytes(), osutil.PermissionFile)
	if err != nil {
		return fmt.Errorf("saving project file: %w", err)
	}

	projectConfig.Path = projectFilePath

	return nil
}

// ParseProjectConfig will parse a project from a yaml string and return the project configuration
func (pm *projectManager) Parse(ctx context.Context, yamlContent string) (*ProjectConfig, error) {
	var projectConfig ProjectConfig

	if err := yaml.Unmarshal([]byte(yamlContent), &projectConfig); err != nil {
		return nil, fmt.Errorf(
			"unable to parse azure.yaml file. Please check the format of the file, "+
				"and also verify you have the latest version of the CLI: %w",
			err,
		)
	}

	for key, svc := range projectConfig.Services {
		svc.Name = key
		svc.Project = &projectConfig

		// By convention, the name of the infrastructure module to use when doing an IaC based deployment is the friendly
		// name of the service. This may be overridden by the `module` property of `azure.yaml`
		if svc.Module == "" {
			svc.Module = key
		}

		if svc.Language == "" || svc.Language == "csharp" || svc.Language == "fsharp" {
			svc.Language = "dotnet"
		}
	}

	return &projectConfig, nil
}

func (pm *projectManager) Initialize(ctx context.Context, projectConfig *ProjectConfig) error {
	var allTools []tools.ExternalTool

	for _, svc := range projectConfig.Services {
		frameworkService, err := pm.serviceManager.GetFrameworkService(ctx, svc)
		if err != nil {
			return fmt.Errorf("getting framework services: %w", err)
		}
		if err := frameworkService.Initialize(ctx); err != nil {
			return err
		}

		requiredTools := frameworkService.RequiredExternalTools()
		allTools = append(allTools, requiredTools...)
	}

	if err := tools.EnsureInstalled(ctx, tools.Unique(allTools)...); err != nil {
		return err
	}

	return nil
}

// LoadProjectConfig loads the azure.yaml configuring into an viewable structure
// This does not evaluate any tooling
func (pm *projectManager) Load(ctx context.Context, projectFilePath string) (*ProjectConfig, error) {
	log.Printf("Reading project from file '%s'\n", projectFilePath)
	bytes, err := os.ReadFile(projectFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading project file: %w", err)
	}

	yaml := string(bytes)

	projectConfig, err := pm.Parse(ctx, yaml)
	if err != nil {
		return nil, fmt.Errorf("parsing project file: %w", err)
	}

	if projectConfig.Metadata != nil {
		telemetry.SetUsageAttributes(fields.StringHashed(fields.TemplateIdKey, projectConfig.Metadata.Template))
	}

	projectConfig.Path = filepath.Dir(projectFilePath)
	return projectConfig, nil
}

func (pm *projectManager) NewProject(ctx context.Context, projectFilePath string, projectName string) (*ProjectConfig, error) {
	newProject := &ProjectConfig{
		Name: projectName,
	}

	err := pm.Save(ctx, newProject, projectFilePath)
	if err != nil {
		return nil, fmt.Errorf("marshaling project file to yaml: %w", err)
	}

	return pm.Load(ctx, projectFilePath)
}

// GetResourceGroupName gets the resource group name for the project.
//
// The resource group name is resolved in the following order:
//   - The user defined value in `azure.yaml`
//   - The user defined environment value `AZURE_RESOURCE_GROUP`
//
// - Resource group discovery by querying Azure Resources
// (see `resourceManager.FindResourceGroupForEnvironment` for more
// details)
func (pm *projectManager) GetResourceGroupName(ctx context.Context, projectConfig *ProjectConfig) (string, error) {

	name, err := projectConfig.ResourceGroupName.Envsubst(pm.env.Getenv)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(name) != "" {
		return name, nil
	}

	envResourceGroupName := environment.GetResourceGroupNameFromEnvVar(pm.env)
	if envResourceGroupName != "" {
		return envResourceGroupName, nil
	}

	resourceManager := infra.NewAzureResourceManager(pm.azCli)
	resourceGroupName, err := resourceManager.FindResourceGroupForEnvironment(ctx, pm.env)
	if err != nil {
		return "", err
	}

	return resourceGroupName, nil
}
