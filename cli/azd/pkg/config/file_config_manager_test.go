package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Test_FileConfigManager_SaveAndLoadConfig(t *testing.T) {
	var azdConfig Config = NewConfig(
		map[string]any{
			"defaults": map[string]any{
				"location":     "eastus2",
				"subscription": "SUBSCRIPTION_ID",
			},
		},
	)

	configFilePath := filepath.Join(t.TempDir(), "config.json")
	configManager := NewFileConfigManager(NewManager())

	err := configManager.Save(azdConfig, configFilePath)
	require.NoError(t, err)

	existingConfig, err := configManager.Load(configFilePath)
	require.NoError(t, err)
	require.NotNil(t, existingConfig)
	require.Equal(t, azdConfig, existingConfig)
}

func Test_FileConfigManager_SaveAndLoadEmptyConfig(t *testing.T) {
	configFilePath := filepath.Join(t.TempDir(), "config.json")

	configManager := NewFileConfigManager(NewManager())
	azdConfig := NewConfig(nil)
	err := configManager.Save(azdConfig, configFilePath)
	require.NoError(t, err)

	existingConfig, err := configManager.Load(configFilePath)
	require.NoError(t, err)
	require.NotNil(t, existingConfig)
}

func Test_FileConfigManager_GetSetSecrets(t *testing.T) {
	tempDir := t.TempDir()
	azdConfigDir := filepath.Join(tempDir, ".azd")

	err := os.Setenv("AZD_CONFIG_DIR", azdConfigDir)
	require.NoError(t, err)

	// Set and save secrets
	configFilePath := filepath.Join(tempDir, "config.json")
	configManager := NewFileConfigManager(NewManager())
	azdConfig := NewConfig(nil).(*config)

	// Standard secrets
	expectedPassword := "P@55w0rd!"
	vaultRef1, err := azdConfig.SetSecret("secrets.password", expectedPassword)
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef1)

	vaultRef2, err := azdConfig.SetSecret("infra.provisioning.sqlPassword", expectedPassword)
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef2)

	// Missing vault reference
	missingVaultRef := fmt.Sprintf("vault://%s/%s", uuid.New().String(), uuid.New().String())
	err = azdConfig.Set("secrets.misingVaultRef", missingVaultRef)
	require.NoError(t, err)

	err = configManager.Save(azdConfig, configFilePath)
	require.NoError(t, err)

	expectedVaultPath := filepath.Join(azdConfigDir, "vaults", fmt.Sprintf("%s.json", azdConfig.vaultId))
	require.FileExists(t, expectedVaultPath)

	// Load and retrieve secrets
	updatedConfig, err := configManager.Load(configFilePath)
	require.NoError(t, err)
	require.NotNil(t, updatedConfig)

	userPassword, ok := updatedConfig.GetString("secrets.password")
	require.True(t, ok)
	require.Equal(t, expectedPassword, userPassword)

	sqlPassword, ok := updatedConfig.GetString("infra.provisioning.sqlPassword")
	require.True(t, ok)
	require.Equal(t, expectedPassword, sqlPassword)

	// Missing vault reference will return empty string
	// even though the value appears to be a vault reference
	missingPassword, ok := updatedConfig.GetString("secrets.misingVaultRef")
	require.False(t, ok)
	require.Empty(t, missingPassword)
}

func Test_FileConfigManager_GetSetSecretsInSection(t *testing.T) {
	tempDir := t.TempDir()
	azdConfigDir := filepath.Join(tempDir, ".azd")

	err := os.Setenv("AZD_CONFIG_DIR", azdConfigDir)
	require.NoError(t, err)

	// Set and save secrets
	configFilePath := filepath.Join(tempDir, "config.json")
	configManager := NewFileConfigManager(NewManager())
	azdConfig := NewConfig(nil)

	vaultRef1, err := azdConfig.SetSecret("infra.provisioning.secret1", "secrect1Value")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef1)

	vaultRef2, err := azdConfig.SetSecret("infra.provisioning.secret2", "secrect2Value")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef2)

	err = azdConfig.Set("infra.provisioning.normalValue", "normalValue")
	require.NoError(t, err)

	err = configManager.Save(azdConfig, configFilePath)
	require.NoError(t, err)

	var provisioningParams map[string]string
	ok, err := azdConfig.GetSection("infra.provisioning", &provisioningParams)
	require.NoError(t, err)
	require.True(t, ok)

	secret1, ok := provisioningParams["secret1"]
	require.True(t, ok)
	require.Equal(t, "secrect1Value", secret1)

	secret2, ok := provisioningParams["secret2"]
	require.True(t, ok)
	require.Equal(t, "secrect2Value", secret2)

	normalValue, ok := provisioningParams["normalValue"]
	require.True(t, ok)
	require.Equal(t, "normalValue", normalValue)
}

func Test_FileConfigManager_UnsetSecret(t *testing.T) {
	// Set and save secrets
	azdConfig := NewConfig(nil).(*config)

	vaultRef1, err := azdConfig.SetSecret("secrets.password1", "password1")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef1)

	vaultRef2, err := azdConfig.SetSecret("secrets.password2", "password2")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef2)

	require.Len(t, azdConfig.vault.Raw(), 2)

	err = azdConfig.Unset("secrets.password1")
	require.NoError(t, err)
	require.Len(t, azdConfig.vault.Raw(), 1)

	err = azdConfig.Unset("secrets.password2")
	require.NoError(t, err)
	require.Len(t, azdConfig.vault.Raw(), 0)
}

func Test_FileConfigManager_UnsetSectionWithSecrets(t *testing.T) {
	// Set and save secrets
	azdConfig := NewConfig(nil).(*config)

	vaultRef1, err := azdConfig.SetSecret("secrets.password1", "password1")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef1)

	vaultRef2, err := azdConfig.SetSecret("secrets.password2", "password2")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef2)

	require.Len(t, azdConfig.vault.Raw(), 2)

	err = azdConfig.Unset("secrets")
	require.NoError(t, err)
	require.Len(t, azdConfig.vault.Raw(), 0)
}

func Test_FileConfigManager_CleanUpEmptyVault(t *testing.T) {
	tempDir := t.TempDir()
	azdConfigDir := filepath.Join(tempDir, ".azd")

	err := os.Setenv("AZD_CONFIG_DIR", azdConfigDir)
	require.NoError(t, err)

	// Set and save secrets
	configFilePath := filepath.Join(tempDir, "config.json")
	configManager := NewFileConfigManager(NewManager())
	azdConfig := NewConfig(nil).(*config)

	vaultRef, err := azdConfig.SetSecret("secrets.password", "P@55w0rd!")
	require.NoError(t, err)
	require.NotEmpty(t, vaultRef)

	err = configManager.Save(azdConfig, configFilePath)
	require.NoError(t, err)

	expectedVaultPath := filepath.Join(azdConfigDir, "vaults", fmt.Sprintf("%s.json", azdConfig.vaultId))
	require.FileExists(t, expectedVaultPath)

	err = azdConfig.Unset("secrets.password")
	require.NoError(t, err)

	err = configManager.Save(azdConfig, configFilePath)
	require.NoError(t, err)

	// vault file should now be deleted since it no longer contains any secrets
	require.NoFileExists(t, expectedVaultPath)
}
