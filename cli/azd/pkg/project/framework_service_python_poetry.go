// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/azure/azure-dev/cli/azd/pkg/async"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/poetry"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/python"
)

type poetryProject struct {
	env       *environment.Environment
	pythonCli *python.PythonCli
	poetryCli poetry.PoetryCli
}

// NewPythonProject creates a new instance of the Python project
func NewPoetryProject(pythonCli *python.PythonCli, poetryCli poetry.PoetryCli, env *environment.Environment) FrameworkService {
	return &poetryProject{
		env:       env,
		pythonCli: pythonCli,
		poetryCli: poetryCli,
	}
}

func (pp *poetryProject) Requirements() FrameworkRequirements {
	return FrameworkRequirements{
		// Python does not require compilation and will just package the raw source files
		Package: FrameworkPackageRequirements{
			RequireRestore: false,
			RequireBuild:   false,
		},
	}
}

// Gets the required external tools for the project
func (pp *poetryProject) RequiredExternalTools(context.Context) []tools.ExternalTool {
	return []tools.ExternalTool{pp.pythonCli, pp.poetryCli}
}

// Initializes the Python project
func (pp *poetryProject) Initialize(ctx context.Context, serviceConfig *ServiceConfig) error {
	return nil
}

// Restores the project dependencies using PIP requirements.txt
func (pp *poetryProject) Restore(
	ctx context.Context,
	serviceConfig *ServiceConfig,
) *async.TaskWithProgress[*ServiceRestoreResult, ServiceProgress] {
	return async.RunTaskWithProgress(
		func(task *async.TaskContextWithProgress[*ServiceRestoreResult, ServiceProgress]) {
		},
	)
}

// Build for Python apps performs a no-op and returns the service path with an optional output path when specified.
func (pp *poetryProject) Build(
	ctx context.Context,
	serviceConfig *ServiceConfig,
	restoreOutput *ServiceRestoreResult,
) *async.TaskWithProgress[*ServiceBuildResult, ServiceProgress] {
	return async.RunTaskWithProgress(
		func(task *async.TaskContextWithProgress[*ServiceBuildResult, ServiceProgress]) {

		},
	)
}

func (pp *poetryProject) Package(
	ctx context.Context,
	serviceConfig *ServiceConfig,
	buildOutput *ServiceBuildResult,
) *async.TaskWithProgress[*ServicePackageResult, ServiceProgress] {
	return async.RunTaskWithProgress(
		func(task *async.TaskContextWithProgress[*ServicePackageResult, ServiceProgress]) {
			packageRoot, err := os.MkdirTemp("", "azd")
			if err != nil {
				task.SetError(fmt.Errorf("creating package directory for %s: %w", serviceConfig.Name, err))
				return
			}

			packageSource := buildOutput.BuildOutputPath
			if packageSource == "" {
				packageSource = filepath.Join(serviceConfig.Path(), serviceConfig.OutputPath)
			}

			task.SetProgress(NewServiceProgress("Copying deployment package"))
			if err := buildForZip(
				packageSource,
				packageRoot,
				buildForZipOptions{
					excludeConditions: []excludeDirEntryCondition{
						excludeVirtualEnv,
						excludePyCache,
					},
				}); err != nil {
				task.SetError(fmt.Errorf("packaging for %s: %w", serviceConfig.Name, err))
				return
			}

			task.SetResult(&ServicePackageResult{
				Build:       buildOutput,
				PackagePath: packageRoot,
			})
		},
	)
}
