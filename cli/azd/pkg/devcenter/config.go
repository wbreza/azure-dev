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

type Config struct {
	Name                  string `json:"name" yaml:"name"`
	Catalog               string `json:"catalog" yaml:"catalog"`
	Project               string `json:"project" yaml:"project"`
	EnvironmentType       string `json:"environmentType" yaml:"environmentType"`
	EnvironmentDefinition string `json:"environmentDefinition" yaml:"environmentDefinition"`
}

func (c *Config) IsValid() bool {
	return c.Name != "" &&
		c.Catalog != "" &&
		c.Project != "" &&
		c.EnvironmentType != "" &&
		c.EnvironmentDefinition != ""
}
