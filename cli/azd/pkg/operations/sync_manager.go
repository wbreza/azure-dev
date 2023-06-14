package operations

import "context"

type syncManager struct {
	printer Printer
}

func NewSyncManager(printer Printer) Manager {
	return &syncManager{
		printer: printer,
	}
}

func (m *syncManager) ReportProgress(ctx context.Context, message string) {
	m.printer.ShowRunning(ctx, message)
}

// Run executes an operation and sends running, success, or error messages
func (m *syncManager) Run(ctx context.Context, operationMessage string, operationFunc OperationRunFunc) error {
	operation := newSyncOperation(m)
	m.printer.ShowRunning(ctx, operationMessage)

	if err := operationFunc(operation); err != nil {
		m.printer.ShowError(ctx, operationMessage)
		return err
	}

	m.printer.ShowSuccess(ctx, operationMessage)
	return nil
}
