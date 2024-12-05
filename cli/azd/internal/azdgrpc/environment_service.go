package azdgrpc

import (
	"context"

	azdext "github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/grpc"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/environment/azdcontext"
)

type environmentService struct {
	azdext.UnimplementedEnvironmentServiceServer
	azdContext *azdcontext.AzdContext
	envManager environment.Manager
}

func NewEnvironmentService(azdContext *azdcontext.AzdContext, envManager environment.Manager) azdext.EnvironmentServiceServer {
	return &environmentService{
		azdContext: azdContext,
		envManager: envManager,
	}
}

func (s *environmentService) GetCurrent(context.Context, *azdext.Empty) (*azdext.EnvironmentResponse, error) {
	defaultEnvironment, err := s.azdContext.GetDefaultEnvironmentName()
	if err != nil {
		return nil, err
	}

	return &azdext.EnvironmentResponse{
		Environment: &azdext.Environment{
			Name: defaultEnvironment,
		},
	}, nil
}

func (s *environmentService) Get(ctx context.Context, req *azdext.GetEnvironmentRequest) (*azdext.EnvironmentResponse, error) {
	env, err := s.envManager.Get(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	return &azdext.EnvironmentResponse{
		Environment: &azdext.Environment{
			Name: env.Name(),
		},
	}, nil
}

// GetValues retrieves all key-value pairs in the specified environment.
func (s *environmentService) GetValues(ctx context.Context, req *azdext.GetEnvironmentRequest) (*azdext.KeyValueListResponse, error) {
	env, err := s.envManager.Get(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	value := env.Dotenv()
	keyValues := make([]*azdext.KeyValue, len(value))

	i := 0
	for key, value := range value {
		keyValues[i] = &azdext.KeyValue{
			Key:   key,
			Value: value,
		}
		i++
	}

	return &azdext.KeyValueListResponse{
		KeyValues: keyValues,
	}, nil
}

// GetValue retrieves the value of a specific key in the specified environment.
func (s *environmentService) GetValue(ctx context.Context, req *azdext.GetEnvRequest) (*azdext.KeyValueResponse, error) {
	env, err := s.envManager.Get(ctx, req.EnvName)
	if err != nil {
		return nil, err
	}

	value := env.Getenv(req.Key)

	return &azdext.KeyValueResponse{
		Key:   req.Key,
		Value: value,
	}, nil
}

// SetValue sets the value of a key in the specified environment.
func (s *environmentService) SetValue(ctx context.Context, req *azdext.SetEnvRequest) (*azdext.Empty, error) {
	env, err := s.envManager.Get(ctx, req.EnvName)
	if err != nil {
		return nil, err
	}

	env.DotenvSet(req.Key, req.Value)

	return &azdext.Empty{}, nil
}
