package operations

import (
	"context"
	"log"

	"github.com/google/uuid"
)

type messageOperation struct {
	correlationId uuid.UUID
	publisher     *operationPublisher
}

// NewOperation creates a new operation
func newMessageOperation(publisher *operationPublisher) *messageOperation {
	return &messageOperation{
		correlationId: uuid.New(),
		publisher:     publisher,
	}
}

// Succeed reports the operation as successful
func (o *messageOperation) Succeed(ctx context.Context, message string) {
	_, msg := NewCorrelatedMessage(o.correlationId, message, StateSuccess)
	if err := o.publisher.Send(ctx, msg); err != nil {
		log.Printf("failed sending success message: %s", err.Error())
	}
}

// Progress reports the operation as in progress
func (o *messageOperation) Progress(ctx context.Context, message string) {
	_, msg := NewCorrelatedMessage(o.correlationId, message, StateProgress)
	if err := o.publisher.Send(ctx, msg); err != nil {
		log.Printf("failed sending progress message: %s", err.Error())
	}
}

// Fail reports the operation as failed
func (o *messageOperation) Fail(ctx context.Context, message string) {
	_, msg := NewCorrelatedMessage(o.correlationId, message, StateError)
	if err := o.publisher.Send(ctx, msg); err != nil {
		log.Printf("failed sending error message: %s", err.Error())
	}
}

// Skip reports the operation as skipped
func (o *messageOperation) Skip(ctx context.Context) {
	_, msg := NewCorrelatedMessage(o.correlationId, "skipped", StateSkipped)
	if err := o.publisher.Send(ctx, msg); err != nil {
		log.Printf("failed sending skip message: %s", err.Error())
	}
}

// Warn reports the operation has a warning
func (o *messageOperation) Warn(ctx context.Context, message string) {
	_, msg := NewCorrelatedMessage(o.correlationId, message, StateWarning)
	if err := o.publisher.Send(ctx, msg); err != nil {
		log.Printf("failed sending warning message: %s", err.Error())
	}
}
