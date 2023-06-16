package tasks

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/messaging"
	"github.com/azure/azure-dev/cli/azd/pkg/operations"
)

type LongRunningTask struct {
	message        string
	console        input.Console
	subscriber     messaging.Subscriber
	subscription   *messaging.Subscription
	finalStepState input.SpinnerUxType
	finalMessage   string
}

func NewLongRunningTask(console input.Console, subscriber messaging.Subscriber) *LongRunningTask {
	return &LongRunningTask{
		console:        console,
		subscriber:     subscriber,
		finalStepState: input.StepDone,
	}
}

func (t *LongRunningTask) Run(ctx context.Context, taskFn TaskFunc) error {
	t.initialize(ctx)
	defer t.cleanup(ctx)

	t.console.ShowSpinner(ctx, t.message, input.StepDone)

	result, err := taskFn(ctx, t)
	if err != nil {
		t.console.StopSpinner(ctx, t.message, input.StepFailed)
		return err
	} else {
		displayMessage := t.message
		if t.finalMessage != "" {
			displayMessage = t.finalMessage
		}
		t.console.StopSpinner(ctx, displayMessage, t.finalStepState)
	}

	err = t.subscription.Flush(ctx)
	if err != nil {
		return err
	}

	if result != nil {
		t.console.MessageUxItem(ctx, result)
	}

	return nil
}

func (t *LongRunningTask) Progress(ctx context.Context, message string) {
	displayMessage := fmt.Sprintf("%s (%s)", t.message, message)
	t.console.ShowSpinner(ctx, displayMessage, input.Step)
}

func (t *LongRunningTask) Warn(ctx context.Context, message string) {
	t.finalize(ctx, message, input.StepWarning)
}

func (t *LongRunningTask) Skip(ctx context.Context, message string) {
	t.finalize(ctx, message, input.StepSkipped)
}

func (t *LongRunningTask) finalize(ctx context.Context, message string, stepState input.SpinnerUxType) {
	if message != "" {
		t.finalMessage = fmt.Sprintf("%s (%s)", t.message, message)
	}
	t.finalStepState = stepState
}

func (t *LongRunningTask) initialize(ctx context.Context) error {
	filter := func(ctx context.Context, envelope *messaging.Envelope) bool {
		return envelope.Type == operations.DefaultMessageKind
	}

	subscription, err := t.subscriber.Subscribe(ctx, filter, t.receiveMessage)
	if err != nil {
		return err
	}

	t.subscription = subscription
	return nil
}

func (t *LongRunningTask) cleanup(ctx context.Context) error {
	if t.subscription == nil {
		return nil
	}

	return t.subscription.Close(ctx)
}

func (t *LongRunningTask) receiveMessage(ctx context.Context, envelope *messaging.Envelope) {
	msg, ok := envelope.Value.(*operations.Message)
	if !ok {
		return
	}

	switch msg.State {
	case operations.StateRunning, operations.StateProgress:
		t.Progress(ctx, msg.Message)
	case operations.StateWarning:
		t.Warn(ctx, msg.Message)
	case operations.StateSkipped:
		t.Skip(ctx, msg.Message)
	}
}
