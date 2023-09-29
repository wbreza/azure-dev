package devcenter

import (
	"testing"

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
