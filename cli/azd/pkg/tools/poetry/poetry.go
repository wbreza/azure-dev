package poetry

import (
	"context"
	"fmt"
	"runtime"

	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
)

type poetryCli struct {
	commandRunner exec.CommandRunner
}

func NewPoetryCli(commandRunner exec.CommandRunner) PoetryCli {
	return &poetryCli{
		commandRunner: commandRunner,
	}
}

type PoetryCli interface {
	tools.ExternalTool
	Install(ctx context.Context, projectPath string) error
	Build(ctx context.Context, projectPath string) error
	Activate(ctx context.Context, projectPath string) error
}

func (cli *poetryCli) CheckInstalled(ctx context.Context) (bool, error) {
	found, err := tools.ToolInPath("poetry")
	if !found {
		return false, err
	}

	return true, nil
}

func (cli *poetryCli) InstallUrl() string {
	return "https://python-poetry.org/docs/#installation"
}

func (cli *poetryCli) Name() string {
	return "Poetry CLI"
}

// Installs the dependencies for the Poetry project
func (cli *poetryCli) Install(ctx context.Context, projectPath string) error {
	runArgs := exec.NewRunArgs("poetry", "install").
		WithCwd(projectPath)

	if _, err := cli.commandRunner.Run(ctx, runArgs); err != nil {
		return fmt.Errorf("installing poetry dependencies, %w", err)
	}

	return nil
}

// Builds the Poetry project
func (cli *poetryCli) Build(ctx context.Context, projectPath string) error {
	runArgs := exec.NewRunArgs("poetry", "build").
		WithCwd(projectPath)

	if _, err := cli.commandRunner.Run(ctx, runArgs); err != nil {
		return fmt.Errorf("building poetry project source, %w", err)
	}

	return nil
}

// Activate Python virtual environment
func (cli *poetryCli) Activate(ctx context.Context, projectPath string) error {
	var command string
	if runtime.GOOS == "windows" {
		command = `& ((poetry env info --path) + "\Scripts\activate.ps1")`
	} else {
		command = "source $(poetry env info --path)/bin/activate"
	}

	runArgs := exec.NewRunArgs("").WithCwd(projectPath)
	if _, err := cli.commandRunner.RunList(ctx, []string{command}, runArgs); err != nil {
		return fmt.Errorf("activating virtual environment, %w", err)
	}

	return nil
}
