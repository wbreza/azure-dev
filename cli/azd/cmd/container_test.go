package cmd

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/azure/azure-dev/cli/azd/internal/telemetry"
	"github.com/azure/azure-dev/cli/azd/internal/tracing"
	"github.com/azure/azure-dev/cli/azd/pkg/environment/azdcontext"
	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/azure/azure-dev/cli/azd/pkg/lazy"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/pkg/project"
	"github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func Test_Lazy_Project_Config_Resolution(t *testing.T) {
	ctx := context.Background()
	container := ioc.NewNestedContainer(nil)
	ioc.RegisterInstance(container, ctx)

	registerCommonDependencies(container)

	// Register the testing lazy component
	container.MustRegisterTransient(
		func(lazyProjectConfig *lazy.Lazy[*project.ProjectConfig]) *testLazyComponent[*project.ProjectConfig] {
			return &testLazyComponent[*project.ProjectConfig]{
				lazy: lazyProjectConfig,
			}
		},
	)

	// Register the testing concrete component
	container.MustRegisterTransient(
		func(projectConfig *project.ProjectConfig) *testConcreteComponent[*project.ProjectConfig] {
			return &testConcreteComponent[*project.ProjectConfig]{
				concrete: projectConfig,
			}
		},
	)

	// The lazy components depends on the lazy project config.
	// The lazy instance itself should never be nil
	var lazyComponent *testLazyComponent[*project.ProjectConfig]
	err := container.Resolve(&lazyComponent)
	require.NoError(t, err)
	require.NotNil(t, lazyComponent.lazy)

	// Get the lazy project config instance itself to use for comparison
	var lazyProjectConfig *lazy.Lazy[*project.ProjectConfig]
	err = container.Resolve(&lazyProjectConfig)
	require.NoError(t, err)
	require.NotNil(t, lazyProjectConfig)

	// At this point a project config is not available, so we should get an error
	projectConfig, err := lazyProjectConfig.GetValue()
	require.Nil(t, projectConfig)
	require.Error(t, err)

	// Set a project config on the lazy instance
	projectConfig = &project.ProjectConfig{
		Name: "test",
	}

	lazyProjectConfig.SetValue(projectConfig)

	// Now lets resolve a type that depends on a concrete project config
	// The project config should be be available not that the lazy has been set above
	var staticComponent *testConcreteComponent[*project.ProjectConfig]
	err = container.Resolve(&staticComponent)
	require.NoError(t, err)
	require.NotNil(t, staticComponent.concrete)

	// Now we validate that the instance returned by the lazy instance is the same as the one resolved directly
	lazyValue, err := lazyComponent.lazy.GetValue()
	require.NoError(t, err)
	directValue, err := lazyProjectConfig.GetValue()
	require.NoError(t, err)

	// Finally we validate that the return project config across all resolutions point to the same project config pointer
	require.Same(t, lazyProjectConfig, lazyComponent.lazy)
	require.Same(t, lazyValue, directValue)
	require.Same(t, directValue, staticComponent.concrete)
}

func Test_Lazy_AzdContext_Resolution(t *testing.T) {
	ctx := context.Background()
	container := ioc.NewNestedContainer(nil)
	ioc.RegisterInstance(container, ctx)

	registerCommonDependencies(container)

	// Register the testing lazy component
	container.MustRegisterTransient(
		func(lazyAzdContext *lazy.Lazy[*azdcontext.AzdContext]) *testLazyComponent[*azdcontext.AzdContext] {
			return &testLazyComponent[*azdcontext.AzdContext]{
				lazy: lazyAzdContext,
			}
		},
	)

	// Register the testing concrete component
	container.MustRegisterTransient(
		func(azdContext *azdcontext.AzdContext) *testConcreteComponent[*azdcontext.AzdContext] {
			return &testConcreteComponent[*azdcontext.AzdContext]{
				concrete: azdContext,
			}
		},
	)

	// The lazy components depends on the lazy project config.
	// The lazy instance itself should never be nil
	var lazyComponent *testLazyComponent[*azdcontext.AzdContext]
	err := container.Resolve(&lazyComponent)
	require.NoError(t, err)
	require.NotNil(t, lazyComponent.lazy)

	// Get the lazy project config instance itself to use for comparison
	var lazyInstance *lazy.Lazy[*azdcontext.AzdContext]
	err = container.Resolve(&lazyInstance)
	require.NoError(t, err)
	require.NotNil(t, lazyInstance)

	// At this point a project config is not available, so we should get an error
	azdContext, err := lazyInstance.GetValue()
	require.Nil(t, azdContext)
	require.Error(t, err)

	// Set a project config on the lazy instance
	azdContext = azdcontext.NewAzdContextWithDirectory(t.TempDir())

	lazyInstance.SetValue(azdContext)

	// Now lets resolve a type that depends on a concrete project config
	// The project config should be be available not that the lazy has been set above
	var staticComponent *testConcreteComponent[*azdcontext.AzdContext]
	err = container.Resolve(&staticComponent)
	require.NoError(t, err)
	require.NotNil(t, staticComponent.concrete)

	// Now we validate that the instance returned by the lazy instance is the same as the one resolved directly
	lazyValue, err := lazyComponent.lazy.GetValue()
	require.NoError(t, err)
	directValue, err := lazyInstance.GetValue()
	require.NoError(t, err)

	// Finally we validate that the return project config across all resolutions point to the same project config pointer
	require.Same(t, lazyInstance, lazyComponent.lazy)
	require.Same(t, lazyValue, directValue)
	require.Same(t, directValue, staticComponent.concrete)
}

