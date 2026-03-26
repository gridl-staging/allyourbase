// Package push Defines CaptureProvider, a thread-safe test helper for capturing push send operations.
package push

import (
	"context"
	"fmt"
	"sync"
)

// CaptureProvider records sends for tests.
type CaptureProvider struct {
	mu    sync.Mutex
	Calls []CaptureCall
}

// CaptureCall stores one provider Send call.
type CaptureCall struct {
	Token   string
	Message Message
}

// Send records a push send operation by storing the token and message copy in a thread-safe manner for test inspection.
func (c *CaptureProvider) Send(_ context.Context, token string, msg *Message) (*Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	callMsg := Message{}
	if msg != nil {
		callMsg = *msg
		if msg.Data != nil {
			callMsg.Data = make(map[string]string, len(msg.Data))
			for k, v := range msg.Data {
				callMsg.Data[k] = v
			}
		}
	}

	c.Calls = append(c.Calls, CaptureCall{
		Token:   token,
		Message: callMsg,
	})
	return &Result{MessageID: fmt.Sprintf("captured-%d", len(c.Calls))}, nil
}

// Reset clears recorded calls.
func (c *CaptureProvider) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = nil
}
