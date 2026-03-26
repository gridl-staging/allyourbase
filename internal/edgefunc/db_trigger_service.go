package edgefunc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// sqlIdentifierRe validates SQL identifiers: start with letter/underscore,
// contain only letters/digits/underscores, max 63 chars (PostgreSQL NAMEDATALEN).
var sqlIdentifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

// ValidSQLIdentifier returns true if s is a safe PostgreSQL identifier.
func ValidSQLIdentifier(s string) bool {
	return sqlIdentifierRe.MatchString(s)
}

// CreateDBTriggerInput holds the parameters for creating a DB trigger.
type CreateDBTriggerInput struct {
	FunctionID    string
	TableName     string
	Schema        string
	Events        []DBTriggerEvent
	FilterColumns []string
}

// DBTriggerService manages the lifecycle of database event triggers.
type DBTriggerService struct {
	store DBTriggerStore
	ddl   dbTriggerDDLStore
}

// NewDBTriggerService creates a new DBTriggerService.
func NewDBTriggerService(store DBTriggerStore) *DBTriggerService {
	svc := &DBTriggerService{store: store}
	if ddlStore, ok := store.(dbTriggerDDLStore); ok {
		svc.ddl = ddlStore
	}
	return svc
}

// Create validates input and persists a new DB trigger.
func (s *DBTriggerService) Create(ctx context.Context, input CreateDBTriggerInput) (*DBTrigger, error) {
	if strings.TrimSpace(input.FunctionID) == "" {
		return nil, ErrFunctionIDRequired
	}
	if strings.TrimSpace(input.TableName) == "" {
		return nil, ErrTableNameRequired
	}
	if !ValidSQLIdentifier(input.TableName) {
		return nil, fmt.Errorf("%w: table %q", ErrInvalidIdentifier, input.TableName)
	}
	if len(input.Events) == 0 {
		return nil, ErrDBEventsRequired
	}
	for _, ev := range input.Events {
		if !ValidDBTriggerEvents[ev] {
			return nil, fmt.Errorf("%w: %s", ErrInvalidDBEvent, ev)
		}
	}
	for _, col := range input.FilterColumns {
		if !ValidSQLIdentifier(col) {
			return nil, fmt.Errorf("%w: column %q", ErrInvalidIdentifier, col)
		}
	}

	schema := input.Schema
	if schema == "" {
		schema = "public"
	}
	if !ValidSQLIdentifier(schema) {
		return nil, fmt.Errorf("%w: schema %q", ErrInvalidIdentifier, schema)
	}

	trigger := &DBTrigger{
		FunctionID:    input.FunctionID,
		TableName:     input.TableName,
		Schema:        schema,
		Events:        input.Events,
		FilterColumns: input.FilterColumns,
		Enabled:       true,
	}

	created, err := s.store.CreateDBTrigger(ctx, trigger)
	if err != nil {
		return nil, err
	}

	if s.ddl != nil {
		if err := s.ddl.InstallTrigger(ctx, created); err != nil {
			// Best-effort cleanup to keep metadata in sync if trigger install fails.
			if delErr := s.store.DeleteDBTrigger(ctx, created.ID); delErr != nil {
				slog.Error("db trigger: cleanup failed after install error",
					"trigger_id", created.ID, "install_error", err, "delete_error", delErr)
			}
			return nil, fmt.Errorf("installing db trigger: %w", err)
		}
	}

	return created, nil
}

// Get returns a DB trigger by ID.
func (s *DBTriggerService) Get(ctx context.Context, id string) (*DBTrigger, error) {
	return s.store.GetDBTrigger(ctx, id)
}

// List returns all DB triggers for a function.
func (s *DBTriggerService) List(ctx context.Context, functionID string) ([]*DBTrigger, error) {
	return s.store.ListDBTriggers(ctx, functionID)
}

// Delete removes a DB trigger.
func (s *DBTriggerService) Delete(ctx context.Context, id string) error {
	if s.ddl == nil {
		return s.store.DeleteDBTrigger(ctx, id)
	}

	trigger, err := s.store.GetDBTrigger(ctx, id)
	if err != nil {
		return err
	}
	if err := s.ddl.RemoveTrigger(ctx, trigger); err != nil {
		return fmt.Errorf("removing db trigger: %w", err)
	}
	return s.store.DeleteDBTrigger(ctx, id)
}

