package edgefunc_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// --- Trigger SQL generation tests ---

func TestInstallTriggerSQL_SingleEvent(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "abc-123",
		TableName: "users",
		Schema:    "public",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	testutil.True(t, strings.Contains(sql, "CREATE TRIGGER"), "should contain CREATE TRIGGER")
	testutil.True(t, strings.Contains(sql, `"_ayb_edge_trig_abc-123"`), "should quote trigger name (hyphens require quoting)")
	testutil.True(t, strings.Contains(sql, "AFTER INSERT"), "should specify INSERT event")
	testutil.True(t, strings.Contains(sql, `"public"."users"`), "should quote schema and table")
	testutil.True(t, strings.Contains(sql, "_ayb_edge_notify('abc-123')"), "should pass trigger ID to function")
	testutil.True(t, strings.Contains(sql, "FOR EACH ROW"), "should fire for each row")
}

func TestInstallTriggerSQL_MultipleEvents(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "xyz-789",
		TableName: "orders",
		Schema:    "app",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert, edgefunc.DBEventUpdate, edgefunc.DBEventDelete},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	testutil.True(t, strings.Contains(sql, "INSERT OR UPDATE OR DELETE"), "should combine events with OR")
	testutil.True(t, strings.Contains(sql, `"app"."orders"`), "should use custom schema")
}

func TestRemoveTriggerSQL(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "abc-123",
		TableName: "users",
		Schema:    "public",
	}

	sql := edgefunc.RemoveTriggerSQL(trigger)
	testutil.True(t, strings.Contains(sql, "DROP TRIGGER IF EXISTS"), "should use IF EXISTS")
	testutil.True(t, strings.Contains(sql, `"_ayb_edge_trig_abc-123"`), "should quote trigger name")
	testutil.True(t, strings.Contains(sql, `"public"."users"`), "should specify schema.table")
}

func TestEnableTriggerSQL(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "abc-123",
		TableName: "users",
		Schema:    "public",
	}

	sql := edgefunc.EnableTriggerSQL(trigger)
	testutil.True(t, strings.Contains(sql, "ENABLE TRIGGER"), "should ENABLE")
	testutil.True(t, strings.Contains(sql, `"_ayb_edge_trig_abc-123"`), "should quote trigger name")
}

func TestDisableTriggerSQL(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "abc-123",
		TableName: "users",
		Schema:    "public",
	}

	sql := edgefunc.DisableTriggerSQL(trigger)
	testutil.True(t, strings.Contains(sql, "DISABLE TRIGGER"), "should DISABLE")
}

func TestTriggerName(t *testing.T) {
	name := edgefunc.TriggerName("abc-123")
	testutil.Equal(t, "_ayb_edge_trig_abc-123", name)
}

func TestInstallTriggerSQL_UUIDTriggerID(t *testing.T) {
	// Real-world trigger IDs are UUIDs which contain hyphens — these must be quoted.
	trigger := &edgefunc.DBTrigger{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		TableName: "users",
		Schema:    "public",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	testutil.True(t, strings.Contains(sql, `"_ayb_edge_trig_550e8400-e29b-41d4-a716-446655440000"`),
		"UUID-based trigger name must be quoted to be valid SQL, got: %s", sql)
}

func TestInstallTriggerSQL_QuotesSpecialChars(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "test-id",
		TableName: `weird"table`,
		Schema:    "public",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	// Double-quote escaping: " becomes ""
	testutil.True(t, strings.Contains(sql, `"weird""table"`), "should escape double quotes in table identifiers")
	testutil.True(t, strings.Contains(sql, `"_ayb_edge_trig_test-id"`), "should quote trigger name with hyphen")
}

func TestInstallTriggerSQL_StripsNullBytes(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "test-id",
		TableName: "users\x00evil",
		Schema:    "public",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	testutil.False(t, strings.Contains(sql, "\x00"), "should strip null bytes from identifiers")
	testutil.True(t, strings.Contains(sql, `"usersevil"`), "should have table name without null byte")
}

func TestInstallTriggerSQL_RejectsUnknownEvents(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "test-id",
		TableName: "users",
		Schema:    "public",
		Events:    []edgefunc.DBTriggerEvent{"INSERT", "TRUNCATE; DROP TABLE users; --"},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	// Should only contain INSERT, not the injected SQL
	testutil.True(t, strings.Contains(sql, "AFTER INSERT ON"), "should contain INSERT")
	testutil.False(t, strings.Contains(sql, "TRUNCATE"), "should not contain unknown event type")
	testutil.False(t, strings.Contains(sql, "DROP"), "should not contain injected SQL")
}

func TestInstallTriggerSQL_EscapesTriggerIDInStringLiteral(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		ID:        "id-with-'quote",
		TableName: "users",
		Schema:    "public",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	}

	sql := edgefunc.InstallTriggerSQL(trigger)
	// Single quotes should be doubled for SQL string literal safety
	testutil.True(t, strings.Contains(sql, "id-with-''quote"), "should escape single quotes in trigger ID, got: %s", sql)
}

