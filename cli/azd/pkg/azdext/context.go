package azdext

import context "context"

type Context struct {
	client       *AzdClient
	project      *ProjectConfig
	environment  *Environment
	azureContext *AzureContext
}

func CurrentContext() (*Context, error) {
	return &Context{}, nil
}

func (c *Context) Client() (*AzdClient, error) {
	if c.client == nil {
		client, err := NewAzdClient()
		if err != nil {
			return nil, err
		}

		c.client = client
	}

	return c.client, nil
}

func (c *Context) Project(ctx context.Context) (*ProjectConfig, error) {
	if c.project == nil {
		client, err := c.Client()
		if err != nil {
			return nil, err
		}

		projectResponse, err := client.Project().Get(ctx, &EmptyRequest{})
		if err != nil {
			return nil, err
		}

		c.project = projectResponse.Project
	}

	return c.project, nil
}

func (c *Context) Environment(ctx context.Context) (*Environment, error) {
	if c.environment == nil {
		client, err := c.Client()
		if err != nil {
			return nil, err
		}

		environmentResponse, err := client.Environment().GetCurrent(ctx, &EmptyRequest{})
		if err != nil {
			return nil, err
		}

		c.environment = environmentResponse.Environment
	}

	return c.environment, nil
}

func (c *Context) AzureContext(ctx context.Context) (*AzureContext, error) {
	if c.azureContext == nil {
		client, err := c.Client()
		if err != nil {
			return nil, err
		}

		deploymentContext, err := client.Deployment().GetDeploymentContext(ctx, &EmptyRequest{})
		if err != nil {
			return nil, err
		}

		c.azureContext = deploymentContext.AzureContext
	}

	return c.azureContext, nil
}
