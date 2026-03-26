package allyourbase

import "fmt"

// Error is a normalized API error.
type Error struct {
	Status  int            `json:"status"`
	Message string         `json:"message"`
	Code    string         `json:"code,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	DocURL  string         `json:"doc_url,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("AYBError(status=%d, message=%s)", e.Status, e.Message)
}
