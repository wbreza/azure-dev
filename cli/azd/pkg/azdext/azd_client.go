package azdext

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AzdClient struct {
	connection        *grpc.ClientConn
	projectClient     ProjectServiceClient
	environmentClient EnvironmentServiceClient
	userConfigClient  UserConfigServiceClient
	promptClient      PromptServiceClient
	deploymentClient  DeploymentServiceClient
}

func NewAzdClient(serverAddress string) (*AzdClient, error) {
	connection, err := grpc.NewClient(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return &AzdClient{
		connection: connection,
	}, nil
}

func (c *AzdClient) Close() {
	c.connection.Close()
}

func (c *AzdClient) Project() ProjectServiceClient {
	if c.projectClient == nil {
		c.projectClient = NewProjectServiceClient(c.connection)
	}

	return c.projectClient
}

func (c *AzdClient) Environment() EnvironmentServiceClient {
	if c.environmentClient == nil {
		c.environmentClient = NewEnvironmentServiceClient(c.connection)
	}

	return c.environmentClient
}

func (c *AzdClient) UserConfig() UserConfigServiceClient {
	if c.userConfigClient == nil {
		c.userConfigClient = NewUserConfigServiceClient(c.connection)
	}

	return c.userConfigClient
}

func (c *AzdClient) Prompt() PromptServiceClient {
	if c.promptClient == nil {
		c.promptClient = NewPromptServiceClient(c.connection)
	}

	return c.promptClient
}

func (c *AzdClient) Deployment() DeploymentServiceClient {
	if c.deploymentClient == nil {
		c.deploymentClient = NewDeploymentServiceClient(c.connection)
	}

	return c.deploymentClient
}