func Test_ProjectConfig_Up_Expectations(t *testing.T) {
	ctx := context.Background()
	rootContainer := ioc.NewNestedContainer(nil)
	ioc.RegisterInstance(rootContainer, ctx)

	registerCommonDependencies(rootContainer)

	projectYaml := heredoc.Doc(`
        name: todo-nodejs-mongo
        metadata:
        template: todo-nodejs-mongo@0.0.1-beta
        services:
            web:
                project: ./src/web
                dist: build
                language: js
                host: appservice
            api:
                project: ./src/api
                language: js
                host: appservice
    `)

	tempDir := tempDirWithDiagnostics(t)
	err := os.Chdir(tempDir)
	require.NoError(t, err)

	projectFilePath := filepath.Join(tempDir, "azure.yaml")
	err = os.WriteFile(projectFilePath, []byte(projectYaml), osutil.PermissionFile)
	require.NoError(t, err)

	packageScope, err := rootContainer.NewScope()
	require.NoError(t, err)

	var project1 *project.ProjectConfig
	err = packageScope.Resolve(&project1)
	require.NoError(t, err)

	provisionScope, err := rootContainer.NewScope()
	require.NoError(t, err)

	var project2 *project.ProjectConfig
	err = provisionScope.Resolve(&project2)
	require.NoError(t, err)

	deployScope, err := rootContainer.NewScope()
	require.NoError(t, err)

	var project3 *project.ProjectConfig
	err = deployScope.Resolve(&project3)
	require.NoError(t, err)

	require.NotSame(t, project1, project2)
	require.NotSame(t, project2, project3)
}

type testLazyComponent[T comparable] struct {
	lazy *lazy.Lazy[T]
}

type testConcreteComponent[T comparable] struct {
	concrete T
}

// TempDirWithDiagnostics creates a temp directory with cleanup that also provides additional
// diagnostic logging and retries.
func tempDirWithDiagnostics(t *testing.T) string {
	temp := t.TempDir()

	if runtime.GOOS == "windows" {
		// Enable our additional custom remove logic for Windows where we see locked files.
		t.Cleanup(func() {
			err := removeAllWithDiagnostics(t, temp)
			if err != nil {
				logHandles(t, temp)
				t.Fatalf("TempDirWithDiagnostics: %s", err)
			}
		})
	}

	return temp
}

func logHandles(t *testing.T, path string) {
	handle, err := osexec.LookPath("handle")
	if err != nil && errors.Is(err, osexec.ErrNotFound) {
		t.Logf("handle.exe not present. Skipping handle detection. PATH: %s", os.Getenv("PATH"))
		return
	}

	if err != nil {
		t.Logf("failed to find handle.exe: %s", err)
		return
	}

	args := exec.NewRunArgs(handle, path, "-nobanner")
	cmd := exec.NewCommandRunner(nil)
	rr, err := cmd.Run(context.Background(), args)
	if err != nil {
		t.Logf("handle.exe failed. stdout: %s, stderr: %s\n", rr.Stdout, rr.Stderr)
		return
	}

	t.Logf("handle.exe output:\n%s\n", rr.Stdout)

	// Ensure telemetry is initialized since we're running in a CI environment
	_ = telemetry.GetTelemetrySystem()

	// Log this to telemetry for ease of correlation
	_, span := tracing.Start(context.Background(), "test.file_cleanup_failure")
	span.SetAttributes(attribute.String("handle.stdout", rr.Stdout))
	span.SetAttributes(attribute.String("ci.build.number", os.Getenv("BUILD_BUILDNUMBER")))
	span.End()
}

func removeAllWithDiagnostics(t *testing.T, path string) error {
	retryCount := 0
	loggedOnce := false
	return retry.Do(
		context.Background(),
		retry.WithMaxRetries(10, retry.NewConstant(1*time.Second)),
		func(_ context.Context) error {
			removeErr := os.RemoveAll(path)
			if removeErr == nil {
				return nil
			}
			t.Logf("failed to clean up %s with error: %v", path, removeErr)

			if retryCount >= 2 && !loggedOnce {
				// Only log once after 2 seconds - logHandles is pretty expensive and slow
				logHandles(t, path)
				loggedOnce = true
			}

			retryCount++
			return retry.RetryableError(removeErr)
		},
	)
}
