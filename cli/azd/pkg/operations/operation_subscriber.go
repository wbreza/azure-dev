package operations

import (
	"context"
	"fmt"
	"log"

	"github.com/azure/azure-dev/cli/azd/pkg/messaging"
	"github.com/google/uuid"
)

type operationSubscriber struct {
	printer            Printer
	subscriber         messaging.Subscriber
	subscription       *messaging.Subscription
	currentOperationId uuid.UUID
}

func NewSubscriber(ctx context.Context, subscriber messaging.Subscriber, printer Printer) *operationSubscriber {
	operationSubscriber := &operationSubscriber{
		printer:    printer,
		subscriber: subscriber,
	}

	operationSubscriber.Start(ctx)
	return operationSubscriber
}

// Starts listening for messages to print to the console
func (p *operationSubscriber) Start(ctx context.Context) error {
	if p.subscription != nil {
		return fmt.Errorf("printer already started")
	}

	filter := func(ctx context.Context, message *messaging.Envelope) bool {
		return message.Type == defaultMessageKind
	}

	subscription, err := p.subscriber.Subscribe(ctx, filter, p.receiveMessage)
	if err != nil {
		return err
	}

	p.subscription = subscription
	return nil
}

// Stops listening for messages
func (p *operationSubscriber) Stop(ctx context.Context) error {
	if p.subscription == nil {
		return fmt.Errorf("printer not started")
	}

	subscrption := p.subscription
	p.subscription = nil
	return subscrption.Close(ctx)
}

// Flushes any pending messages and blocks until they have all been handled
func (p *operationSubscriber) Flush(ctx context.Context) error {
	if p.subscription == nil {
		return fmt.Errorf("printer not started")
	}

	return p.subscription.Flush(ctx)
}

// Receives messages from the message bus and prints them to the console
func (p *operationSubscriber) receiveMessage(ctx context.Context, envelope *messaging.Envelope) {
	msg, ok := envelope.Value.(*Message)
	if !ok {
		return
	}

	switch msg.State {
	case StateRunning:
		// New operation, start spinner
		if p.currentOperationId == uuid.Nil {
			p.currentOperationId = msg.CorrelationId
			p.printer.ShowRunning(ctx, msg.Message)
		} else { // Existing operation in progress, report as progress
			p.printer.ShowProgress(ctx, msg.Message)
		}
	case StateProgress:
		// Only display progress when we are already running an operation
		if p.currentOperationId != uuid.Nil {
			p.printer.ShowProgress(ctx, msg.Message)
		}
	case StateSuccess, StateError, StateWarning, StateSkipped:
		if p.currentOperationId != msg.CorrelationId {
			return
		}

		switch msg.State {
		case StateSuccess:
			p.printer.ShowSuccess(ctx, msg.Message)
		case StateError:
			p.printer.ShowError(ctx, msg.Message)
		case StateWarning:
			p.printer.ShowWarning(ctx, msg.Message)
		case StateSkipped:
			p.printer.ShowSkipped(ctx, msg.Message)
		}

		// Only stop the spinner and reset the state if messages are from the same operation
		p.reset()
	default:
		log.Printf("unknown operation state %s", msg.State)
	}
}

func (p *operationSubscriber) reset() {
	p.currentOperationId = uuid.Nil
}
