// Package storage provides row-level security for database operations through transaction context management based on authentication claims.
package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type dbQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type txFinalizer interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// withRLS returns a database querier with row-level security context applied if authentication claims are in ctx, otherwise the service pool. It also returns a finalizer function to handle transaction commit/rollback based on the query result, and any setup error.
func (s *Service) withRLS(ctx context.Context) (dbQuerier, func(error) error, error) {
	if s.pool == nil {
		return nil, nil, fmt.Errorf("database pool is not configured")
	}

	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return s.pool, func(error) error { return nil }, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}

	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}

	done := func(queryErr error) error { return finalizeStorageTx(ctx, tx, queryErr, s.logger) }
	return tx, done, nil
}

func finalizeStorageTx(ctx context.Context, tx txFinalizer, queryErr error, logger *slog.Logger) error {
	if queryErr != nil {
		if err := tx.Rollback(ctx); err != nil && logger != nil {
			logger.Error("storage tx rollback failed", "error", err)
		}
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		if logger != nil {
			logger.Error("storage tx commit failed", "error", err)
		}
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
