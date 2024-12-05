package azdgrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/azdconfig"
	"github.com/azure/azure-dev/cli/azd/pkg/config"
)

// configService is the implementation of ConfigServiceServer.
type userConfigService struct {
	azdconfig.UnimplementedUserConfigServiceServer

	configManager config.UserConfigManager
	config        config.Config
}

// NewConfigService creates a new instance of configService.
func NewUserConfigService(userConfigManager config.UserConfigManager) (azdconfig.UserConfigServiceServer, error) {
	config, err := userConfigManager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load user config: %v", err)
	}

	return &userConfigService{
		configManager: userConfigManager,
		config:        config,
	}, nil
}

func (s *userConfigService) Get(ctx context.Context, req *azdconfig.GetRequest) (*azdconfig.GetResponse, error) {
	value, exists := s.config.Get(req.Path)

	var valueBytes []byte
	if exists {
		bytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %v", err)
		}

		valueBytes = bytes
	}

	return &azdconfig.GetResponse{
		Value: valueBytes,
		Found: exists,
	}, nil
}

func (s *userConfigService) GetString(ctx context.Context, req *azdconfig.GetStringRequest) (*azdconfig.GetStringResponse, error) {
	value, exists := s.config.GetString(req.Path)

	return &azdconfig.GetStringResponse{
		Value: value,
		Found: exists,
	}, nil
}

func (s *userConfigService) GetSection(ctx context.Context, req *azdconfig.GetSectionRequest) (*azdconfig.GetSectionResponse, error) {
	var section map[string]any

	exists, err := s.config.GetSection(req.Path, &section)
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

	return &azdconfig.GetSectionResponse{
		Section: valueBytes,
		Found:   exists,
	}, nil
}

func (s *userConfigService) Set(ctx context.Context, req *azdconfig.SetRequest) (*azdconfig.SetResponse, error) {
	var value any
	if err := json.Unmarshal(req.Value, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %v", err)
	}

	if err := s.config.Set(req.Path, value); err != nil {
		return nil, fmt.Errorf("failed to set value: %v", err)
	}

	if err := s.configManager.Save(s.config); err != nil {
		return nil, fmt.Errorf("failed to save config: %v", err)
	}

	return &azdconfig.SetResponse{}, nil
}

func (s *userConfigService) Unset(ctx context.Context, req *azdconfig.UnsetRequest) (*azdconfig.UnsetResponse, error) {
	if err := s.config.Unset(req.Path); err != nil {
		return nil, fmt.Errorf("failed to unset value: %v", err)
	}

	if err := s.configManager.Save(s.config); err != nil {
		return nil, fmt.Errorf("failed to save config: %v", err)
	}

	return &azdconfig.UnsetResponse{}, nil
}
