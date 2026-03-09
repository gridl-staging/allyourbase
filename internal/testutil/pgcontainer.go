//go:build integration

package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGContainer holds a connection pool to a temporary test database.
type PGContainer struct {
	Pool       *pgxpool.Pool
	ConnString string
	baseURL    string
	dbName     string
}

// Cleanup closes the pool and drops the temporary database.
func (pg *PGContainer) Cleanup() {
	pg.Pool.Close()
	cp, err := pgxpool.New(context.Background(), pg.baseURL)
	if err == nil {
		cp.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pg.dbName+" WITH (FORCE)")
		cp.Close()
	}
}

// StartPostgresForTestMain connects to TEST_DATABASE_URL, creates a temporary
// database for isolation, and returns a pool + cleanup function.
// Panics on failure since TestMain has no *testing.T.
func StartPostgresForTestMain(ctx context.Context) (*PGContainer, func()) {
	baseURL := os.Getenv("TEST_DATABASE_URL")
	if baseURL == "" {
		panic("TEST_DATABASE_URL is not set. Use `make test-integration` or set it manually.")
	}

	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	adminPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		panic(fmt.Sprintf("connecting to TEST_DATABASE_URL: %v", err))
	}
	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		adminPool.Close()
		panic(fmt.Sprintf("creating temp database %s: %v", dbName, err))
	}
	adminPool.Close()

	tempURL, err := replaceDBInURL(baseURL, dbName)
	if err != nil {
		panic(fmt.Sprintf("building temp database URL: %v", err))
	}

	pool, err := pgxpool.New(ctx, tempURL)
	if err != nil {
		panic(fmt.Sprintf("connecting to temp database: %v", err))
	}
	if err := pool.Ping(ctx); err != nil {
		panic(fmt.Sprintf("pinging temp database: %v", err))
	}

	pg := &PGContainer{
		Pool:       pool,
		ConnString: tempURL,
		baseURL:    baseURL,
		dbName:     dbName,
	}
	return pg, pg.Cleanup
}

func replaceDBInURL(connStr, newDB string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", err
	}
	u.Path = "/" + newDB
	return u.String(), nil
}
