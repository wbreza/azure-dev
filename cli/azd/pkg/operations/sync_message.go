package operations

import "context"

type syncOperation struct {
	manager *syncManager
}

func newSyncOperation(manager *syncManager) *syncOperation {
	return &syncOperation{
		manager: manager,
	}
}

// Succeed reports the operation as successful
func (o *syncOperation) Succeed(ctx context.Context, message string) {
	o.manager.printer.ShowSuccess(ctx, message)
}

// Progress reports the operation as in progress
func (o *syncOperation) Progress(ctx context.Context, message string) {
	o.manager.printer.ShowProgress(ctx, message)
}

// Fail reports the operation as failed
func (o *syncOperation) Fail(ctx context.Context, message string) {
	o.manager.printer.ShowError(ctx, message)
}

// Skip reports the operation as skipped
func (o *syncOperation) Skip(ctx context.Context) {
	o.manager.printer.ShowSkipped(ctx, "skipped")
}

// Warn reports the operation has a warning
func (o *syncOperation) Warn(ctx context.Context, message string) {
	o.manager.printer.ShowWarning(ctx, message)
}
