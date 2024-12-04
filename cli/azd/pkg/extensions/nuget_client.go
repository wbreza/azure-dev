package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

const (
	searchQueryService = "SearchQueryService"
	registrationBase   = "RegistrationsBaseUrl"
	flatContainer      = "PackageBaseAddress"
)

// NuGetClient holds feed and API endpoints
type NuGetClient struct {
	pipeline runtime.Pipeline
}

type nugetResource struct {
	ID   string `json:"@id"`
	Type string `json:"@type"`
}

type listResourcesResponse struct {
	Resources []nugetResource `json:"resources"`
	Version   string          `json:"version"`
}

type listPackagesResponse struct {
	Data []*PackageInfo `json:"data"`
}

type getCatalogEntryResponse struct {
	Count int                `json:"count"`
	Items []*catalogItemList `json:"items"`
}

type catalogItemList struct {
	Count  int            `json:"count"`
	Items  []*catalogItem `json:"items"`
	Parent string         `json:"parent"`
	Lower  string         `json:"lower"`
	Upper  string         `json:"upper"`
}

type catalogItem struct {
	CatalogEntry      *PackageInfo `json:"catalogEntry"`
	PackageContentUrl string       `json:"packageContent"`
	RegistrationUrl   string       `json:"registration"`
}

type NugetFeed struct {
	feedUrl  string
	pipeline runtime.Pipeline

	searchService     string
	registrationBase  string
	flatContainerBase string
}

// Initialize the NuGet client by discovering service URLs
func NewNuGetClient(transport policy.Transporter) *NuGetClient {
	pipeline := runtime.NewPipeline("nuget-client", "1.0.0", runtime.PipelineOptions{}, &policy.ClientOptions{
		Transport: transport,
	})

	return &NuGetClient{
		pipeline: pipeline,
	}
}

func (c *NuGetClient) GetFeed(ctx context.Context, feedUrl string) (*NugetFeed, error) {
	getFeedReq, err := runtime.NewRequest(ctx, http.MethodGet, feedUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.pipeline.Do(getFeedReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed index: %w", err)
	}
	defer resp.Body.Close()

	var index struct {
		Resources []struct {
			Type string `json:"@type"`
			ID   string `json:"@id"`
		} `json:"resources"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to parse feed index: %w", err)
	}

	feed := &NugetFeed{
		pipeline: c.pipeline,
	}

	for _, resource := range index.Resources {
		parts := strings.Split(resource.Type, "/")
		resourceTypeName := parts[0]

		switch resourceTypeName {
		case searchQueryService:
			feed.searchService = resource.ID
		case registrationBase:
			feed.registrationBase = resource.ID
		case flatContainer:
			feed.flatContainerBase = resource.ID
		}
	}

	return feed, nil
}

// List all packages on the NuGet feed
func (c *NugetFeed) ListPackages(ctx context.Context, query string, take int) ([]*PackageInfo, error) {
	url := fmt.Sprintf("%s?q=%s&take=%d", c.searchService, query, take)
	req, err := runtime.NewRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query packages: %w", err)
	}
	defer resp.Body.Close()

	var result listPackagesResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse package list: %w", err)
	}

	return result.Data, nil
}

// Get metadata of a specific package
func (c *NugetFeed) GetPackage(ctx context.Context, packageId string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s%s/index.json", c.registrationBase, packageId)
	req, err := runtime.NewRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version metadata: %w", err)
	}
	defer resp.Body.Close()

	var catalogResponse getCatalogEntryResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalogResponse); err != nil {
		return nil, fmt.Errorf("failed to parse version metadata: %w", err)
	}

	if catalogResponse.Count == 0 {
		return nil, fmt.Errorf("package not found")
	}

	catalogEntry := catalogResponse.Items[0]
	if len(catalogEntry.Items) > 0 {
		return catalogEntry.Items[0].CatalogEntry, nil
	}

	return nil, fmt.Errorf("version not found")
}

// Download a NuGet package
func (c *NugetFeed) DownloadPackage(ctx context.Context, packageID, version, outputPath string) error {
	url := fmt.Sprintf("%s%s/%s/%s.%s.nupkg", c.flatContainerBase, packageID, version, packageID, version)
	req, err := runtime.NewRequest(ctx, http.MethodGet, url)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.pipeline.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download package: %w", err)
	}
	defer resp.Body.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to save package: %w", err)
	}

	return nil
}
