package cmd

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/cmd/middleware"
	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type contextKey string

const actionName contextKey = "action"
const middlewareAName contextKey = "middleware-A"
const middlewareBName contextKey = "middleware-B"

func Test_BuildAndRunSimpleCommand(t *testing.T) {
	ran := false

	root := actions.NewActionDescriptor("root", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{
			RunE: func(cmd *cobra.Command, args []string) error {
				ran = true
				return nil
			},
		},
	})

	builder := NewCobraBuilder(ioc.Global)
	cmd, err := builder.BuildCommand(root)

	require.NotNil(t, cmd)
	require.NoError(t, err)

	err = cmd.ExecuteContext(context.Background())

	require.NoError(t, err)
	require.True(t, ran)
}

func Test_BuildAndRunSimpleAction(t *testing.T) {
	resetOsArgs(t)

	root := actions.NewActionDescriptor("root", &actions.ActionDescriptorOptions{
		ActionResolver: newTestAction,
		FlagsResolver:  newTestFlags,
	})

	builder := NewCobraBuilder(ioc.Global)
	cmd, err := builder.BuildCommand(root)

	require.NotNil(t, cmd)
	require.NoError(t, err)

	os.Args = []string{"", "-r"}
	err = cmd.ExecuteContext(context.Background())

	require.NoError(t, err)
}

func Test_BuildAndRunSimpleActionWithMiddleware(t *testing.T) {
	resetOsArgs(t)

	root := actions.NewActionDescriptor("root", &actions.ActionDescriptorOptions{
		ActionResolver: newTestAction,
		FlagsResolver:  newTestFlags,
	}).UseMiddleware("A", newTestMiddlewareA)

	builder := NewCobraBuilder(ioc.Global)
	cmd, err := builder.BuildCommand(root)

	require.NotNil(t, cmd)
	require.NoError(t, err)

	actionRan := false
	middlewareRan := false

	ctx := context.Background()
	ctx = context.WithValue(ctx, actionName, &actionRan)
	ctx = context.WithValue(ctx, middlewareAName, &middlewareRan)

	os.Args = []string{"", "-r"}
	err = cmd.ExecuteContext(ctx)

	require.NoError(t, err)
	require.True(t, actionRan)
	require.True(t, middlewareRan)
}

func Test_BuildAndRunActionWithNestedMiddleware(t *testing.T) {
	resetOsArgs(t)

	root := actions.NewActionDescriptor("root", nil).
		UseMiddleware("A", newTestMiddlewareA)

	root.Add("child", &actions.ActionDescriptorOptions{
		ActionResolver: newTestAction,
		FlagsResolver:  newTestFlags,
	}).UseMiddleware("B", newTestMiddlewareB)

	builder := NewCobraBuilder(ioc.Global)
	cmd, err := builder.BuildCommand(root)

	require.NotNil(t, cmd)
	require.NoError(t, err)

	actionRan := false
	middlewareARan := false
	middlewareBRan := false

	ctx := context.Background()
	ctx = context.WithValue(ctx, actionName, &actionRan)
	ctx = context.WithValue(ctx, middlewareAName, &middlewareARan)
	ctx = context.WithValue(ctx, middlewareBName, &middlewareBRan)

	os.Args = []string{"", "child", "-r"}
	err = cmd.ExecuteContext(ctx)

	require.NoError(t, err)
	require.True(t, actionRan)
	require.True(t, middlewareARan)
	require.True(t, middlewareBRan)
}

