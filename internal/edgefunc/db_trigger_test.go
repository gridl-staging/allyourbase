package edgefunc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// --- DBTrigger.MatchesDBEvent tests ---

func TestDBTriggerMatch_SingleEvent(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		Events:  []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
		Enabled: true,
	}

	testutil.True(t, trigger.MatchesDBEvent("INSERT"), "should match INSERT")
	testutil.False(t, trigger.MatchesDBEvent("UPDATE"), "should not match UPDATE")
	testutil.False(t, trigger.MatchesDBEvent("DELETE"), "should not match DELETE")
}

func TestDBTriggerMatch_MultipleEvents(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		Events:  []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert, edgefunc.DBEventUpdate},
		Enabled: true,
	}

	testutil.True(t, trigger.MatchesDBEvent("INSERT"), "should match INSERT")
	testutil.True(t, trigger.MatchesDBEvent("UPDATE"), "should match UPDATE")
	testutil.False(t, trigger.MatchesDBEvent("DELETE"), "should not match DELETE")
}

func TestDBTriggerMatch_AllEvents(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		Events:  []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert, edgefunc.DBEventUpdate, edgefunc.DBEventDelete},
		Enabled: true,
	}

	testutil.True(t, trigger.MatchesDBEvent("INSERT"), "should match INSERT")
	testutil.True(t, trigger.MatchesDBEvent("UPDATE"), "should match UPDATE")
	testutil.True(t, trigger.MatchesDBEvent("DELETE"), "should match DELETE")
}

func TestDBTriggerMatch_Disabled(t *testing.T) {
	trigger := &edgefunc.DBTrigger{
		Events:  []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
		Enabled: false,
	}

	testutil.False(t, trigger.MatchesDBEvent("INSERT"), "disabled trigger should not match")
}

// --- DBTriggerService tests ---

func TestDBTriggerService_Create(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	trigger, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID:    uuid.New().String(),
		TableName:     "users",
		Schema:        "public",
		Events:        []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert, edgefunc.DBEventUpdate},
		FilterColumns: []string{"email", "name"},
	})
	testutil.NoError(t, err)
	testutil.True(t, trigger.ID != "", "should have an ID")
	testutil.Equal(t, "users", trigger.TableName)
	testutil.Equal(t, "public", trigger.Schema)
	testutil.SliceLen(t, trigger.Events, 2)
	testutil.SliceLen(t, trigger.FilterColumns, 2)
	testutil.True(t, trigger.Enabled, "should be enabled by default")
}

func TestDBTriggerService_Create_DefaultSchema(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	trigger, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "public", trigger.Schema)
}

func TestDBTriggerService_Create_MissingFunctionID(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		TableName: "users",
		Events:    []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionIDRequired), "should require function ID, got: %v", err)
}

func TestDBTriggerService_Create_MissingTableName(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrTableNameRequired), "should require table name, got: %v", err)
}

func TestDBTriggerService_Create_MissingEvents(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrDBEventsRequired), "should require events, got: %v", err)
}

func TestDBTriggerService_Create_InvalidEvent(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{"TRUNCATE"},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidDBEvent), "should reject invalid event, got: %v", err)
}

func TestDBTriggerService_Create_InvalidTableName(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	// Table name with SQL injection attempt
	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users; DROP TABLE users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidIdentifier), "should reject SQL injection in table name, got: %v", err)
}

func TestDBTriggerService_Create_InvalidSchema(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Schema:     "public\x00; DROP TABLE evil",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidIdentifier), "should reject null byte in schema, got: %v", err)
}

func TestDBTriggerService_Create_InvalidFilterColumn(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID:    uuid.New().String(),
		TableName:     "users",
		Events:        []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
		FilterColumns: []string{"email", "name; DROP TABLE users"},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidIdentifier), "should reject SQL injection in filter column, got: %v", err)
}

