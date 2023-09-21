package devcenter

const (
	// Environment variable names
	DevCenterNameEnvName          = "AZURE_DEVCENTER_NAME"
	DevCenterCatalogEnvName       = "AZURE_DEVCENTER_CATALOG"
	DevCenterProjectEnvName       = "AZURE_DEVCENTER_PROJECT"
	DevCenterEnvTypeEnvName       = "AZURE_DEVCENTER_ENVIRONMENT_TYPE"
	DevCenterEnvDefinitionEnvName = "AZURE_DEVCENTER_ENVIRONMENT_DEFINITION"

	// Environment configuration paths
	DevCenterNamePath          = "devCenter.name"
	DevCenterCatalogPath       = "devCenter.catalog"
	DevCenterProjectPath       = "devCenter.project"
	DevCenterEnvTypePath       = "devCenter.environmentType"
	DevCenterEnvDefinitionPath = "devCenter.environmentDefinition"
)

// Config provides the Azure DevCenter configuration used for devcenter enabled projects
type Config struct {
	Name                  string `json:"name,omitempty"                  yaml:"name,omitempty"`
	Catalog               string `json:"catalog,omitempty"               yaml:"catalog,omitempty"`
	Project               string `json:"project,omitempty"               yaml:"project,omitempty"`
	EnvironmentType       string `json:"environmentType,omitempty"       yaml:"environmentType,omitempty"`
	EnvironmentDefinition string `json:"environmentDefinition,omitempty" yaml:"environmentDefinition,omitempty"`
}

func (c *Config) IsValid() bool {
	return c.Name != "" &&
		c.Catalog != "" &&
		c.Project != "" &&
		c.EnvironmentType != "" &&
		c.EnvironmentDefinition != ""
}
