package sms

import (
	"context"
	"regexp"
	"sync"
)

// CaptureProvider records SMS sends for use in integration tests.
type CaptureProvider struct {
	mu    sync.Mutex
	Calls []CaptureCall
}

// CaptureCall records a single Send invocation.
type CaptureCall struct {
	To   string
	Body string
}

func (c *CaptureProvider) Send(_ context.Context, to, body string) (*SendResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = append(c.Calls, CaptureCall{To: to, Body: body})
	return &SendResult{Status: "captured"}, nil
}

// LastCode extracts a 4-8 digit OTP from the last captured SMS body.
func (c *CaptureProvider) LastCode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Calls) == 0 {
		return ""
	}
	re := regexp.MustCompile(`\b(\d{4,8})\b`)
	matches := re.FindStringSubmatch(c.Calls[len(c.Calls)-1].Body)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// Reset clears all recorded calls.
func (c *CaptureProvider) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = nil
}