func TestValidSQLIdentifier(t *testing.T) {
	cases := []struct {
		name  string
		input string
		valid bool
	}{
		{"simple", "users", true},
		{"underscore_prefix", "_internal_table", true},
		{"with_digits", "table_1", true},
		{"single_char", "a", true},
		{"max_length_63", "a" + string(make([]byte, 62)), false}, // 63 bytes of 'a' but byte 0 is bad
		{"starts_with_digit", "1table", false},
		{"contains_space", "my table", false},
		{"contains_semicolon", "users;drop", false},
		{"contains_hyphen", "my-table", false},
		{"contains_dot", "schema.table", false},
		{"empty", "", false},
		{"null_byte", "users\x00evil", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := edgefunc.ValidSQLIdentifier(tc.input)
			testutil.Equal(t, tc.valid, got)
		})
	}
}

func TestDBTriggerDispatcher_ChainDepthGuard(t *testing.T) {
	store := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	var invocations int
	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations++
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	// Simulate a context at max trigger chain depth
	ctx := edgefunc.WithTriggerChainDepth(context.Background(), edgefunc.MaxTriggerChainDepth)

	err := dispatcher.DispatchEvent(ctx, &edgefunc.DBTriggerQueueEvent{
		ID:        uuid.New().String(),
		TriggerID: created.ID,
		Operation: "INSERT",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, invocations)
}

func TestDBTriggerService_Get(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, got.ID)
}

func TestDBTriggerService_Get_NotFound(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Get(context.Background(), "nonexistent")
	testutil.True(t, errors.Is(err, edgefunc.ErrDBTriggerNotFound), "should return not found, got: %v", err)
}

func TestDBTriggerService_List(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID, TableName: "users", Events: []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID, TableName: "orders", Events: []edgefunc.DBTriggerEvent{edgefunc.DBEventDelete},
	})

	triggers, err := svc.List(context.Background(), funcID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, triggers, 2)
}

func TestDBTriggerService_Delete(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	testutil.NoError(t, err)

	_, err = svc.Get(context.Background(), created.ID)
	testutil.True(t, errors.Is(err, edgefunc.ErrDBTriggerNotFound), "should be deleted")
}

func TestDBTriggerService_SetEnabled(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	testutil.True(t, created.Enabled, "should start enabled")

	updated, err := svc.SetEnabled(context.Background(), created.ID, false)
	testutil.NoError(t, err)
	testutil.False(t, updated.Enabled, "should be disabled")

	updated, err = svc.SetEnabled(context.Background(), created.ID, true)
	testutil.NoError(t, err)
	testutil.True(t, updated.Enabled, "should be re-enabled")
}

func TestDBTriggerService_Create_InstallsDDLTrigger(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, store.installCalls)
}

func TestDBTriggerService_Delete_RemovesDDLTrigger(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, store.removeCalls)
}

func TestDBTriggerService_SetEnabled_UpdatesDDLTriggerState(t *testing.T) {
	store := newMockDBTriggerStore()
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	_, err = svc.SetEnabled(context.Background(), created.ID, false)
	testutil.NoError(t, err)
	_, err = svc.SetEnabled(context.Background(), created.ID, true)
	testutil.NoError(t, err)

	testutil.Equal(t, 2, len(store.setEnabledCalls))
	if len(store.setEnabledCalls) == 2 {
		testutil.False(t, store.setEnabledCalls[0], "first toggle should disable trigger")
		testutil.True(t, store.setEnabledCalls[1], "second toggle should enable trigger")
	}
}

func TestDBTriggerService_Delete_DDLRemoveError(t *testing.T) {
	store := newMockDBTriggerStore()
	store.removeErr = errors.New("ddl remove failed")
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	testutil.ErrorContains(t, err, "removing db trigger")
	// Trigger should NOT be deleted from store when DDL fails
	_, err = svc.Get(context.Background(), created.ID)
	testutil.NoError(t, err)
}