func Test_BuildAndRunActionWithNestedAndConditionalMiddleware(t *testing.T) {
	resetOsArgs(t)

	root := actions.NewActionDescriptor("root", nil).
		// This middleware will always run because its registered at the root
		UseMiddleware("A", newTestMiddlewareA)

	root.Add("child", &actions.ActionDescriptorOptions{
		ActionResolver: newTestAction,
		FlagsResolver:  newTestFlags,
	}).
		// This middleware is an example of a middleware that will only be registered if it passes
		// the predicate. Typically this would be based on a value in the action descriptor.
		UseMiddlewareWhen("B", newTestMiddlewareB, func(descriptor *actions.ActionDescriptor) bool {
			return false
		})

	builder := NewCobraBuilder(ioc.Global)
	cmd, err := builder.BuildCommand(root)

	require.NotNil(t, cmd)
	require.NoError(t, err)

	actionRan := false
	middlewareARan := false
	middlewareBRan := false

	ctx := context.Background()
	ctx = context.WithValue(ctx, actionName, &actionRan)
	ctx = context.WithValue(ctx, middlewareAName, &middlewareARan)
	ctx = context.WithValue(ctx, middlewareBName, &middlewareBRan)

	os.Args = []string{"", "child", "-r"}
	err = cmd.ExecuteContext(ctx)

	require.NoError(t, err)
	require.True(t, actionRan)
	require.True(t, middlewareARan)
	require.False(t, middlewareBRan)
}

func Test_BuildCommandsWithAutomaticHelpAndOutputFlags(t *testing.T) {
	root := actions.NewActionDescriptor("root", &actions.ActionDescriptorOptions{
		OutputFormats: []output.Format{output.JsonFormat, output.TableFormat},
		DefaultFormat: output.TableFormat,
	})

	cobraBuilder := NewCobraBuilder(ioc.Global)
	cmd, err := cobraBuilder.BuildCommand(root)

	require.NoError(t, err)
	require.NotNil(t, cmd)

	helpFlag := cmd.Flag("help")
	outputFlag := cmd.Flag("output")

	require.NotNil(t, helpFlag)
	require.Equal(t, "help", helpFlag.Name)
	require.Equal(t, "h", helpFlag.Shorthand)
	require.Equal(t, "Gets help for root.", helpFlag.Usage)

	require.NotNil(t, outputFlag)
	require.Equal(t, "output", outputFlag.Name)
	require.Equal(t, "o", outputFlag.Shorthand)
	require.Equal(t, "The output format (the supported formats are json, table).", outputFlag.Usage)
}

func resetOsArgs(t *testing.T) {
	defaultArgs := os.Args

	t.Cleanup(func() {
		os.Args = defaultArgs
	})
}

// Types for test

// Action

type testFlags struct {
	ran bool
}

func newTestFlags(cmd *cobra.Command) *testFlags {
	flags := &testFlags{}
	flags.Bind(cmd)

	return flags
}

func (a *testFlags) Bind(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&a.ran, "ran", "r", false, "sets whether the test command ran")
}

type testAction struct {
	flags *testFlags
}

func newTestAction(flags *testFlags) actions.Action {
	return &testAction{
		flags: flags,
	}
}

func (a *testAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	actionRan, ok := ctx.Value(actionName).(*bool)
	if ok {
		*actionRan = true
	}

	if !a.flags.ran {
		return nil, errors.New("flag was not set")
	}

	return nil, nil
}

// Middleware

type testMiddlewareA struct {
}

func newTestMiddlewareA() middleware.Middleware {
	return &testMiddlewareA{}
}

func (m *testMiddlewareA) Run(ctx context.Context, nextFn middleware.NextFn) (*actions.ActionResult, error) {
	middlewareRan, ok := ctx.Value(middlewareAName).(*bool)
	if ok {
		*middlewareRan = true
	}

	return nextFn(ctx)
}

type testMiddlewareB struct {
}

func newTestMiddlewareB() middleware.Middleware {
	return &testMiddlewareB{}
}

func (m *testMiddlewareB) Run(ctx context.Context, nextFn middleware.NextFn) (*actions.ActionResult, error) {
	middlewareRan, ok := ctx.Value(middlewareBName).(*bool)
	if ok {
		*middlewareRan = true
	}

	return nextFn(ctx)
}