// --- Worker dispatch loop tests (using mock event store) ---

func TestDBTriggerWorkerDispatch_SuccessfulEvent(t *testing.T) {
	triggerStore := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(triggerStore)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	var invoked bool
	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invoked = true
			testutil.Equal(t, funcID, functionID)

			// Verify the payload structure
			var payload map[string]interface{}
			testutil.NoError(t, json.Unmarshal(req.Body, &payload))
			testutil.Equal(t, "db_event", payload["type"])
			testutil.Equal(t, "users", payload["table"])
			testutil.Equal(t, "public", payload["schema"])
			testutil.Equal(t, "INSERT", payload["operation"])

			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(triggerStore, invoker)

	eventStore := &mockDBTriggerEventStore{
		events: []*edgefunc.DBTriggerQueueEvent{
			{
				ID:         uuid.New().String(),
				TriggerID:  created.ID,
				TableName:  "users",
				SchemaName: "public",
				Operation:  "INSERT",
				RowID:      "row-1",
				Payload:    json.RawMessage(`{"id":"row-1","name":"Alice"}`),
			},
		},
	}

	// Simulate what processAvailableEvents does
	events, err := eventStore.ClaimPendingEvents(context.Background(), 10)
	testutil.NoError(t, err)
	testutil.SliceLen(t, events, 1)

	err = dispatcher.DispatchEvent(context.Background(), events[0])
	testutil.NoError(t, err)
	testutil.True(t, invoked, "function should have been invoked")

	testutil.NoError(t, eventStore.MarkCompleted(context.Background(), events[0].ID))
	testutil.Equal(t, "completed", eventStore.statuses[events[0].ID])
}

func TestDBTriggerWorkerDispatch_FailedEvent(t *testing.T) {
	triggerStore := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(triggerStore)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			return edgefunc.Response{}, errors.New("runtime error")
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(triggerStore, invoker)

	eventStore := &mockDBTriggerEventStore{
		events: []*edgefunc.DBTriggerQueueEvent{
			{
				ID:         uuid.New().String(),
				TriggerID:  created.ID,
				TableName:  "users",
				SchemaName: "public",
				Operation:  "INSERT",
				Payload:    json.RawMessage(`{"id":"1"}`),
			},
		},
	}

	events, _ := eventStore.ClaimPendingEvents(context.Background(), 10)

	err := dispatcher.DispatchEvent(context.Background(), events[0])
	testutil.True(t, err != nil, "should return error for retry")

	testutil.NoError(t, eventStore.MarkFailed(context.Background(), events[0].ID))
	testutil.Equal(t, "failed", eventStore.statuses[events[0].ID])
}

// --- Mock DBTriggerEventStore ---

type mockDBTriggerEventStore struct {
	events   []*edgefunc.DBTriggerQueueEvent
	statuses map[string]string
}

func (m *mockDBTriggerEventStore) ClaimPendingEvents(_ context.Context, limit int) ([]*edgefunc.DBTriggerQueueEvent, error) {
	if m.statuses == nil {
		m.statuses = make(map[string]string)
	}
	var claimed []*edgefunc.DBTriggerQueueEvent
	for i, e := range m.events {
		if i >= limit {
			break
		}
		e.Status = edgefunc.DBEventStatusProcessing
		m.statuses[e.ID] = "processing"
		claimed = append(claimed, e)
	}
	m.events = nil // Consume events
	if claimed == nil {
		claimed = []*edgefunc.DBTriggerQueueEvent{}
	}
	return claimed, nil
}

func (m *mockDBTriggerEventStore) MarkCompleted(_ context.Context, eventID string) error {
	if m.statuses == nil {
		m.statuses = make(map[string]string)
	}
	m.statuses[eventID] = "completed"
	return nil
}

func (m *mockDBTriggerEventStore) MarkFailed(_ context.Context, eventID string) error {
	if m.statuses == nil {
		m.statuses = make(map[string]string)
	}
	m.statuses[eventID] = "failed"
	return nil
}
