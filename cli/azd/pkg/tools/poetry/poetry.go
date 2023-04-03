package poetry

import (
	"context"

	"github.com/azure/azure-dev/cli/azd/pkg/tools"
)

type poetryCli struct {
}

func NewPoetryCli() PoetryCli {
	return &poetryCli{}
}

type PoetryCli interface {
	tools.ExternalTool
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
