//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/testutil"
)

// TestPoolExhaustionRecovery verifies that when all pool connections are
// occupied, new queries block (up to a timeout), and after connections are
// released the pool recovers and serves new queries normally.
func TestPoolExhaustionRecovery(t *testing.T) {
	ctx := context.Background()

	// Create a very small pool (2 connections max) to make exhaustion easy.
	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        2,
		MinConns:        0,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	// Acquire both connections with pg_sleep so they stay occupied.
	var wg sync.WaitGroup
	var sleepErrors atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine holds a connection for 2 seconds.
			_, err := pool.DB().Exec(ctx, "SELECT pg_sleep(2)")
			if err != nil {
				sleepErrors.Add(1)
			}
		}()
	}

	// Give the goroutines a moment to acquire their connections.
	time.Sleep(200 * time.Millisecond)

	// Try a query with a short timeout — should fail because pool is exhausted.
	shortCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err = pool.DB().Ping(shortCtx)
	// This should either timeout or fail because no connections are available.
	testutil.Error(t, err)

	// Wait for the sleep goroutines to finish and release connections.
	wg.Wait()
	testutil.Equal(t, int32(0), sleepErrors.Load())

	// Pool should now be recovered — new queries should succeed.
	var result int
	err = pool.DB().QueryRow(ctx, "SELECT 1").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result)
}

// TestStatementTimeout verifies that statement_timeout properly cancels
// long-running queries and the connection remains usable afterward.
func TestStatementTimeout(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        2,
		MinConns:        0,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	db := pool.DB()

	// Set a very short statement_timeout and run a long query.
	// The statement_timeout is session-level, so we need to acquire a
	// connection, set the timeout, and run the query in one transaction.
	tx, err := db.Begin(ctx)
	testutil.NoError(t, err)

	// Set 200ms statement timeout on this transaction's session.
	_, err = tx.Exec(ctx, "SET LOCAL statement_timeout = '200ms'")
	testutil.NoError(t, err)

	// Run a query that takes longer than the timeout — should be cancelled.
	_, err = tx.Exec(ctx, "SELECT pg_sleep(10)")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "cancel")

	// Rollback the failed transaction.
	err = tx.Rollback(ctx)
	testutil.NoError(t, err)

	// The pool should still be healthy — run a normal query.
	var result int
	err = db.QueryRow(ctx, "SELECT 42").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 42, result)
}

// TestContextCancellationRecovery verifies that cancelling a query via context
// does not permanently break the pool — the connection is returned and
// subsequent queries succeed.
func TestContextCancellationRecovery(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        2,
		MinConns:        0,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	db := pool.DB()

	// Start a long query, then cancel the context.
	cancelCtx, cancel := context.WithCancel(ctx)
	var queryErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, queryErr = db.Exec(cancelCtx, "SELECT pg_sleep(30)")
	}()

	// Give the query time to start, then cancel.
	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()

	// The query should have been cancelled.
	testutil.Error(t, queryErr)

	// Pool should recover — subsequent queries should succeed.
	var result int
	err = db.QueryRow(ctx, "SELECT 99").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 99, result)
}

// TestConcurrentDDLStress runs concurrent CREATE and DROP TABLE operations
// to verify the pool handles DDL contention gracefully without panics or
// permanent connection corruption.
func TestConcurrentDDLStress(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        5,
		MinConns:        0,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	db := pool.DB()

	// Pre-clean any leftover tables from previous test runs.
	for i := 0; i < 10; i++ {
		db.Exec(ctx, "DROP TABLE IF EXISTS chaos_ddl_"+string(rune('a'+i)))
	}

	// Run 10 concurrent goroutines, each creating and dropping its own table.
	var wg sync.WaitGroup
	var errors atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tableName := "chaos_ddl_" + string(rune('a'+idx))
			for iter := 0; iter < 5; iter++ {
				_, err := db.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+tableName+" (id serial PRIMARY KEY, val text)")
				if err != nil {
					errors.Add(1)
					return
				}
				_, err = db.Exec(ctx, "INSERT INTO "+tableName+" (val) VALUES ('iter-"+string(rune('0'+iter))+"')")
				if err != nil {
					errors.Add(1)
					return
				}
				_, err = db.Exec(ctx, "DROP TABLE IF EXISTS "+tableName)
				if err != nil {
					errors.Add(1)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	testutil.Equal(t, int32(0), errors.Load())

	// Pool should still be healthy after DDL stress.
	var result int
	err = db.QueryRow(ctx, "SELECT 1").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result)
}

// TestTransactionRollbackUnderLoad verifies that transaction rollbacks under
// concurrent load don't corrupt the pool or leak connections.
func TestTransactionRollbackUnderLoad(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        4,
		MinConns:        0,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	db := pool.DB()

	// Create a test table.
	_, err = db.Exec(ctx, "CREATE TABLE IF NOT EXISTS chaos_rollback_test (id serial PRIMARY KEY, val int NOT NULL)")
	testutil.NoError(t, err)
	defer db.Exec(ctx, "DROP TABLE IF EXISTS chaos_rollback_test")

	// Run 8 concurrent transactions, alternating between commit and rollback.
	var wg sync.WaitGroup
	var rollbackErrors atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tx, err := db.Begin(ctx)
			if err != nil {
				rollbackErrors.Add(1)
				return
			}

			// Insert a row in the transaction.
			_, err = tx.Exec(ctx, "INSERT INTO chaos_rollback_test (val) VALUES ($1)", idx)
			if err != nil {
				tx.Rollback(ctx)
				rollbackErrors.Add(1)
				return
			}

			// Even-indexed goroutines commit, odd ones rollback.
			if idx%2 == 0 {
				if err := tx.Commit(ctx); err != nil {
					rollbackErrors.Add(1)
				}
			} else {
				if err := tx.Rollback(ctx); err != nil {
					rollbackErrors.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()
	testutil.Equal(t, int32(0), rollbackErrors.Load())

	// Only even-indexed rows (0, 2, 4, 6) should have been committed.
	var count int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM chaos_rollback_test").Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 4, count)

	// Pool is still healthy.
	var result int
	err = db.QueryRow(ctx, "SELECT 1").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result)
}
