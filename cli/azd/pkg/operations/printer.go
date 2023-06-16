package operations

import (
	"context"
	"fmt"
	"sync"

	"github.com/azure/azure-dev/cli/azd/pkg/ext"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
)

type PrinterEventArgs struct {
}

var (
	PrinterInitializeEvent ext.Event = "initialize"
	PrinterFlushEvent      ext.Event = "flush"
)

type printer struct {
	*ext.EventDispatcher[PrinterEventArgs] `yaml:",omitempty"`
	console                                input.Console
	currentMessage                         string
	lock                                   sync.Mutex
}

func NewPrinter(console input.Console) Printer {
	return &printer{
		console:         console,
		EventDispatcher: ext.NewEventDispatcher[PrinterEventArgs](PrinterInitializeEvent, PrinterFlushEvent),
	}
}

func (p *printer) Initialize(ctx context.Context) error {
	return p.RaiseEvent(ctx, PrinterInitializeEvent, PrinterEventArgs{})
}

func (p *printer) Flush(ctx context.Context) error {
	return p.RaiseEvent(ctx, PrinterFlushEvent, PrinterEventArgs{})
}

func (p *printer) ShowRunning(ctx context.Context, message string) {
	if p.currentMessage == "" {
		// New Spinner
		p.lock.Lock()
		p.currentMessage = message
		p.console.ShowSpinner(ctx, p.currentMessage, input.Step)
	} else {
		// Update existing spinner
		p.ShowProgress(ctx, message)
	}
}

func (p *printer) ShowProgress(ctx context.Context, message string) {
	if p.currentMessage == "" {
		return
	}

	displayMessage := fmt.Sprintf("%s (%s)", p.currentMessage, message)
	p.console.ShowSpinner(ctx, displayMessage, input.Step)
}

func (p *printer) ShowSuccess(ctx context.Context, message string) {
	p.stopAndReset(ctx, message, input.StepDone)
}

func (p *printer) ShowError(ctx context.Context, message string) {
	p.stopAndReset(ctx, message, input.StepFailed)
}

func (p *printer) ShowSkipped(ctx context.Context, message string) {
	p.stopAndReset(ctx, message, input.StepSkipped)
}

func (p *printer) ShowWarning(ctx context.Context, message string) {
	p.stopAndReset(ctx, message, input.StepWarning)
}

func (p *printer) stopAndReset(ctx context.Context, message string, stepType input.SpinnerUxType) {
	if p.currentMessage == "" {
		return
	}

	p.console.StopSpinner(ctx, p.currentMessage, input.StepDone)
	p.currentMessage = ""
	p.lock.Unlock()
}
