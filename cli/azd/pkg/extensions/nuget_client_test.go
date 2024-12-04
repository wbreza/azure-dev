package extensions

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

const feedUrl = "https://pkgs.dev.azure.com/wallace/wabrez/_packaging/azd/nuget/v3/index.json"

func Test_ListPackages(t *testing.T) {
	ctx := context.Background()
	nugetClient := NewNuGetClient(http.DefaultClient)

	feed, err := nugetClient.GetFeed(ctx, feedUrl)
	require.NoError(t, err)
	require.NotNil(t, feed)

	packages, err := feed.ListPackages(ctx, "", 10)
	require.NoError(t, err)
	require.NotNil(t, packages)

	packageInfo, err := feed.GetPackage(ctx, packages[0].Id)
	require.NoError(t, err)
	require.NotNil(t, packageInfo)
}
