package azdgrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/azdenv"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/environment/azdcontext"
)

type environmentService struct {
	azdenv.UnimplementedEnvironmentServiceServer
	azdContext *azdcontext.AzdContext
	envManager environment.Manager
}

func NewEnvironmentService(azdContext *azdcontext.AzdContext, envManager environment.Manager) azdenv.EnvironmentServiceServer {
	return &environmentService{
		azdContext: azdContext,
		envManager: envManager,
	}
}

func (s *environmentService) GetCurrent(ctx context.Context, req *azdenv.EmptyResponse) (*azdenv.EnvironmentResponse, error) {
	env, err := s.currentEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	return &azdenv.EnvironmentResponse{
		Environment: &azdenv.Environment{
			Name: env.Name(),
		},
	}, nil
}

func (s *environmentService) Get(ctx context.Context, req *azdenv.GetEnvironmentRequest) (*azdenv.EnvironmentResponse, error) {
	env, err := s.envManager.Get(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	return &azdenv.EnvironmentResponse{
		Environment: &azdenv.Environment{
			Name: env.Name(),
		},
	}, nil
}

// GetValues retrieves all key-value pairs in the specified environment.
func (s *environmentService) GetValues(ctx context.Context, req *azdenv.GetEnvironmentRequest) (*azdenv.KeyValueListResponse, error) {
	env, err := s.envManager.Get(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	value := env.Dotenv()
	keyValues := make([]*azdenv.KeyValue, len(value))

	i := 0
	for key, value := range value {
		keyValues[i] = &azdenv.KeyValue{
			Key:   key,
			Value: value,
		}
		i++
	}

	return &azdenv.KeyValueListResponse{
		KeyValues: keyValues,
	}, nil
}

// GetValue retrieves the value of a specific key in the specified environment.
func (s *environmentService) GetValue(ctx context.Context, req *azdenv.GetEnvRequest) (*azdenv.KeyValueResponse, error) {
	env, err := s.envManager.Get(ctx, req.EnvName)
	if err != nil {
		return nil, err
	}

	value := env.Getenv(req.Key)

	return &azdenv.KeyValueResponse{
		Key:   req.Key,
		Value: value,
	}, nil
}

// SetValue sets the value of a key in the specified environment.
func (s *environmentService) SetValue(ctx context.Context, req *azdenv.SetEnvRequest) (*azdenv.EmptyResponse, error) {
	env, err := s.envManager.Get(ctx, req.EnvName)
	if err != nil {
		return nil, err
	}

	env.DotenvSet(req.Key, req.Value)

	return &azdenv.EmptyResponse{}, nil
}

func (s *environmentService) currentEnvironment(ctx context.Context) (*environment.Environment, error) {
	defaultEnvironment, err := s.azdContext.GetDefaultEnvironmentName()
	if err != nil {
		return nil, err
	}

	env, err := s.envManager.Get(ctx, defaultEnvironment)
	if err != nil {
		return nil, fmt.Errorf("failed to get current environment: %v", err)
	}

	return env, nil
}

// GetConfig retrieves a config value by path.
func (s *environmentService) GetConfig(ctx context.Context, req *azdenv.GetConfigRequest) (*azdenv.GetConfigResponse, error) {
	env, err := s.currentEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	value, exists := env.Config.Get(req.Path)

	var valueBytes []byte
	if exists {
		bytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %v", err)
		}

		valueBytes = bytes
	}

	return &azdenv.GetConfigResponse{
		Value: valueBytes,
		Found: exists,
	}, nil
}

// GetConfigString retrieves a config value as a string by path.
func (s *environmentService) GetConfigString(ctx context.Context, req *azdenv.GetConfigStringRequest) (*azdenv.GetConfigStringResponse, error) {
	env, err := s.currentEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	value, exists := env.Config.GetString(req.Path)

	return &azdenv.GetConfigStringResponse{
		Value: value,
		Found: exists,
	}, nil
}

// GetConfigSection retrieves a config section by path.
func (s *environmentService) GetConfigSection(ctx context.Context, req *azdenv.GetConfigSectionRequest) (*azdenv.GetConfigSectionResponse, error) {
	env, err := s.currentEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	var section map[string]any

	exists, err := env.Config.GetSection(req.Path, &section)
	if err != nil {
		return nil, fmt.Errorf("failed to get section: %v", err)
	}

	var valueBytes []byte
	if exists {
		bytes, err := json.Marshal(section)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %v", err)
		}

		valueBytes = bytes
	}

	return &azdenv.GetConfigSectionResponse{
		Section: valueBytes,
		Found:   exists,
	}, nil
}

// SetConfig sets a config value at a given path.
func (s *environmentService) SetConfig(ctx context.Context, req *azdenv.SetConfigRequest) (*azdenv.EmptyResponse, error) {
	env, err := s.currentEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	var value any
	if err := json.Unmarshal(req.Value, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %v", err)
	}

	if err := env.Config.Set(req.Path, value); err != nil {
		return nil, fmt.Errorf("failed to set value: %v", err)
	}

	if err := s.envManager.Save(ctx, env); err != nil {
		return nil, fmt.Errorf("failed to save config: %v", err)
	}

	return &azdenv.EmptyResponse{}, nil
}

// UnsetConfig unsets a config value at a given path.
func (s *environmentService) UnsetConfig(ctx context.Context, req *azdenv.UnsetConfigRequest) (*azdenv.EmptyResponse, error) {
	env, err := s.currentEnvironment(ctx)
	if err != nil {
		return nil, err
	}

	if err := env.Config.Unset(req.Path); err != nil {
		return nil, fmt.Errorf("failed to unset value: %v", err)
	}

	if err := s.envManager.Save(ctx, env); err != nil {
		return nil, fmt.Errorf("failed to save config: %v", err)
	}

	return &azdenv.EmptyResponse{}, nil
}
