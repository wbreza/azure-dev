package devcenter

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/infra"
	"github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_ParseConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		partialConfig := map[string]any{
			"name":                  "DEVCENTER_NAME",
			"project":               "PROJECT",
			"environmentDefinition": "ENVIRONMENT_DEFINITION",
		}

		config, err := ParseConfig(partialConfig)
		require.NoError(t, err)
		require.Equal(t, "DEVCENTER_NAME", config.Name)
		require.Equal(t, "PROJECT", config.Project)
		require.Equal(t, "ENVIRONMENT_DEFINITION", config.EnvironmentDefinition)
	})

	t.Run("Failure", func(t *testing.T) {
		partialConfig := "not a map"
		config, err := ParseConfig(partialConfig)
		require.Error(t, err)
		require.Nil(t, config)
	})
}

func Test_MergeConfigs(t *testing.T) {
	t.Run("MergeMissingValues", func(t *testing.T) {
		baseConfig := &Config{
			Name:                  "DEVCENTER_NAME",
			Project:               "PROJECT",
			EnvironmentDefinition: "ENVIRONMENT_DEFINITION",
		}

		overrideConfig := &Config{
			EnvironmentType: "Dev",
		}

		mergedConfig := MergeConfigs(baseConfig, overrideConfig)

		require.Equal(t, "DEVCENTER_NAME", mergedConfig.Name)
		require.Equal(t, "PROJECT", mergedConfig.Project)
		require.Equal(t, "ENVIRONMENT_DEFINITION", mergedConfig.EnvironmentDefinition)
		require.Equal(t, "Dev", mergedConfig.EnvironmentType)
	})

	t.Run("OverrideEmpty", func(t *testing.T) {
		baseConfig := &Config{}

		overrideConfig := &Config{
			Name:                  "OVERRIDE",
			Project:               "OVERRIDE",
			EnvironmentDefinition: "OVERRIDE",
			Catalog:               "OVERRIDE",
			EnvironmentType:       "OVERRIDE",
		}

		mergedConfig := MergeConfigs(baseConfig, overrideConfig)

		require.Equal(t, "OVERRIDE", mergedConfig.Name)
		require.Equal(t, "OVERRIDE", mergedConfig.Project)
		require.Equal(t, "OVERRIDE", mergedConfig.EnvironmentDefinition)
		require.Equal(t, "OVERRIDE", mergedConfig.Catalog)
		require.Equal(t, "OVERRIDE", mergedConfig.EnvironmentType)
	})

	// The base config is a full configuration so there isn't anything to override
	t.Run("NoOverride", func(t *testing.T) {
		baseConfig := &Config{
			Name:                  "DEVCENTER_NAME",
			Project:               "PROJECT",
			EnvironmentDefinition: "ENVIRONMENT_DEFINITION",
			Catalog:               "CATALOG",
			EnvironmentType:       "ENVIRONMENT_TYPE",
		}

		overrideConfig := &Config{
			Name:                  "OVERRIDE",
			Project:               "OVERRIDE",
			EnvironmentDefinition: "OVERRIDE",
			Catalog:               "OVERRIDE",
			EnvironmentType:       "OVERRIDE",
		}

		mergedConfig := MergeConfigs(baseConfig, overrideConfig)

		require.Equal(t, "DEVCENTER_NAME", mergedConfig.Name)
		require.Equal(t, "PROJECT", mergedConfig.Project)
		require.Equal(t, "ENVIRONMENT_DEFINITION", mergedConfig.EnvironmentDefinition)
		require.Equal(t, "CATALOG", mergedConfig.Catalog)
		require.Equal(t, "ENVIRONMENT_TYPE", mergedConfig.EnvironmentType)
	})
}

type mockDevCenterManager struct {
	mock.Mock
}

func (m *mockDevCenterManager) WritableProjects(ctx context.Context) ([]*devcentersdk.Project, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*devcentersdk.Project), args.Error(1)
}

func (m *mockDevCenterManager) WritableProjectsWithFilter(
	ctx context.Context,
	devCenterFilter DevCenterFilterPredicate,
	projectFilter ProjectFilterPredicate,
) ([]*devcentersdk.Project, error) {
	args := m.Called(ctx, devCenterFilter, projectFilter)
	return args.Get(0).([]*devcentersdk.Project), args.Error(1)
}

func (m *mockDevCenterManager) Deployment(
	ctx context.Context,
	env *devcentersdk.Environment,
	filter DeploymentFilterPredicate,
) (infra.Deployment, error) {
	args := m.Called(ctx, env, filter)
	return args.Get(0).(infra.Deployment), args.Error(1)
}

func (m *mockDevCenterManager) LatestArmDeployment(
	ctx context.Context,
	env *devcentersdk.Environment,
	filter DeploymentFilterPredicate,
) (*armresources.DeploymentExtended, error) {
	args := m.Called(ctx, env, filter)
	return args.Get(0).(*armresources.DeploymentExtended), args.Error(1)
}

func (m *mockDevCenterManager) Outputs(
	ctx context.Context,
	env *devcentersdk.Environment,
) (map[string]provisioning.OutputParameter, error) {
	args := m.Called(ctx, env)
	return args.Get(0).(map[string]provisioning.OutputParameter), args.Error(1)
}
