package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeSessionIDRows struct {
	ids         []string
	index       int
	scanErrAt   int
	scanErr     error
	iterErr     error
	closed      bool
	currentItem string
}

func (r *fakeSessionIDRows) Next() bool {
	if r.index >= len(r.ids) {
		return false
	}
	r.currentItem = r.ids[r.index]
	r.index++
	return true
}

func (r *fakeSessionIDRows) Scan(dest ...any) error {
	if r.scanErr != nil && r.index-1 == r.scanErrAt {
		return r.scanErr
	}
	if len(dest) != 1 {
		return errors.New("expected single destination")
	}
	out, ok := dest[0].(*string)
	if !ok {
		return errors.New("destination is not *string")
	}
	*out = r.currentItem
	return nil
}

func (r *fakeSessionIDRows) Err() error {
	return r.iterErr
}

func (r *fakeSessionIDRows) Close() {
	r.closed = true
}

func TestDenyListFromSessionRowsAddsReturnedIDs(t *testing.T) {
	t.Parallel()

	svc := &Service{
		tokenDur: time.Hour,
		denyList: NewTokenDenyList(),
	}
	rows := &fakeSessionIDRows{
		ids: []string{"session-1", "session-2"},
	}

	err := svc.denyListFromSessionRows(rows)
	testutil.NoError(t, err)
	testutil.True(t, rows.closed, "rows should be closed")
	testutil.True(t, svc.denyList.IsDenied("session-1"), "session-1 should be denied")
	testutil.True(t, svc.denyList.IsDenied("session-2"), "session-2 should be denied")
}

func TestDenyListFromSessionRowsReturnsScanError(t *testing.T) {
	t.Parallel()

	svc := &Service{
		tokenDur: time.Hour,
		denyList: NewTokenDenyList(),
	}
	rows := &fakeSessionIDRows{
		ids:       []string{"session-1"},
		scanErrAt: 0,
		scanErr:   errors.New("scan failed"),
	}

	err := svc.denyListFromSessionRows(rows)
	testutil.ErrorContains(t, err, "scanning revoked session id")
	testutil.True(t, rows.closed, "rows should be closed")
}

func TestDenyListFromSessionRowsReturnsIterationError(t *testing.T) {
	t.Parallel()

	svc := &Service{
		tokenDur: time.Hour,
		denyList: NewTokenDenyList(),
	}
	rows := &fakeSessionIDRows{
		ids:     []string{"session-1"},
		iterErr: errors.New("rows failed"),
	}

	err := svc.denyListFromSessionRows(rows)
	testutil.ErrorContains(t, err, "iterating revoked session ids")
	testutil.True(t, rows.closed, "rows should be closed")
}
