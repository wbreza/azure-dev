package middleware

import (
	"context"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal/telemetry"
	"github.com/azure/azure-dev/cli/azd/internal/telemetry/events"
	"go.opentelemetry.io/otel/codes"
)

type TelemetryMiddleware struct {
	buildOptions *actions.BuildOptions
}

func NewTelemetryMiddleware(buildOptions *actions.BuildOptions) *TelemetryMiddleware {
	return &TelemetryMiddleware{
		buildOptions: buildOptions,
	}
}

func (m *TelemetryMiddleware) Run(ctx context.Context, options Options, next NextFn) (*actions.ActionResult, error) {
	// When telemetry is disabled for an action just continue the middleware chain
	if m.buildOptions != nil && m.buildOptions.DisableTelemetry {
		return next(ctx)
	}

	// Note: CommandPath is constructed using the Use member on each command up to the root.
	// It does not contain user input, and is safe for telemetry emission.
	spanCtx, span := telemetry.GetTracer().Start(ctx, events.GetCommandEventName(options.Name))
	defer span.End()

	result, err := next(spanCtx)
	if err != nil {
		span.SetStatus(codes.Error, "UnknownError")
	}

	return result, err
}
