package mockdevcentersdk

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"github.com/azure/azure-dev/cli/azd/test/mocks"
)

func MockDevCenterGraphQuery(mockContext *mocks.MockContext) {
	mockContext.HttpClient.When(func(request *http.Request) bool {
		return strings.Contains(request.URL.Path, "providers/Microsoft.ResourceGraph/resources")
	}).RespondFn(func(request *http.Request) (*http.Response, error) {
		body := armresourcegraph.ClientResourcesResponse{
			QueryResponse: armresourcegraph.QueryResponse{
				Data: []*devcentersdk.GenericResource{
					{
						//nolint:lll
						Id:       "/subscriptions/SUBSCRIPTION_ID/resourceGroups/RESOURCE_GROUP/providers/Microsoft.DevCenter/projects/Project1",
						Location: "eastus2",
						Name:     "Project1",
						Type:     "microsoft.devcenter/projects",
						TenantId: "TENANT_ID",
						Properties: map[string]interface{}{
							"devCenterUri": "https://DEV_CENTER.eastus2.devcenter.azure.com",
							//nolint:lll
							"devCenterId": "/subscriptions/SUBSCRIPTION_ID/resourceGroups/RESOURCE_GROUP/providers/Microsoft.DevCenter/devcenters/DEV_CENTER",
						},
					},
				},
			},
		}

		return mocks.CreateHttpResponseWithBody(request, http.StatusOK, body)
	})
}

func MockListEnvironments(
	mockContext *mocks.MockContext,
	projectName string,
	environments []*devcentersdk.Environment,
) *http.Request {
	mockRequest := &http.Request{}

	mockContext.HttpClient.When(func(request *http.Request) bool {
		return request.Method == http.MethodGet && request.URL.Path == fmt.Sprintf("/projects/%s/environments", projectName)
	}).RespondFn(func(request *http.Request) (*http.Response, error) {
		*mockRequest = *request

		response := devcentersdk.EnvironmentListResponse{
			Value: environments,
		}

		return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
	})

	return mockRequest
}

func MockGetEnvironment(
	mockContext *mocks.MockContext,
	projectName string,
	userId string,
	environmentName string,
	environment *devcentersdk.Environment,
) *http.Request {
	mockRequest := &http.Request{}

	mockContext.HttpClient.When(func(request *http.Request) bool {
		return request.Method == http.MethodGet &&
			request.URL.Path == fmt.Sprintf(
				"/projects/%s/users/%s/environments/%s",
				projectName,
				userId,
				environmentName,
			)
	}).RespondFn(func(request *http.Request) (*http.Response, error) {
		*mockRequest = *request

		response := environment

		if environment == nil {
			return mocks.CreateEmptyHttpResponse(request, http.StatusNotFound)
		}

		return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
	})

	return mockRequest
}

func MockListEnvironmentDefinitions(
	mockContext *mocks.MockContext,
	projectName string,
	environmentDefinitions []*devcentersdk.EnvironmentDefinition,
) *http.Request {
	mockRequest := &http.Request{}

	mockContext.HttpClient.When(func(request *http.Request) bool {
		return request.Method == http.MethodGet &&
			request.URL.Path == fmt.Sprintf("/projects/%s/environmentDefinitions", projectName)
	}).RespondFn(func(request *http.Request) (*http.Response, error) {
		*mockRequest = *request

		response := devcentersdk.EnvironmentDefinitionListResponse{
			Value: environmentDefinitions,
		}

		if environmentDefinitions == nil {
			return mocks.CreateEmptyHttpResponse(request, http.StatusNotFound)
		}

		return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
	})

	return mockRequest
}
