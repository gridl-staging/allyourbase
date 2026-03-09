//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestNewPool(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        5,
		MinConns:        1,
		HealthCheckSecs: 0, // disable health check for fast test
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	// DB() should return a usable pool.
	testutil.NotNil(t, pool.DB())

	// Should be able to query.
	var result int
	err = pool.DB().QueryRow(ctx, "SELECT 1").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, result)
}

func TestNewPoolEmptyURL(t *testing.T) {
	ctx := context.Background()
	_, err := postgres.New(ctx, postgres.Config{
		URL: "",
	}, testutil.DiscardLogger())
	testutil.ErrorContains(t, err, "database URL is required")
}

func TestNewPoolInvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := postgres.New(ctx, postgres.Config{
		URL:      "postgresql://invalid:invalid@localhost:1/nodb",
		MaxConns: 1,
		MinConns: 0,
	}, testutil.DiscardLogger())
	// Should fail on ping.
	testutil.NotNil(t, err)
}

func TestPoolClose(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        2,
		MinConns:        1,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)

	// Close should not panic.
	pool.Close()

	// After close, queries should fail.
	err = pool.DB().Ping(ctx)
	testutil.NotNil(t, err)
}

func TestPoolCloseDoubleCallSafe(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        2,
		MinConns:        1,
		HealthCheckSecs: 0,
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)

	// Calling Close twice should not panic.
	pool.Close()
	pool.Close()
}

func TestPoolWithHealthCheck(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             sharedPG.ConnString,
		MaxConns:        2,
		MinConns:        1,
		HealthCheckSecs: 1, // 1 second interval
	}, testutil.DiscardLogger())
	testutil.NoError(t, err)
	defer pool.Close()

	// Pool should work with health check enabled.
	var result int
	err = pool.DB().QueryRow(ctx, "SELECT 42").Scan(&result)
	testutil.NoError(t, err)
	testutil.Equal(t, 42, result)
}