// SetEnabled toggles a DB trigger's enabled state.
func (s *DBTriggerService) SetEnabled(ctx context.Context, id string, enabled bool) (*DBTrigger, error) {
	trigger, err := s.store.GetDBTrigger(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.ddl != nil {
		if err := s.ddl.SetTriggerEnabled(ctx, trigger, enabled); err != nil {
			return nil, fmt.Errorf("toggling db trigger: %w", err)
		}
	}
	trigger.Enabled = enabled
	return s.store.UpdateDBTrigger(ctx, trigger)
}

type dbTriggerDDLStore interface {
	InstallTrigger(ctx context.Context, trigger *DBTrigger) error
	RemoveTrigger(ctx context.Context, trigger *DBTrigger) error
	SetTriggerEnabled(ctx context.Context, trigger *DBTrigger, enabled bool) error
}

// dbTriggerSourceKey is the context key for tracking which trigger is currently dispatching.
type dbTriggerSourceKey struct{}

// DBTriggerSource returns the trigger ID that is the source of a DB event dispatch, if any.
func DBTriggerSource(ctx context.Context) string {
	if v, ok := ctx.Value(dbTriggerSourceKey{}).(string); ok {
		return v
	}
	return ""
}

// WithDBTriggerSource returns a context marking the given trigger as the source of dispatch.
func WithDBTriggerSource(ctx context.Context, triggerID string) context.Context {
	return context.WithValue(ctx, dbTriggerSourceKey{}, triggerID)
}

// DBTriggerDispatcher resolves trigger events to functions and invokes them.
type DBTriggerDispatcher struct {
	store   DBTriggerStore
	invoker FunctionInvoker
	logger  *slog.Logger
}

// NewDBTriggerDispatcher creates a dispatcher for DB trigger events.
func NewDBTriggerDispatcher(store DBTriggerStore, invoker FunctionInvoker, opts ...func(*DBTriggerDispatcher)) *DBTriggerDispatcher {
	d := &DBTriggerDispatcher{
		store:   store,
		invoker: invoker,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithDBDispatcherLogger sets the logger for the dispatcher.
func WithDBDispatcherLogger(logger *slog.Logger) func(*DBTriggerDispatcher) {
	return func(d *DBTriggerDispatcher) {
		d.logger = logger
	}
}

// DispatchEvent resolves a queued event to its trigger, checks matching, and invokes
// the associated function. Returns an error if invocation fails (for retry purposes).
func (d *DBTriggerDispatcher) DispatchEvent(ctx context.Context, event *DBTriggerQueueEvent) error {
	// Recursion guard: block direct self-recursion (same trigger re-firing)
	if source := DBTriggerSource(ctx); source == event.TriggerID {
		d.logger.Info("skipping db trigger dispatch (direct recursion guard)",
			"trigger_id", event.TriggerID,
			"event_id", event.ID,
		)
		return nil
	}

	// Chain depth guard: prevent indirect cycles (A→B→A) across all trigger types
	if err := CheckTriggerChainDepth(ctx); err != nil {
		d.logger.Warn("skipping db trigger dispatch (chain depth exceeded)",
			"trigger_id", event.TriggerID,
			"event_id", event.ID,
			"depth", TriggerChainDepth(ctx),
		)
		return nil
	}

	trigger, err := d.store.GetDBTrigger(ctx, event.TriggerID)
	if err != nil {
		return fmt.Errorf("loading trigger %s: %w", event.TriggerID, err)
	}

	if !trigger.MatchesDBEvent(event.Operation) {
		return nil
	}

	// Build the invocation payload
	payload, err := json.Marshal(dbEventPayload{
		Type:      "db_event",
		Table:     event.TableName,
		Schema:    event.SchemaName,
		Operation: event.Operation,
		Record:    event.Payload,
		OldRecord: nil, // OLD record support is a future enhancement
	})
	if err != nil {
		return fmt.Errorf("marshaling db event payload: %w", err)
	}

	req := Request{
		Method: "POST",
		Path:   "/db-event",
		Body:   payload,
	}

	childCtx := WithDBTriggerSource(ctx, event.TriggerID)
	childCtx = WithTriggerMeta(childCtx, TriggerDB, event.TriggerID)
	childCtx = WithTriggerChainDepth(childCtx, TriggerChainDepth(ctx)+1)
	if _, err := d.invoker.InvokeByID(childCtx, trigger.FunctionID, req); err != nil {
		d.logger.Error("db trigger dispatch failed",
			"trigger_id", event.TriggerID,
			"function_id", trigger.FunctionID,
			"event_id", event.ID,
			"table", event.TableName,
			"operation", event.Operation,
			"error", err,
		)
		return fmt.Errorf("invoking function %s: %w", trigger.FunctionID, err)
	}

	d.logger.Info("db trigger dispatch success",
		"trigger_id", event.TriggerID,
		"function_id", trigger.FunctionID,
		"event_id", event.ID,
		"table", event.TableName,
		"operation", event.Operation,
	)
	return nil
}

// dbEventPayload is the JSON payload sent to edge functions on database events.
type dbEventPayload struct {
	Type      string          `json:"type"`
	Table     string          `json:"table"`
	Schema    string          `json:"schema"`
	Operation string          `json:"operation"`
	Record    json.RawMessage `json:"record,omitempty"`
	OldRecord json.RawMessage `json:"old_record,omitempty"`
}
