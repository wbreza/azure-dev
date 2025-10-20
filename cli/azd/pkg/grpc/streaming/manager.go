// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package streaming

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// StreamRequestManager handles gRPC bidirectional streaming with request/response correlation
// and optional progress tracking. It provides a generic solution for managing streaming
// communication patterns used across multiple azd services.
type StreamRequestManager[TMessage StreamMessage] struct {
	stream        BidiStream[TMessage]
	responseChans sync.Map // map[string]chan TMessage
	stopChan      chan struct{}
	stopped       bool
	mu            sync.RWMutex
}

// NewStreamRequestManager creates a new instance of the stream request manager
func NewStreamRequestManager[TMessage StreamMessage](stream BidiStream[TMessage]) *StreamRequestManager[TMessage] {
	manager := &StreamRequestManager[TMessage]{
		stream:   stream,
		stopChan: make(chan struct{}),
	}

	manager.startResponseDispatcher()
	return manager
}

// Send sends a request and waits for a matching response, automatically determining
// the expected response type using the StreamMessage.IsExpectedResponse method
func (srm *StreamRequestManager[TMessage]) Send(
	ctx context.Context,
	req TMessage,
) (TMessage, error) {
	var zero TMessage

	srm.mu.RLock()
	if srm.stopped {
		srm.mu.RUnlock()
		return zero, fmt.Errorf("stream manager is stopped")
	}
	srm.mu.RUnlock()

	// Create a response channel for this request
	respChan := make(chan TMessage, 1)
	srm.responseChans.Store(req.GetRequestId(), respChan)
	defer srm.responseChans.Delete(req.GetRequestId())

	// Send the request
	if err := srm.stream.Send(req); err != nil {
		return zero, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response via the async dispatcher
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case resp, ok := <-respChan:
		if !ok {
			return zero, fmt.Errorf("response channel closed")
		}

		if errMsg := resp.StreamingError(); errMsg != nil {
			return zero, fmt.Errorf("service error: %s", errMsg.GetMessage())
		}

		if !req.IsExpectedResponse(resp) {
			return zero, fmt.Errorf("received unexpected response type")
		}

		return resp, nil
	}
}

// SendWithProgress sends a request, handles progress updates, and waits for a matching response,
// automatically determining the expected response type using the StreamMessage.IsExpectedResponse method
func (srm *StreamRequestManager[TMessage]) SendWithProgress(
	ctx context.Context,
	req TMessage,
	progress ProgressReporter,
) (TMessage, error) {
	var zero TMessage

	srm.mu.RLock()
	if srm.stopped {
		srm.mu.RUnlock()
		return zero, fmt.Errorf("stream manager is stopped")
	}
	srm.mu.RUnlock()

	respChan := make(chan TMessage, 1)
	srm.responseChans.Store(req.GetRequestId(), respChan)
	defer srm.responseChans.Delete(req.GetRequestId())

	if err := srm.stream.Send(req); err != nil {
		return zero, fmt.Errorf("failed to send request: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case resp, ok := <-respChan:
			if !ok {
				return zero, fmt.Errorf("response channel closed")
			}

			if errMsg := resp.StreamingError(); errMsg != nil {
				return zero, fmt.Errorf("service error: %s", errMsg.GetMessage())
			}

			if progressMsg := resp.Progress(); progressMsg != nil {
				if progress != nil {
					// Let the progress reporter decide how to handle the progress message
					progress.SetProgress(progressMsg.GetMessage())
				}
				continue // Wait for the actual response
			}

			if !req.IsExpectedResponse(resp) {
				return zero, fmt.Errorf("received unexpected response type")
			}

			return resp, nil
		}
	}
}

// Stop stops the response dispatcher and cleans up resources
func (srm *StreamRequestManager[TMessage]) Stop() {
	srm.mu.Lock()
	defer srm.mu.Unlock()

	if srm.stopped {
		return
	}

	srm.stopped = true
	close(srm.stopChan)

	// Close all pending response channels
	srm.responseChans.Range(func(key, value any) bool {
		ch := value.(chan TMessage)
		close(ch)
		return true
	})
}

// startResponseDispatcher starts a goroutine to receive and dispatch responses
func (srm *StreamRequestManager[TMessage]) startResponseDispatcher() {
	go func() {
		for {
			select {
			case <-srm.stopChan:
				return
			default:
				resp, err := srm.stream.Recv()
				if err != nil {
					// propagate error to all waiting calls by closing channels
					srm.responseChans.Range(func(key, value any) bool {
						ch := value.(chan TMessage)
						close(ch)
						return true
					})
					return
				}

				if ch, ok := srm.responseChans.Load(resp.GetRequestId()); ok {
					select {
					case ch.(chan TMessage) <- resp:
					case <-srm.stopChan:
						return
					}
				} else {
					log.Printf("No response channel found for RequestId: %s", resp.GetRequestId())
				}
			}
		}
	}()
}
