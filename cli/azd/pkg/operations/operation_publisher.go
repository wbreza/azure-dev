package operations

import (
	"context"
	"log"

	"github.com/azure/azure-dev/cli/azd/pkg/messaging"
)

type operationPublisher struct {
	publisher  messaging.Publisher
	subscriber *operationSubscriber
}

func NewPublisher(publisher messaging.Publisher, operationSubscriber *operationSubscriber) Manager {
	return &operationPublisher{
		publisher:  publisher,
		subscriber: operationSubscriber,
	}
}

func (om *operationPublisher) Send(ctx context.Context, message *Message) error {
	envelope := messaging.NewEnvelope(defaultMessageKind, message)
	return om.publisher.Send(ctx, envelope)
}

func (om *operationPublisher) ReportProgress(ctx context.Context, progressMessage string) {
	envelope, _ := NewMessage(progressMessage, StateProgress)
	if err := om.publisher.Send(ctx, envelope); err != nil {
		log.Printf("failed sending progress message: %s", err.Error())
	}
}

func (om *operationPublisher) Run(ctx context.Context, operationMessage string, operationFunc OperationRunFunc) error {
	operation := newMessageOperation(om)

	envelope, _ := NewCorrelatedMessage(operation.correlationId, operationMessage, StateRunning)
	if err := om.publisher.Send(ctx, envelope); err != nil {
		log.Printf("failed sending start message: %s", err.Error())
	}

	if err := operationFunc(operation); err != nil {
		envelope, _ := NewCorrelatedMessage(operation.correlationId, operationMessage, StateError)
		if err := om.publisher.Send(ctx, envelope); err != nil {
			log.Printf("failed sending error message: %s", err.Error())
		}

		return err
	}

	envelope, _ = NewCorrelatedMessage(operation.correlationId, operationMessage, StateSuccess)
	if err := om.publisher.Send(ctx, envelope); err != nil {
		log.Printf("failed sending success message: %s", err.Error())
	}

	return nil
}
