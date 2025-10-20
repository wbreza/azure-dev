// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package streaming

// StreamMessage defines the interface that all gRPC streaming messages must implement
// to be used with the StreamRequestManager.
type StreamMessage interface {
	// GetRequestId returns the unique identifier for this request/response
	GetRequestId() string
	// StreamingError returns any error information in the message
	StreamingError() ErrorMessage
	// Progress returns progress information if this is a progress update
	Progress() ProgressMessage
	// IsExpectedResponse returns true if the given message is the expected response for this request
	IsExpectedResponse(response StreamMessage) bool
}

// ErrorMessage defines the interface for error information in streaming messages
type ErrorMessage interface {
	// GetMessage returns the error message text
	GetMessage() string

	// GetDetails returns the details of the error
	GetDetails() string
}

// ProgressMessage defines the interface for progress information in streaming messages
type ProgressMessage interface {
	// GetMessage returns the progress message text
	GetMessage() string
	// GetRequestId returns the request ID this progress update is for
	GetRequestId() string
}

// BidiStream defines the interface for bidirectional gRPC streams
type BidiStream[TMessage StreamMessage] interface {
	// Send sends a message on the stream
	Send(TMessage) error
	// Recv receives a message from the stream
	Recv() (TMessage, error)
}

// ProgressReporter defines the interface for reporting progress updates
type ProgressReporter interface {
	SetProgress(any)
}
