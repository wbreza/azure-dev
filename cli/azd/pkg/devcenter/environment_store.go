package devcenter

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/contracts"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"golang.org/x/exp/slices"
)

const (
	RemoteKindDevCenter environment.RemoteKind = "devcenter"
)

type EnvironmentStore struct {
	config          *Config
	devCenterClient devcentersdk.DevCenterClient
	manager         *Manager
}

func NewEnvironmentStore(config *Config, devCenterClient devcentersdk.DevCenterClient, manager *Manager) environment.RemoteDataStore {
	return &EnvironmentStore{
		config:          config,
		devCenterClient: devCenterClient,
		manager:         manager,
	}
}

func (s *EnvironmentStore) EnvPath(env *environment.Environment) string {
	return fmt.Sprintf("projects/%s/environments/%s", s.config.Project, env.GetEnvName())
}

func (s *EnvironmentStore) ConfigPath(env *environment.Environment) string {
	return ""
}

func (s *EnvironmentStore) List(ctx context.Context) ([]*contracts.EnvListEnvironment, error) {
	if !s.config.IsValid() {
		return []*contracts.EnvListEnvironment{}, nil
	}

	environmentListResponse, err := s.devCenterClient.
		DevCenterByName(s.config.Name).
		ProjectByName(s.config.Project).
		Environments().
		Get(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get devcenter environment list: %w", err)
	}

	matches := []*contracts.EnvListEnvironment{}
	for _, environment := range environmentListResponse.Value {
		if environment.EnvironmentDefinitionName == s.config.EnvironmentDefinition {
			matches = append(matches, &contracts.EnvListEnvironment{
				Name:       environment.Name,
				DotEnvPath: environment.ResourceGroupId,
			})
		}
	}

	return matches, nil

}

func (s *EnvironmentStore) Get(ctx context.Context, name string) (*environment.Environment, error) {
	envs, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	matchingIndex := slices.IndexFunc(envs, func(env *contracts.EnvListEnvironment) bool {
		return env.Name == name
	})

	if matchingIndex < 0 {
		return nil, fmt.Errorf("%s %w", name, environment.ErrNotFound)
	}

	matchingEnv := envs[matchingIndex]
	env := environment.New(matchingEnv.Name)

	if err := s.Reload(ctx, env); err != nil {
		return nil, err
	}

	return env, nil
}

func (s *EnvironmentStore) Reload(ctx context.Context, env *environment.Environment) error {
	environment, err := s.devCenterClient.
		DevCenterByName(s.config.Name).
		ProjectByName(s.config.Project).
		EnvironmentByName(env.GetEnvName()).
		Get(ctx)

	if err != nil {
		return fmt.Errorf("failed to get devcenter environment: %w", err)
	}

	outputs, err := s.manager.Outputs(ctx, environment)
	if err != nil {
		return fmt.Errorf("failed to get environment outputs: %w", err)
	}

	for key, outputParam := range outputs {
		env.DotenvSet(key, fmt.Sprintf("%v", outputParam.Value))
	}

	env.Config.Set(DevCenterNamePath, s.config.Name)
	env.Config.Set(DevCenterProjectPath, s.config.Project)
	env.Config.Set(DevCenterCatalogPath, s.config.Catalog)
	env.Config.Set(DevCenterEnvTypePath, s.config.EnvironmentType)
	env.Config.Set(DevCenterEnvDefinitionPath, s.config.EnvironmentDefinition)

	return nil
}

func (s *EnvironmentStore) Save(ctx context.Context, env *environment.Environment) error {
	return nil
}
