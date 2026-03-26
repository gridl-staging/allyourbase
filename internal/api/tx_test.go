package api

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeTxFinalizer struct {
	commitCalls   int
	rollbackCalls int
	commitErr     error
	rollbackErr   error
}

func (f *fakeTxFinalizer) Commit(context.Context) error {
	f.commitCalls++
	return f.commitErr
}

func (f *fakeTxFinalizer) Rollback(context.Context) error {
	f.rollbackCalls++
	return f.rollbackErr
}

func TestFinalizeTxReturnsCommitError(t *testing.T) {
	t.Parallel()
	tx := &fakeTxFinalizer{commitErr: errors.New("commit failed")}

	err := finalizeTx(context.Background(), tx, nil, slog.Default())
	testutil.ErrorContains(t, err, "commit failed")
	testutil.Equal(t, 1, tx.commitCalls)
	testutil.Equal(t, 0, tx.rollbackCalls)
}

func TestFinalizeTxRollsBackOnQueryError(t *testing.T) {
	t.Parallel()
	tx := &fakeTxFinalizer{}

	err := finalizeTx(context.Background(), tx, errors.New("query failed"), slog.Default())
	testutil.NoError(t, err)
	testutil.Equal(t, 0, tx.commitCalls)
	testutil.Equal(t, 1, tx.rollbackCalls)
}
