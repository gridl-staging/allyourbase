package matview

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeRefresherStore struct {
	entry              *Registration
	getErr             error
	exists             bool
	populated          bool
	existsErr          error
	uniqueIndex        bool
	uniqueIndexErr     error
	lockedRefreshErr   error
	lastRefreshSQL     string
	lastLockKey        string
	updateStatusCall   int
	lastStatus         RefreshStatus
	lastErrorText      *string
	registerErr        error
}

func (f *fakeRefresherStore) Get(ctx context.Context, id string) (*Registration, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.entry, nil
}

func (f *fakeRefresherStore) MatviewState(ctx context.Context, schemaName, viewName string) (bool, bool, error) {
	if f.existsErr != nil {
		return false, false, f.existsErr
	}
	return f.exists, f.populated, nil
}

func (f *fakeRefresherStore) HasConcurrentUniqueIndex(ctx context.Context, schemaName, viewName string) (bool, error) {
	if f.uniqueIndexErr != nil {
		return false, f.uniqueIndexErr
	}
	return f.uniqueIndex, nil
}

func (f *fakeRefresherStore) LockedRefresh(ctx context.Context, lockKey, sql string) error {
	f.lastLockKey = lockKey
	f.lastRefreshSQL = sql
	return f.lockedRefreshErr
}

func (f *fakeRefresherStore) UpdateRefreshStatus(ctx context.Context, id string, status RefreshStatus, durationMs int, errText *string) error {
	f.updateStatusCall++
	f.lastStatus = status
	f.lastErrorText = errText
	return nil
}

func (f *fakeRefresherStore) GetByName(ctx context.Context, schemaName, viewName string) (*Registration, error) {
	if f.entry != nil && f.entry.SchemaName == schemaName && f.entry.ViewName == viewName {
		return f.entry, nil
	}
	return nil, ErrRegistrationNotFound
}

func (f *fakeRefresherStore) Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error) {
	if f.registerErr != nil {
		return nil, f.registerErr
	}
	if f.entry != nil {
		return f.entry, nil
	}
	return &Registration{
		ID:          "auto-" + schemaName + "." + viewName,
		SchemaName:  schemaName,
		ViewName:    viewName,
		RefreshMode: mode,
	}, nil
}

func TestRefreshNowReturnsInProgressWhenLockNotAcquired(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r1",
			SchemaName:  "public",
			ViewName:    "leaderboard",
			RefreshMode: RefreshModeStandard,
		},
		exists:           true,
		populated:        true,
		lockedRefreshErr: ErrRefreshInProgress,
	}

	svc := NewService(store)
	_, err := svc.RefreshNow(context.Background(), "r1")
	testutil.True(t, errors.Is(err, ErrRefreshInProgress), "expected ErrRefreshInProgress")
	testutil.Equal(t, 1, store.updateStatusCall)
	testutil.Equal(t, RefreshStatusError, store.lastStatus)
}

func TestRefreshNowRecordsSuccessOnLockedRefresh(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r1",
			SchemaName:  "public",
			ViewName:    "leaderboard",
			RefreshMode: RefreshModeConcurrent,
		},
		exists:      true,
		populated:   true,
		uniqueIndex: true,
	}

	svc := NewService(store)
	_, err := svc.RefreshNow(context.Background(), "r1")
	testutil.NoError(t, err)
	testutil.Equal(t, `REFRESH MATERIALIZED VIEW CONCURRENTLY "public"."leaderboard"`, store.lastRefreshSQL)
	testutil.Equal(t, "public.leaderboard", store.lastLockKey)
	testutil.Equal(t, 1, store.updateStatusCall)
	testutil.Equal(t, RefreshStatusSuccess, store.lastStatus)
}

func TestRefreshNowRecordsErrorOnRefreshFailure(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r1",
			SchemaName:  "public",
			ViewName:    "leaderboard",
			RefreshMode: RefreshModeStandard,
		},
		exists:           true,
		populated:        true,
		lockedRefreshErr: errors.New("refresh failed"),
	}

	svc := NewService(store)
	_, err := svc.RefreshNow(context.Background(), "r1")
	testutil.True(t, err != nil, "refresh should fail")
	testutil.Equal(t, 1, store.updateStatusCall)
	testutil.Equal(t, RefreshStatusError, store.lastStatus)
	testutil.True(t, store.lastErrorText != nil, "error text should be recorded")
}

func TestRefreshNowRejectsMissingConcurrentIndex(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r1",
			SchemaName:  "public",
			ViewName:    "stats",
			RefreshMode: RefreshModeConcurrent,
		},
		exists:      true,
		populated:   true,
		uniqueIndex: false,
	}

	svc := NewService(store)
	_, err := svc.RefreshNow(context.Background(), "r1")
	testutil.True(t, errors.Is(err, ErrConcurrentRefreshRequiresIndex), "expected ErrConcurrentRefreshRequiresIndex")
	testutil.Equal(t, 1, store.updateStatusCall)
	testutil.Equal(t, RefreshStatusError, store.lastStatus)
}

func TestRefreshNowRejectsDroppedMatview(t *testing.T) {
	t.Parallel()

	store := &fakeRefresherStore{
		entry: &Registration{
			ID:          "r1",
			SchemaName:  "public",
			ViewName:    "gone",
			RefreshMode: RefreshModeStandard,
		},
		exists: false,
	}

	svc := NewService(store)
	_, err := svc.RefreshNow(context.Background(), "r1")
	testutil.True(t, errors.Is(err, ErrNotMaterializedView), "expected ErrNotMaterializedView")
	testutil.Equal(t, 1, store.updateStatusCall)
	testutil.Equal(t, RefreshStatusError, store.lastStatus)
}
