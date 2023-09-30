package devcenter

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/azure/azure-dev/cli/azd/pkg/azsdk"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/pkg/templates"
	"github.com/azure/azure-dev/cli/azd/test/mocks"
	"github.com/azure/azure-dev/cli/azd/test/mocks/mockdevcentersdk"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var mockEnvDefinitions []*devcentersdk.EnvironmentDefinition = []*devcentersdk.EnvironmentDefinition{
	{
		Id:           "/projects/Project1/catalogs/SampleCatalog/environmentDefinitions/WebApp",
		Name:         "WebApp",
		CatalogName:  "SampleCatalog",
		Description:  "Description of WebApp",
		TemplatePath: "azuredeploy.json",
		Parameters: []devcentersdk.Parameter{
			{
				Id:      "repoUrl",
				Name:    "repoUrl",
				Type:    devcentersdk.ParameterTypeString,
				Default: "https://github.com/Azure-Samples/todo-nodejs-mongo",
			},
		},
	},
	{
		Id:           "/projects/Project1/catalogs/SampleCatalog/environmentDefinitions/ContainerApp",
		Name:         "ContainerApp",
		CatalogName:  "SampleCatalog",
		Description:  "Description of ContainerApp",
		TemplatePath: "azuredeploy.json",
		Parameters: []devcentersdk.Parameter{
			{
				Id:      "repoUrl",
				Name:    "repoUrl",
				Type:    devcentersdk.ParameterTypeString,
				Default: "https://github.com/Azure-Samples/todo-nodejs-mongo-aca",
			},
		},
	},
	{
		Id:           "/projects/Project1/catalogs/SampleCatalog/environmentDefinitions/FunctionApp",
		Name:         "FunctionApp",
		CatalogName:  "SampleCatalog",
		Description:  "Description of FunctionApp",
		TemplatePath: "azuredeploy.json",
		Parameters: []devcentersdk.Parameter{
			{
				Id:      "repoUrl",
				Name:    "repoUrl",
				Type:    devcentersdk.ParameterTypeString,
				Default: "https://github.com/Azure-Samples/todo-nodejs-mongo-swa-func",
			},
		},
	},
}

var mockProjects []*devcentersdk.Project = []*devcentersdk.Project{
	{
		Id:   "/projects/Project1",
		Name: "Project1",
		DevCenter: &devcentersdk.DevCenter{
			Name:       "DEV_CENTER",
			ServiceUri: "https://DEV_CENTER.eastus2.devcenter.azure.com",
		},
	},
	{
		Id:   "/projects/Project2",
		Name: "Project2",
		DevCenter: &devcentersdk.DevCenter{
			ServiceUri: "https://DEV_CENTER.eastus2.devcenter.azure.com",
		},
	},
}

func Test_TemplateSource_ListTemplates(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := mocks.NewMockContext(context.Background())
		mockdevcentersdk.MockDevCenterGraphQuery(mockContext)
		mockdevcentersdk.MockListEnvironmentDefinitions(mockContext, "Project1", mockEnvDefinitions)
		mockdevcentersdk.MockListEnvironmentDefinitions(mockContext, "Project2", []*devcentersdk.EnvironmentDefinition{})

		manager := &mockDevCenterManager{}
		manager.
			On("WritableProjectsWithFilter", *mockContext.Context, mock.Anything, mock.Anything).
			Return(mockProjects, nil)

		templateSource := newTemplateSourceForTest(t, mockContext, &Config{}, manager)
		templateList, err := templateSource.ListTemplates(*mockContext.Context)
		require.NoError(t, err)
		require.NotNil(t, templateList)
		require.Len(t, templateList, len(mockEnvDefinitions))
		require.Len(t, templateList[0].Metadata.Project, 4)
	})

	t.Run("Fail", func(t *testing.T) {
		mockContext := mocks.NewMockContext(context.Background())
		mockdevcentersdk.MockDevCenterGraphQuery(mockContext)
		mockdevcentersdk.MockListEnvironmentDefinitions(mockContext, "Project1", mockEnvDefinitions)
		// Mock will throw 404 not found for this API call causing a failure
		mockdevcentersdk.MockListEnvironmentDefinitions(mockContext, "Project2", nil)

		manager := &mockDevCenterManager{}
		manager.
			On("WritableProjectsWithFilter", *mockContext.Context, mock.Anything, mock.Anything).
			Return(mockProjects, nil)

		templateSource := newTemplateSourceForTest(t, mockContext, &Config{}, manager)
		templateList, err := templateSource.ListTemplates(*mockContext.Context)
		require.Error(t, err)
		require.Nil(t, templateList)
	})
}

func newTemplateSourceForTest(
	t *testing.T,
	mockContext *mocks.MockContext,
	config *Config,
	manager Manager,
) templates.Source {
	coreOptions := azsdk.
		DefaultClientOptionsBuilder(*mockContext.Context, mockContext.HttpClient, "azd").
		BuildCoreClientOptions()

	armOptions := azsdk.
		DefaultClientOptionsBuilder(*mockContext.Context, mockContext.HttpClient, "azd").
		BuildArmClientOptions()

	resourceGraphClient, err := armresourcegraph.NewClient(mockContext.Credentials, armOptions)
	require.NoError(t, err)

	devCenterClient, err := devcentersdk.NewDevCenterClient(
		mockContext.Credentials,
		coreOptions,
		resourceGraphClient,
	)

	require.NoError(t, err)

	if manager == nil {
		manager = &mockDevCenterManager{}
	}

	return NewTemplateSource(config, manager, devCenterClient)
}
