package matview

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestMatviewRefreshHandlerValidPayload(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r1",
			SchemaName:  "public",
			ViewName:    "leaderboard",
			RefreshMode: RefreshModeStandard,
		},
		exists:    true,
		populated: true,
	}

	svc := NewService(store)
	handler := MatviewRefreshHandler(svc, store)

	payload := json.RawMessage(`{"schema":"public","view_name":"leaderboard"}`)
	err := handler(context.Background(), payload)
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW "public"."leaderboard"`, store.lastRefreshSQL)
}

func TestMatviewRefreshHandlerMissingViewName(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRefresherStore{})
	handler := MatviewRefreshHandler(svc, &fakeRefresherStore{})

	payload := json.RawMessage(`{"schema":"public"}`)
	err := handler(context.Background(), payload)
	testutil.True(t, err != nil, "should fail with missing view_name")
	testutil.ErrorContains(t, err, "view_name")
}

func TestMatviewRefreshHandlerDefaultSchema(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r2",
			SchemaName:  "public",
			ViewName:    "stats",
			RefreshMode: RefreshModeStandard,
		},
		exists:    true,
		populated: true,
	}

	svc := NewService(store)
	handler := MatviewRefreshHandler(svc, store)

	// Schema omitted — should default to "public"
	payload := json.RawMessage(`{"view_name":"stats"}`)
	err := handler(context.Background(), payload)
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW "public"."stats"`, store.lastRefreshSQL)
}

func TestMatviewRefreshHandlerInvalidJSON(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRefresherStore{})
	handler := MatviewRefreshHandler(svc, &fakeRefresherStore{})

	payload := json.RawMessage(`not json`)
	err := handler(context.Background(), payload)
	testutil.True(t, err != nil, "should fail with invalid JSON")
}

func TestMatviewRefreshHandlerAutoRegistersOnNotFound(t *testing.T) {
	t.Parallel()

	// Use a dedicated lookup fake that returns ErrRegistrationNotFound from GetByName,
	// then returns a registration from Register. The service store has an entry
	// matching the auto-registered ID so Get() works.
	reg := &Registration{
		ID:          "auto-public.new_mv",
		SchemaName:  "public",
		ViewName:    "new_mv",
		RefreshMode: RefreshModeStandard,
	}
	store := &fakeRefresherStore{
		entry:     reg,
		exists:    true,
		populated: true,
	}
	// Override GetByName to NOT match so the handler auto-registers.
	lookup := &fakeAutoRegisterLookup{registered: reg}

	svc := NewService(store)
	handler := MatviewRefreshHandler(svc, lookup)

	payload := json.RawMessage(`{"schema":"public","view_name":"new_mv"}`)
	err := handler(context.Background(), payload)
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW "public"."new_mv"`, store.lastRefreshSQL)
}

func TestMatviewRefreshHandlerDuplicateAutoRegisterFallsBackToLookup(t *testing.T) {
	t.Parallel()

	reg := &Registration{
		ID:          "auto-public.race_mv",
		SchemaName:  "public",
		ViewName:    "race_mv",
		RefreshMode: RefreshModeStandard,
	}
	store := &fakeRefresherStore{
		entry:     reg,
		exists:    true,
		populated: true,
	}
	lookup := &fakeDuplicateThenLookup{
		registered: reg,
	}

	svc := NewService(store)
	handler := MatviewRefreshHandler(svc, lookup)

	payload := json.RawMessage(`{"schema":"public","view_name":"race_mv"}`)
	err := handler(context.Background(), payload)
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW "public"."race_mv"`, store.lastRefreshSQL)
	testutil.Equal(t, 2, lookup.getCalls)
	testutil.Equal(t, 1, lookup.registerCalls)
}

// fakeAutoRegisterLookup always returns ErrRegistrationNotFound from GetByName,
// simulating the "first scheduled refresh" auto-registration path.
type fakeAutoRegisterLookup struct {
	registered *Registration
}

func (f *fakeAutoRegisterLookup) GetByName(ctx context.Context, schemaName, viewName string) (*Registration, error) {
	return nil, ErrRegistrationNotFound
}

func (f *fakeAutoRegisterLookup) Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error) {
	return f.registered, nil
}

// fakeDuplicateThenLookup simulates concurrent registration:
// first lookup misses, register returns duplicate, second lookup finds row.
type fakeDuplicateThenLookup struct {
	registered    *Registration
	getCalls      int
	registerCalls int
}

func (f *fakeDuplicateThenLookup) GetByName(ctx context.Context, schemaName, viewName string) (*Registration, error) {
	f.getCalls++
	if f.getCalls == 1 {
		return nil, ErrRegistrationNotFound
	}
	return f.registered, nil
}

func (f *fakeDuplicateThenLookup) Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error) {
	f.registerCalls++
	return nil, ErrDuplicateRegistration
}

func TestMatviewRefreshHandlerPropagatesDBError(t *testing.T) {
	t.Parallel()

	// Simulate a transient DB error from GetByName (not ErrRegistrationNotFound).
	// The handler should propagate it, NOT attempt auto-registration.
	dbErr := errors.New("connection refused")
	store := &fakeRefresherStore{
		getErr: dbErr,
	}

	svc := NewService(store)

	// Create a lookup that returns the DB error from GetByName
	lookup := &fakeDBErrorLookup{dbErr: dbErr}
	handler := MatviewRefreshHandler(svc, lookup)

	payload := json.RawMessage(`{"schema":"public","view_name":"test"}`)
	err := handler(context.Background(), payload)
	testutil.True(t, err != nil, "should propagate DB error")
	testutil.ErrorContains(t, err, "connection refused")
}

// fakeDBErrorLookup simulates a DB error from GetByName (not ErrRegistrationNotFound).
type fakeDBErrorLookup struct {
	dbErr error
}

func (f *fakeDBErrorLookup) GetByName(ctx context.Context, schemaName, viewName string) (*Registration, error) {
	return nil, f.dbErr
}

func (f *fakeDBErrorLookup) Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error) {
	return nil, errors.New("Register should not be called when GetByName returns a non-NotFound error")
}
