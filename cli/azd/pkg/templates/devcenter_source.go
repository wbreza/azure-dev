package templates

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/azure/azure-dev/cli/azd/pkg/devcentersdk"
	"go.uber.org/multierr"
	"golang.org/x/exp/slices"
)

type DevCenterSource struct {
	devCenterClient devcentersdk.DevCenterClient
}

func NewDevCenterSource(devCenterClient devcentersdk.DevCenterClient) *DevCenterSource {
	return &DevCenterSource{
		devCenterClient: devCenterClient,
	}
}

func (s *DevCenterSource) Name() string {
	return "DevCenter"
}

func (s *DevCenterSource) ListTemplates(ctx context.Context) ([]*Template, error) {
	projects, err := s.devCenterClient.WritableProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed getting writable projects: %w", err)
	}

	templatesChan := make(chan *Template)
	errorsChan := make(chan error)

	// Perform the lookup and checking for projects in parallel to speed up the process
	var wg sync.WaitGroup

	for _, project := range projects {
		wg.Add(1)

		go func(project *devcentersdk.Project) {
			defer wg.Done()

			envDefinitions, err := s.devCenterClient.
				DevCenterByEndpoint(project.DevCenter.ServiceUri).
				ProjectByName(project.Name).
				EnvironmentDefinitions().
				Get(ctx)

			if err != nil {
				errorsChan <- err
				return
			}

			for _, envDefinition := range envDefinitions.Value {
				// We only want to consider environment definitions that have
				// a repo url parameter as valid templates for azd
				var repoUrls []string
				containsRepoUrl := slices.ContainsFunc(envDefinition.Parameters, func(p devcentersdk.Parameter) bool {
					if strings.EqualFold(p.Name, "repourl") {

						// Repo url parameter can support multiple values
						// Values can either have a default or multiple allowed values but not both
						if p.Default != nil {
							repoUrls = append(repoUrls, p.Default.(string))
						} else {
							repoUrls = append(repoUrls, p.Allowed...)
						}
						return true
					}

					return false
				})

				if containsRepoUrl {
					definitionParts := []string{
						project.DevCenter.Name,
						project.Name,
						envDefinition.CatalogName,
						envDefinition.Name,
					}
					definitionPath := strings.Join(definitionParts, "/")

					// List an available AZD template for each repo url that is referenced in the template
					for _, url := range repoUrls {
						templatesChan <- &Template{
							Id:             definitionPath,
							Name:           fmt.Sprintf("%s (%s)", envDefinition.Name, project.Name),
							Source:         fmt.Sprintf("%s/%s/%s", project.DevCenter.Name, project.Name, envDefinition.CatalogName),
							Description:    envDefinition.Description,
							RepositoryPath: url,

							// Metadata will be used when creating any azd environments that are based on this template
							Metadata: Metadata{
								Project: map[string]string{
									"devCenter.name":                  project.DevCenter.Name,
									"devCenter.project":               project.Name,
									"devCenter.catalog":               envDefinition.CatalogName,
									"devCenter.environmentDefinition": envDefinition.Name,
									"devCenter.repoUrl":               url,
								},
							},
						}
					}
				}
			}
		}(project)
	}

	go func() {
		wg.Wait()
		close(templatesChan)
		close(errorsChan)
	}()

	templates := []*Template{}
	for template := range templatesChan {
		templates = append(templates, template)
	}

	var allErrors error
	for err := range errorsChan {
		allErrors = multierr.Append(allErrors, err)
	}

	if allErrors != nil {
		return nil, allErrors
	}

	return templates, nil
}

func (s *DevCenterSource) GetTemplate(ctx context.Context, path string) (*Template, error) {
	templates, err := s.ListTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to list templates: %w", err)
	}

	for _, template := range templates {
		if template.Id == path {
			return template, nil
		}

		if template.RepositoryPath == path {
			return template, nil
		}
	}

	return nil, fmt.Errorf("template with path '%s' was not found, %w", path, ErrTemplateNotFound)
}
