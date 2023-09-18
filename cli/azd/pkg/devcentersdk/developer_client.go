package devcentersdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/azure/azure-dev/cli/azd/pkg/azsdk"
	"github.com/azure/azure-dev/cli/azd/pkg/convert"
)

type DevCenterClient interface {
	DevCenters() *DevCenterListRequestBuilder
	DevCenterByEndpoint(endpoint string) *DevCenterItemRequestBuilder
	DevCenterByName(name string) *DevCenterItemRequestBuilder
}

type devCenterClient struct {
	credential azcore.TokenCredential
	options    *azcore.ClientOptions
	pipeline   runtime.Pipeline
	cache      map[string]interface{}
}

func NewDevCenterClient(
	credential azcore.TokenCredential,
	options *azcore.ClientOptions,
) (DevCenterClient, error) {
	if options == nil {
		options = &azcore.ClientOptions{}
	}

	options.PerCallPolicies = append(options.PerCallPolicies, NewApiVersionPolicy(nil))
	pipeline := NewPipeline(credential, ServiceConfig, options)

	return &devCenterClient{
		pipeline:   pipeline,
		credential: credential,
		options:    options,
		cache:      map[string]interface{}{},
	}, nil
}

func (c *devCenterClient) DevCenters() *DevCenterListRequestBuilder {
	return NewDevCenterListRequestBuilder(c)
}

func (c *devCenterClient) DevCenterByEndpoint(endpoint string) *DevCenterItemRequestBuilder {
	return NewDevCenterItemRequestBuilder(c, &DevCenter{ServiceUri: endpoint})
}

func (c *devCenterClient) DevCenterByName(name string) *DevCenterItemRequestBuilder {
	return NewDevCenterItemRequestBuilder(c, &DevCenter{Name: name})
}

func (c *devCenterClient) projectList(ctx context.Context) ([]*Project, error) {
	projects, ok := c.cache["projects"].([]*Project)
	if ok {
		return projects, nil
	}

	query := `
	Resources
	| where type in~ ('microsoft.devcenter/projects')
	| where properties['provisioningState'] =~ 'Succeeded'
	| project id, location, tenantId, name, properties, type
	`
	options := azsdk.DefaultClientOptionsBuilder(ctx, http.DefaultClient, "azd").BuildArmClientOptions()
	resourceGraphClient, err := armresourcegraph.NewClient(c.credential, options)
	if err != nil {
		return nil, err
	}

	queryRequest := armresourcegraph.QueryRequest{
		Query: &query,
		Options: &armresourcegraph.QueryRequestOptions{
			AllowPartialScopes: convert.RefOf(true),
		},
	}
	res, err := resourceGraphClient.Resources(ctx, queryRequest, nil)
	if err != nil {
		return nil, err
	}

	list, ok := res.QueryResponse.Data.([]interface{})
	if !ok {
		return nil, errors.New("error converting data to list")
	}

	jsonBytes, err := json.Marshal(list)
	if err != nil {
		return nil, fmt.Errorf("failed marshalling list: %w", err)
	}

	var resources []*GenericResource
	err = json.Unmarshal(jsonBytes, &resources)
	if err != nil {
		return nil, fmt.Errorf("failed unmarshalling list: %w", err)
	}

	projects = []*Project{}
	for _, resource := range resources {
		projectId, err := resourceFromId(resource.Id)
		if err != nil {
			return nil, fmt.Errorf("failed parsing resource id: %w", err)
		}

		devCenterId, err := resourceFromId(resource.Properties["devCenterId"].(string))
		if err != nil {
			return nil, fmt.Errorf("failed parsing dev center id: %w", err)
		}

		project := &Project{
			Id:             resource.Id,
			Name:           resource.Name,
			ResourceGroup:  projectId.ResourceGroup,
			SubscriptionId: projectId.SubscriptionId,
			Description:    convert.ToStringWithDefault(resource.Properties["description"], ""),
			DevCenter: &DevCenter{
				Id:             devCenterId.Id,
				SubscriptionId: devCenterId.SubscriptionId,
				ResourceGroup:  devCenterId.ResourceGroup,
				Name:           devCenterId.ResourceName,
				ServiceUri:     strings.TrimSuffix(convert.ToStringWithDefault(resource.Properties["devCenterUri"], ""), "/"),
			},
		}

		projects = append(projects, project)
	}

	c.cache["projects"] = projects
	return projects, nil
}

func (c *devCenterClient) projectListByDevCenter(ctx context.Context, devCenter *DevCenter) ([]*Project, error) {
	allProjects, err := c.projectList(ctx)
	if err != nil {
		return nil, err
	}

	filteredProjects := []*Project{}
	for _, project := range allProjects {
		hasMatchingServiceUri := devCenter.ServiceUri != "" && project.DevCenter.ServiceUri == devCenter.ServiceUri
		hasMatchingDevCenterName := devCenter.Name != "" && project.DevCenter.Name == devCenter.Name

		if hasMatchingServiceUri || hasMatchingDevCenterName {
			filteredProjects = append(filteredProjects, project)
		}
	}

	return filteredProjects, nil
}

func (c *devCenterClient) projectByDevCenter(ctx context.Context, devCenter *DevCenter, projectName string) (*Project, error) {
	projects, err := c.projectListByDevCenter(ctx, devCenter)
	if err != nil {
		return nil, err
	}

	matchingIndex := slices.IndexFunc(projects, func(project *Project) bool {
		return project.Name == projectName
	})

	if matchingIndex < 0 {
		return nil, fmt.Errorf("failed to find project '%s'", projectName)
	}

	return projects[matchingIndex], nil
}

func (c *devCenterClient) devCenterList(ctx context.Context) ([]*DevCenter, error) {
	devCenters, ok := c.cache["devcenters"].([]*DevCenter)
	if ok {
		return devCenters, nil
	}

	devCenters = []*DevCenter{}
	projects, err := c.projectList(ctx)
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		exists := slices.ContainsFunc(devCenters, func(devcenter *DevCenter) bool {
			return devcenter.ServiceUri == project.DevCenter.ServiceUri
		})

		if !exists {
			devCenters = append(devCenters, project.DevCenter)
		}
	}

	c.cache["devcenters"] = devCenters
	return devCenters, nil
}

func (c *devCenterClient) host(ctx context.Context, devCenter *DevCenter) (string, error) {
	devCenterList, err := c.devCenterList(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get dev center list: %w", err)
	}

	index := slices.IndexFunc(devCenterList, func(dc *DevCenter) bool {
		if devCenter.ServiceUri != "" {
			return devCenter.ServiceUri == dc.ServiceUri
		} else if devCenter.Name != "" {
			return devCenter.Name == dc.Name
		}

		return false
	})

	if index < 0 {
		return "", errors.New("failed to find dev center")
	}

	return devCenterList[index].ServiceUri, nil
}
