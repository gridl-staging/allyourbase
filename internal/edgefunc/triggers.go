package edgefunc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// CronTrigger links an edge function to a jobs.Schedule for periodic execution.
type CronTrigger struct {
	ID         string          `json:"id"`
	FunctionID string          `json:"functionId"`
	ScheduleID string          `json:"scheduleId"`
	CronExpr   string          `json:"cronExpr"`
	Timezone   string          `json:"timezone"`
	Payload    json.RawMessage `json:"payload"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

var (
	ErrCronTriggerNotFound             = errors.New("cron trigger not found")
	ErrInvalidCronExpr                 = errors.New("invalid cron expression")
	ErrInvalidTimezone                 = errors.New("invalid timezone")
	ErrFunctionIDRequired              = errors.New("function ID is required")
	ErrCronExprRequired                = errors.New("cron expression is required")
	ErrCronTriggerServiceNotConfigured = errors.New("cron trigger service is not configured")
)

// CronTriggerStore defines CRUD for cron trigger persistence.
type CronTriggerStore interface {
	CreateCronTrigger(ctx context.Context, trigger *CronTrigger) (*CronTrigger, error)
	GetCronTrigger(ctx context.Context, id string) (*CronTrigger, error)
	ListCronTriggers(ctx context.Context, functionID string) ([]*CronTrigger, error)
	UpdateCronTrigger(ctx context.Context, trigger *CronTrigger) (*CronTrigger, error)
	DeleteCronTrigger(ctx context.Context, id string) error
}

// CronJobPayload is the payload stored in jobs.Schedule for edge function cron triggers.
// The job handler extracts the function ID and constructs an invocation request.
type CronJobPayload struct {
	FunctionID    string          `json:"function_id"`
	CronTriggerID string          `json:"cron_trigger_id"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// DBTriggerEvent represents a database operation type that can fire a trigger.
type DBTriggerEvent string

const (
	DBEventInsert DBTriggerEvent = "INSERT"
	DBEventUpdate DBTriggerEvent = "UPDATE"
	DBEventDelete DBTriggerEvent = "DELETE"
)

// ValidDBTriggerEvents are the allowed event types for DB triggers.
var ValidDBTriggerEvents = map[DBTriggerEvent]bool{
	DBEventInsert: true,
	DBEventUpdate: true,
	DBEventDelete: true,
}

// DBTrigger links an edge function to a database table for event-driven invocation.
type DBTrigger struct {
	ID            string           `json:"id"`
	FunctionID    string           `json:"functionId"`
	TableName     string           `json:"tableName"`
	Schema        string           `json:"schema"`
	Events        []DBTriggerEvent `json:"events"`
	FilterColumns []string         `json:"filterColumns,omitempty"`
	Enabled       bool             `json:"enabled"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

var (
	ErrDBTriggerNotFound  = errors.New("db trigger not found")
	ErrTableNameRequired  = errors.New("table name is required")
	ErrDBEventsRequired   = errors.New("at least one event type is required")
	ErrInvalidDBEvent     = errors.New("invalid db event type (must be INSERT, UPDATE, or DELETE)")
	ErrDBTriggerDuplicate = errors.New("db trigger already exists for this function and table")
	ErrInvalidIdentifier  = errors.New("invalid SQL identifier: must start with a letter or underscore, contain only letters/digits/underscores, and be at most 63 characters")
)

// DBTriggerStore defines CRUD for DB trigger persistence.
type DBTriggerStore interface {
	CreateDBTrigger(ctx context.Context, trigger *DBTrigger) (*DBTrigger, error)
	GetDBTrigger(ctx context.Context, id string) (*DBTrigger, error)
	ListDBTriggers(ctx context.Context, functionID string) ([]*DBTrigger, error)
	ListDBTriggersByTable(ctx context.Context, schema, tableName string) ([]*DBTrigger, error)
	UpdateDBTrigger(ctx context.Context, trigger *DBTrigger) (*DBTrigger, error)
	DeleteDBTrigger(ctx context.Context, id string) error
}

// MatchesDBEvent returns true if the trigger should fire for the given operation.
func (t *DBTrigger) MatchesDBEvent(operation string) bool {
	if !t.Enabled {
		return false
	}
	op := DBTriggerEvent(operation)
	for _, ev := range t.Events {
		if ev == op {
			return true
		}
	}
	return false
}

// DBTriggerEventStatus represents the processing status of a queued DB trigger event.
type DBTriggerEventStatus string

const (
	DBEventStatusPending    DBTriggerEventStatus = "pending"
	DBEventStatusProcessing DBTriggerEventStatus = "processing"
	DBEventStatusCompleted  DBTriggerEventStatus = "completed"
	DBEventStatusFailed     DBTriggerEventStatus = "failed"
)

// DBTriggerQueueEvent represents a queued database trigger event awaiting dispatch.
type DBTriggerQueueEvent struct {
	ID          string               `json:"id"`
	TriggerID   string               `json:"triggerId"`
	TableName   string               `json:"tableName"`
	SchemaName  string               `json:"schemaName"`
	Operation   string               `json:"operation"`
	RowID       string               `json:"rowId"`
	Payload     json.RawMessage      `json:"payload"`
	Status      DBTriggerEventStatus `json:"status"`
	Attempts    int                  `json:"attempts"`
	CreatedAt   time.Time            `json:"createdAt"`
	ProcessedAt *time.Time           `json:"processedAt,omitempty"`
}

// MaxDBTriggerRetries is the maximum number of dispatch attempts for a failed event.
const MaxDBTriggerRetries = 3

// DBTriggerEventStore defines operations on the event queue table.
type DBTriggerEventStore interface {
	// ClaimPendingEvents atomically claims up to limit pending events for processing.
	ClaimPendingEvents(ctx context.Context, limit int) ([]*DBTriggerQueueEvent, error)
	// MarkCompleted marks an event as successfully processed.
	MarkCompleted(ctx context.Context, eventID string) error
	// MarkFailed marks an event as failed, incrementing the attempt count.
	// Events exceeding MaxDBTriggerRetries are marked as permanently failed.
	MarkFailed(ctx context.Context, eventID string) error
}

// StorageTrigger links an edge function to a storage bucket for event-driven invocation.
type StorageTrigger struct {
	ID           string    `json:"id"`
	FunctionID   string    `json:"functionId"`
	Bucket       string    `json:"bucket"`
	EventTypes   []string  `json:"eventTypes"`
	PrefixFilter string    `json:"prefixFilter,omitempty"`
	SuffixFilter string    `json:"suffixFilter,omitempty"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

var (
	ErrStorageTriggerNotFound = errors.New("storage trigger not found")
	ErrBucketRequired         = errors.New("bucket is required")
	ErrEventTypesRequired     = errors.New("at least one event type is required")
	ErrInvalidEventType       = errors.New("invalid event type (must be upload or delete)")
)

// ValidStorageEventTypes are the allowed event types for storage triggers.
var ValidStorageEventTypes = map[string]bool{
	"upload": true,
	"delete": true,
}

// StorageTriggerStore defines CRUD for storage trigger persistence.
type StorageTriggerStore interface {
	CreateStorageTrigger(ctx context.Context, trigger *StorageTrigger) (*StorageTrigger, error)
	GetStorageTrigger(ctx context.Context, id string) (*StorageTrigger, error)
	ListStorageTriggers(ctx context.Context, functionID string) ([]*StorageTrigger, error)
	ListStorageTriggersByBucket(ctx context.Context, bucket string) ([]*StorageTrigger, error)
	UpdateStorageTrigger(ctx context.Context, trigger *StorageTrigger) (*StorageTrigger, error)
	DeleteStorageTrigger(ctx context.Context, id string) error
}

// MatchesStorageEvent returns true if the trigger should fire for the given event.
func (t *StorageTrigger) MatchesStorageEvent(bucket, name, operation string) bool {
	if !t.Enabled {
		return false
	}
	if t.Bucket != bucket {
		return false
	}

	// Check event type
	matched := false
	for _, et := range t.EventTypes {
		if et == operation {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}

	// Check prefix filter
	if t.PrefixFilter != "" && !strings.HasPrefix(name, t.PrefixFilter) {
		return false
	}

	// Check suffix filter
	if t.SuffixFilter != "" && !strings.HasSuffix(name, t.SuffixFilter) {
		return false
	}

	return true
}
