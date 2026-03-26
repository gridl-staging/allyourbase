// Package edgefunc defines types for logging edge function invocations and provides interfaces for persisting and retrieving execution logs.
package edgefunc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// TriggerType identifies the source that triggered an edge function invocation.
type TriggerType string

const (
	TriggerHTTP     TriggerType = "http"
	TriggerDB       TriggerType = "db"
	TriggerCron     TriggerType = "cron"
	TriggerStorage  TriggerType = "storage"
	TriggerFunction TriggerType = "function"
)

// LogEntry represents a single edge function invocation log.
// LogEntry records execution details for a single edge function invocation, including status, duration, output, errors, HTTP request context, and trigger metadata.
type LogEntry struct {
	ID                 uuid.UUID `json:"id"`
	FunctionID         uuid.UUID `json:"functionId"`
	InvocationID       uuid.UUID `json:"invocationId"`
	Status             string    `json:"status"` // "success" or "error"
	DurationMs         int       `json:"durationMs"`
	Stdout             string    `json:"stdout,omitempty"`
	StdoutBytes        int       `json:"stdoutBytes"`
	Error              string    `json:"error,omitempty"`
	ResponseStatusCode int       `json:"responseStatusCode"`
	RequestMethod      string    `json:"requestMethod,omitempty"`
	RequestPath        string    `json:"requestPath,omitempty"`
	TriggerType        string    `json:"triggerType,omitempty"`
	TriggerID          string    `json:"triggerId,omitempty"`
	ParentInvocationID string    `json:"parentInvocationId,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
}

var ErrInvalidLogFilter = errors.New("invalid log filter")

// LogListOptions defines supported filters and pagination when listing logs.
type LogListOptions struct {
	Page        int
	PerPage     int
	Status      string
	TriggerType string
	Since       *time.Time
	Until       *time.Time
}

// LogStore persists and retrieves edge function execution logs.
type LogStore interface {
	WriteLog(ctx context.Context, entry *LogEntry) error
	ListByFunction(ctx context.Context, functionID uuid.UUID, opts LogListOptions) ([]*LogEntry, error)
}

// triggerMetaKey is the context key for propagating trigger source metadata to the service's log writer.
type triggerMetaKey struct{}

// TriggerMeta holds trigger source metadata that dispatchers attach to context for logging.
type TriggerMeta struct {
	Type TriggerType
	ID   string
}

// WithTriggerMeta returns a context annotated with trigger source metadata.
// Dispatchers (DB, cron, storage) should call this before invoking a function
// so that the service can record the trigger source in the log entry.
func WithTriggerMeta(ctx context.Context, triggerType TriggerType, triggerID string) context.Context {
	return context.WithValue(ctx, triggerMetaKey{}, TriggerMeta{Type: triggerType, ID: triggerID})
}

// GetTriggerMeta extracts trigger source metadata from the context, if present.
func GetTriggerMeta(ctx context.Context) (TriggerMeta, bool) {
	m, ok := ctx.Value(triggerMetaKey{}).(TriggerMeta)
	return m, ok
}
