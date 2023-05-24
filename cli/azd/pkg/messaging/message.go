package messaging

import (
	"context"
	"time"
)

type MessageKind string

const (
	SimpleMessage MessageKind = "Simple"
)

type Message struct {
	Timestamp time.Time
	Type      MessageKind
	Value     any
	Tags      map[string]any
}

func NewSimpleMessage(message string) *Message {
	return NewMessage(SimpleMessage, message)
}

func NewMessage[T any](kind MessageKind, value T) *Message {
	return &Message{
		Type:      kind,
		Value:     value,
		Timestamp: time.Now(),
		Tags:      map[string]any{},
	}
}

type Publisher interface {
	Send(ctx context.Context, msg *Message)
}

type Subscriber interface {
	Subscribe(ctx context.Context, filter MessageFilter, handler MessageHandler) *Subscription
}