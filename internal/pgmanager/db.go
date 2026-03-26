package pgmanager

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql
)

type rowScanner interface {
	Scan(dest ...any) error
}

type databaseClient interface {
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Close() error
}

type sqlDBClient struct {
	*sql.DB
}

func (db sqlDBClient) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return db.DB.QueryRowContext(ctx, query, args...)
}

// openPgx opens a database/sql connection using the pgx driver.
func openPgx(ctx context.Context, connURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", connURL)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openPgxClient(ctx context.Context, connURL string) (databaseClient, error) {
	db, err := openPgx(ctx, connURL)
	if err != nil {
		return nil, err
	}
	return sqlDBClient{DB: db}, nil
}

func managedConnURL(port uint32, database string) string {
	return fmt.Sprintf("postgresql://%s:%s@127.0.0.1:%d/%s?sslmode=disable",
		dbUser, dbPass, port, database)
}