func TestDBTriggerService_SetEnabled_DDLError(t *testing.T) {
	store := newMockDBTriggerStore()
	store.setEnabledErr = errors.New("ddl toggle failed")
	svc := edgefunc.NewDBTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: uuid.New().String(),
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	_, err = svc.SetEnabled(context.Background(), created.ID, false)
	testutil.ErrorContains(t, err, "toggling db trigger")
	// Trigger should remain in original state
	got, err := svc.Get(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.True(t, got.Enabled, "should still be enabled after DDL error")
}

// --- DBTriggerDispatcher tests ---

func TestDBTriggerDispatcher_DispatchEvent(t *testing.T) {
	store := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	var invocations []dbDispatcherInvokeCall
	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, dbDispatcherInvokeCall{functionID: functionID, req: req})
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	err := dispatcher.DispatchEvent(context.Background(), &edgefunc.DBTriggerQueueEvent{
		ID:         uuid.New().String(),
		TriggerID:  created.ID,
		TableName:  "users",
		SchemaName: "public",
		Operation:  "INSERT",
		RowID:      "123",
		Payload:    []byte(`{"id":"123","name":"Alice"}`),
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(invocations))
	testutil.Equal(t, funcID, invocations[0].functionID)
	testutil.Equal(t, "POST", invocations[0].req.Method)
	testutil.Equal(t, "/db-event", invocations[0].req.Path)

	// Verify payload structure matches documented db_event shape
	var payload map[string]interface{}
	testutil.NoError(t, json.Unmarshal(invocations[0].req.Body, &payload))
	testutil.Equal(t, "db_event", payload["type"])
	testutil.Equal(t, "users", payload["table"])
	testutil.Equal(t, "public", payload["schema"])
	testutil.Equal(t, "INSERT", payload["operation"])
	record, ok := payload["record"].(map[string]interface{})
	testutil.True(t, ok, "record should be an object, got %T", payload["record"])
	testutil.Equal(t, "123", record["id"])
	testutil.Equal(t, "Alice", record["name"])
}

func TestDBTriggerDispatcher_TriggerNotFound(t *testing.T) {
	store := newMockDBTriggerStore()

	invoker := &mockDBDispatcherInvoker{}
	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	err := dispatcher.DispatchEvent(context.Background(), &edgefunc.DBTriggerQueueEvent{
		ID:        uuid.New().String(),
		TriggerID: uuid.New().String(), // nonexistent
		Operation: "INSERT",
	})
	testutil.True(t, err != nil, "should error when trigger not found")
}

func TestDBTriggerDispatcher_TriggerDisabled(t *testing.T) {
	store := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	triggerSvc.SetEnabled(context.Background(), created.ID, false)

	var invocations int
	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations++
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	err := dispatcher.DispatchEvent(context.Background(), &edgefunc.DBTriggerQueueEvent{
		ID:        uuid.New().String(),
		TriggerID: created.ID,
		Operation: "INSERT",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, invocations)
}

func TestDBTriggerDispatcher_EventDoesNotMatchTrigger(t *testing.T) {
	store := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	var invocations int
	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations++
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	// DELETE event shouldn't match INSERT-only trigger
	err := dispatcher.DispatchEvent(context.Background(), &edgefunc.DBTriggerQueueEvent{
		ID:        uuid.New().String(),
		TriggerID: created.ID,
		Operation: "DELETE",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, invocations)
}

func TestDBTriggerDispatcher_RecursionGuard(t *testing.T) {
	store := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	var invocations int
	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations++
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	// Mark context as being inside a db trigger dispatch for the same trigger
	ctx := edgefunc.WithDBTriggerSource(context.Background(), created.ID)

	err := dispatcher.DispatchEvent(ctx, &edgefunc.DBTriggerQueueEvent{
		ID:        uuid.New().String(),
		TriggerID: created.ID,
		Operation: "INSERT",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, invocations)
}

func TestDBTriggerDispatcher_InvokerError(t *testing.T) {
	store := newMockDBTriggerStore()
	triggerSvc := edgefunc.NewDBTriggerService(store)

	funcID := uuid.New().String()
	created, _ := triggerSvc.Create(context.Background(), edgefunc.CreateDBTriggerInput{
		FunctionID: funcID,
		TableName:  "users",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})

	invoker := &mockDBDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			return edgefunc.Response{}, errors.New("function execution failed")
		},
	}

	dispatcher := edgefunc.NewDBTriggerDispatcher(store, invoker)

	err := dispatcher.DispatchEvent(context.Background(), &edgefunc.DBTriggerQueueEvent{
		ID:        uuid.New().String(),
		TriggerID: created.ID,
		Operation: "INSERT",
		Payload:   []byte(`{"id":"123"}`),
	})
	// Invocation errors should be returned so the event can be retried
	testutil.True(t, err != nil, "should return invocation error for retry")
}

// --- Mock DBTriggerStore ---

type mockDBTriggerStore struct {
	triggers        map[string]*edgefunc.DBTrigger
	byFunc          map[string][]string
	byTable         map[string][]string // key: "schema.table"
	installCalls    int
	removeCalls     int
	setEnabledCalls []bool
	installErr      error
	removeErr       error
	setEnabledErr   error
}

func newMockDBTriggerStore() *mockDBTriggerStore {
	return &mockDBTriggerStore{
		triggers: make(map[string]*edgefunc.DBTrigger),
		byFunc:   make(map[string][]string),
		byTable:  make(map[string][]string),
	}
}

func (m *mockDBTriggerStore) CreateDBTrigger(_ context.Context, t *edgefunc.DBTrigger) (*edgefunc.DBTrigger, error) {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	m.triggers[t.ID] = t
	m.byFunc[t.FunctionID] = append(m.byFunc[t.FunctionID], t.ID)
	key := t.Schema + "." + t.TableName
	m.byTable[key] = append(m.byTable[key], t.ID)
	return t, nil
}

func (m *mockDBTriggerStore) GetDBTrigger(_ context.Context, id string) (*edgefunc.DBTrigger, error) {
	t, ok := m.triggers[id]
	if !ok {
		return nil, edgefunc.ErrDBTriggerNotFound
	}
	return t, nil
}

func (m *mockDBTriggerStore) ListDBTriggers(_ context.Context, functionID string) ([]*edgefunc.DBTrigger, error) {
	var result []*edgefunc.DBTrigger
	for _, id := range m.byFunc[functionID] {
		if t, ok := m.triggers[id]; ok {
			result = append(result, t)
		}
	}
	if result == nil {
		result = []*edgefunc.DBTrigger{}
	}
	return result, nil
}

func (m *mockDBTriggerStore) ListDBTriggersByTable(_ context.Context, schema, tableName string) ([]*edgefunc.DBTrigger, error) {
	key := schema + "." + tableName
	var result []*edgefunc.DBTrigger
	for _, id := range m.byTable[key] {
		if t, ok := m.triggers[id]; ok {
			result = append(result, t)
		}
	}
	if result == nil {
		result = []*edgefunc.DBTrigger{}
	}
	return result, nil
}

func (m *mockDBTriggerStore) UpdateDBTrigger(_ context.Context, t *edgefunc.DBTrigger) (*edgefunc.DBTrigger, error) {
	if _, ok := m.triggers[t.ID]; !ok {
		return nil, edgefunc.ErrDBTriggerNotFound
	}
	t.UpdatedAt = time.Now()
	m.triggers[t.ID] = t
	return t, nil
}

func (m *mockDBTriggerStore) DeleteDBTrigger(_ context.Context, id string) error {
	t, ok := m.triggers[id]
	if !ok {
		return edgefunc.ErrDBTriggerNotFound
	}
	delete(m.triggers, id)
	for i, tid := range m.byFunc[t.FunctionID] {
		if tid == id {
			m.byFunc[t.FunctionID] = append(m.byFunc[t.FunctionID][:i], m.byFunc[t.FunctionID][i+1:]...)
			break
		}
	}
	key := t.Schema + "." + t.TableName
	for i, tid := range m.byTable[key] {
		if tid == id {
			m.byTable[key] = append(m.byTable[key][:i], m.byTable[key][i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockDBTriggerStore) InstallTrigger(_ context.Context, _ *edgefunc.DBTrigger) error {
	m.installCalls++
	return m.installErr
}

func (m *mockDBTriggerStore) RemoveTrigger(_ context.Context, _ *edgefunc.DBTrigger) error {
	m.removeCalls++
	return m.removeErr
}

func (m *mockDBTriggerStore) SetTriggerEnabled(_ context.Context, _ *edgefunc.DBTrigger, enabled bool) error {
	m.setEnabledCalls = append(m.setEnabledCalls, enabled)
	return m.setEnabledErr
}

// --- Mock Invoker for DB Dispatcher ---

type dbDispatcherInvokeCall struct {
	functionID string
	req        edgefunc.Request
}

type mockDBDispatcherInvoker struct {
	fn func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error)
}

func (m *mockDBDispatcherInvoker) InvokeByID(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
	if m.fn != nil {
		return m.fn(ctx, functionID, req)
	}
	return edgefunc.Response{StatusCode: 200}, nil
}
