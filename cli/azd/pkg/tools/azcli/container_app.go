package azcli

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers"
	azdinternal "github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/azure/azure-dev/cli/azd/pkg/convert"
	"github.com/azure/azure-dev/cli/azd/pkg/httputil"
	"github.com/benbjohnson/clock"
)

type ContainerAppService interface {
	// Gets the ingress configuration for the specified container app
	GetIngressConfiguration(
		ctx context.Context,
		subscriptionId,
		resourceGroup,
		appName string,
	) (*ContainerAppIngressConfiguration, error)
	// Adds and activates a new revision to the specified container app
	AddRevision(
		ctx context.Context,
		subscriptionId string,
		resourceGroupName string,
		appName string,
		imageName string,
	) error
}

func NewContainerAppService(
	credentialProvider account.SubscriptionCredentialProvider,
	httpClient httputil.HttpClient,
	clock clock.Clock,
) ContainerAppService {
	return &containerAppService{
		credentialProvider: credentialProvider,
		httpClient:         httpClient,
		userAgent:          azdinternal.MakeUserAgentString(""),
		clock:              clock,
	}
}

type containerAppService struct {
	credentialProvider account.SubscriptionCredentialProvider
	httpClient         httputil.HttpClient
	userAgent          string
	clock              clock.Clock
}

type ContainerAppIngressConfiguration struct {
	HostNames []string
}

// Gets the ingress configuration for the specified container app
func (cas *containerAppService) GetIngressConfiguration(
	ctx context.Context,
	subscriptionId string,
	resourceGroup string,
	appName string,
) (*ContainerAppIngressConfiguration, error) {
	containerApp, err := cas.getContainerApp(ctx, subscriptionId, resourceGroup, appName)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving container app properties: %w", err)
	}

	var hostNames []string
	if containerApp.Properties != nil &&
		containerApp.Properties.Configuration != nil &&
		containerApp.Properties.Configuration.Ingress != nil &&
		containerApp.Properties.Configuration.Ingress.Fqdn != nil {
		hostNames = []string{*containerApp.Properties.Configuration.Ingress.Fqdn}
	} else {
		hostNames = []string{}
	}

	return &ContainerAppIngressConfiguration{
		HostNames: hostNames,
	}, nil
}

// Adds and activates a new revision to the specified container app
func (cas *containerAppService) AddRevision(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	appName string,
	imageName string,
) error {
	appClient, err := cas.createContainerAppsClient(ctx, subscriptionId)
	if err != nil {
		return err
	}

	containerApp, err := cas.getContainerApp(ctx, subscriptionId, resourceGroupName, appName)
	if err != nil {
		return fmt.Errorf("getting container app: %w", err)
	}

	// Get the latest revision name
	currentRevisionName := *containerApp.Properties.LatestRevisionName
	revisionsClient, err := cas.createRevisionsClient(ctx, subscriptionId)
	if err != nil {
		return err
	}

	revisionResponse, err := revisionsClient.GetRevision(ctx, resourceGroupName, appName, currentRevisionName, nil)
	if err != nil {
		return fmt.Errorf("getting revision '%s': %w", currentRevisionName, err)
	}

	// Update the revision with the new image name and suffix
	revision := revisionResponse.Revision
	revision.Properties.Template.RevisionSuffix = convert.RefOf(fmt.Sprintf("azd-deploy-%d", cas.clock.Now().Unix()))
	revision.Properties.Template.Containers[0].Image = convert.RefOf(imageName)

	// Update the container app with the new revision
	containerApp.Properties.Template = revision.Properties.Template

	// Copy the secret configuration from the current version
	// Secret values are not returned by the API, so we need to get them separately
	// to ensure the update call succeeds
	secretsResponse, err := appClient.ListSecrets(ctx, resourceGroupName, appName, nil)
	if err != nil {
		return fmt.Errorf("listing secrets: %w", err)
	}

	secrets := []*armappcontainers.Secret{}
	for _, secret := range secretsResponse.SecretsCollection.Value {
		secrets = append(secrets, &armappcontainers.Secret{
			Name:  secret.Name,
			Value: secret.Value,
		})
	}

	containerApp.Properties.Configuration.Secrets = secrets

	// Update the container app
	err = cas.updateContainerApp(ctx, subscriptionId, resourceGroupName, appName, containerApp)
	if err != nil {
		return fmt.Errorf("updating container app revision: %w", err)
	}

	// If the container app is in multiple revision mode, update the traffic to point to the new revision
	if *containerApp.Properties.Configuration.ActiveRevisionsMode == armappcontainers.ActiveRevisionsModeMultiple {
		newRevisionName := fmt.Sprintf("%s--%s", appName, *revision.Properties.Template.RevisionSuffix)
		err = cas.setTrafficWeights(ctx, subscriptionId, resourceGroupName, appName, containerApp, newRevisionName)
		if err != nil {
			return fmt.Errorf("setting traffic weights: %w", err)
		}
	}

	return nil
}

func (cas *containerAppService) setTrafficWeights(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	appName string,
	containerApp *armappcontainers.ContainerApp,
	revisionName string,
) error {
	containerApp.Properties.Configuration.Ingress.Traffic = []*armappcontainers.TrafficWeight{
		{
			RevisionName: &revisionName,
			Weight:       convert.RefOf[int32](100),
		},
	}

	err := cas.updateContainerApp(ctx, subscriptionId, resourceGroupName, appName, containerApp)
	if err != nil {
		return fmt.Errorf("updating traffic weights: %w", err)
	}

	return nil
}

func (cas *containerAppService) getContainerApp(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	appName string,
) (*armappcontainers.ContainerApp, error) {
	appClient, err := cas.createContainerAppsClient(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	containerAppResponse, err := appClient.Get(ctx, resourceGroupName, appName, nil)
	if err != nil {
		return nil, fmt.Errorf("getting container app: %w", err)
	}

	return &containerAppResponse.ContainerApp, nil
}

func (cas *containerAppService) updateContainerApp(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	appName string,
	containerApp *armappcontainers.ContainerApp,
) error {
	appClient, err := cas.createContainerAppsClient(ctx, subscriptionId)
	if err != nil {
		return err
	}

	poller, err := appClient.BeginUpdate(ctx, resourceGroupName, appName, *containerApp, nil)
	if err != nil {
		return fmt.Errorf("begin updating ingress traffic: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("polling for container app update completion: %w", err)
	}

	return nil
}

func (cas *containerAppService) createContainerAppsClient(
	ctx context.Context,
	subscriptionId string,
) (*armappcontainers.ContainerAppsClient, error) {
	credential, err := cas.credentialProvider.CredentialForSubscription(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	options := clientOptionsBuilder(cas.httpClient, cas.userAgent).BuildArmClientOptions()
	client, err := armappcontainers.NewContainerAppsClient(subscriptionId, credential, options)
	if err != nil {
		return nil, fmt.Errorf("creating ContainerApps client: %w", err)
	}

	return client, nil
}

func (cas *containerAppService) createRevisionsClient(
	ctx context.Context,
	subscriptionId string,
) (*armappcontainers.ContainerAppsRevisionsClient, error) {
	credential, err := cas.credentialProvider.CredentialForSubscription(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	options := clientOptionsBuilder(cas.httpClient, cas.userAgent).BuildArmClientOptions()
	client, err := armappcontainers.NewContainerAppsRevisionsClient(subscriptionId, credential, options)
	if err != nil {
		return nil, fmt.Errorf("creating ContainerApps client: %w", err)
	}

	return client, nil
}
