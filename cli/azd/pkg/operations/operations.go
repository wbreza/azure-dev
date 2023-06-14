package operations

import (
	"context"
)

// Operation represent an atomic long running operation
type Operation interface {
	// Succeed reports the operation as successful
	Succeed(ctx context.Context, message string)

	// Progress reports the operation as in progress
	Progress(ctx context.Context, message string)

	// Fail reports the operation as failed
	Fail(ctx context.Context, message string)

	// Skip reports the operation as skipped
	Skip(ctx context.Context)

	// Warn reports the operation has a warning
	Warn(ctx context.Context, message string)
}

type OperationRunFunc func(operation Operation) error

// Manager orchestrates running operations and sending progress updates
type Manager interface {
	// ReportProgress sends a progress update message
	ReportProgress(ctx context.Context, message string)

	// Run executes an operation and sends running, success, or error messages
	Run(ctx context.Context, operationMessage string, operationFunc OperationRunFunc) error
}

// Printers orchestrates rendering operation updates in the UX CLI
type Printer interface {
	ShowRunning(ctx context.Context, message string)
	ShowProgress(ctx context.Context, message string)
	ShowError(ctx context.Context, message string)
	ShowSuccess(ctx context.Context, message string)
	ShowSkipped(ctx context.Context, message string)
	ShowWarning(ctx context.Context, message string)
}
