package edgefunc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ Store = (*PostgresStore)(nil)

const funcColumns = `id, name, entry_point, source, compiled_js, timeout_ms, env_vars, public, created_at, updated_at`

// PostgresStore implements Store using a pgxpool connection to _ayb_edge_functions.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Create inserts a new edge function. Returns ErrFunctionNameConflict on duplicate name.
func (s *PostgresStore) Create(ctx context.Context, fn *EdgeFunction) (*EdgeFunction, error) {
	envJSON, err := marshalEnvVars(fn.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("marshaling env vars: %w", err)
	}

	timeoutMs := timeoutToMs(fn.Timeout)

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_edge_functions (name, entry_point, source, compiled_js, timeout_ms, env_vars, public)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+funcColumns,
		fn.Name, fn.EntryPoint, fn.Source, fn.CompiledJS, timeoutMs, envJSON, fn.Public,
	)

	result, err := scanEdgeFunction(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%w: %s", ErrFunctionNameConflict, fn.Name)
		}
		return nil, fmt.Errorf("creating edge function: %w", err)
	}
	return result, nil
}

// Get returns an edge function by ID. Returns ErrFunctionNotFound if not found.
func (s *PostgresStore) Get(ctx context.Context, id uuid.UUID) (*EdgeFunction, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+funcColumns+` FROM _ayb_edge_functions WHERE id = $1`, id,
	)
	fn, err := scanEdgeFunction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("getting edge function: %w", err)
	}
	return fn, nil
}

// GetByName returns an edge function by name. Returns ErrFunctionNotFound if not found.
func (s *PostgresStore) GetByName(ctx context.Context, name string) (*EdgeFunction, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+funcColumns+` FROM _ayb_edge_functions WHERE name = $1`, name,
	)
	fn, err := scanEdgeFunction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrFunctionNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("getting edge function by name: %w", err)
	}
	return fn, nil
}

// List returns edge functions ordered by name with pagination.
// page and perPage of 0 use defaults (page 1, perPage 50).
func (s *PostgresStore) List(ctx context.Context, page, perPage int) ([]*EdgeFunction, error) {
	if perPage <= 0 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage

	rows, err := s.pool.Query(ctx,
		`SELECT `+funcColumns+` FROM _ayb_edge_functions ORDER BY name LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing edge functions: %w", err)
	}
	defer rows.Close()

	return scanEdgeFunctions(rows)
}

// Update updates a mutable edge function fields by ID. Returns ErrFunctionNotFound if not found.
func (s *PostgresStore) Update(ctx context.Context, fn *EdgeFunction) (*EdgeFunction, error) {
	envJSON, err := marshalEnvVars(fn.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("marshaling env vars: %w", err)
	}

	timeoutMs := timeoutToMs(fn.Timeout)

	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_edge_functions SET
			entry_point = $2,
			source = $3,
			compiled_js = $4,
			timeout_ms = $5,
			env_vars = $6,
			public = $7,
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+funcColumns,
		fn.ID, fn.EntryPoint, fn.Source, fn.CompiledJS, timeoutMs, envJSON, fn.Public,
	)

	result, err := scanEdgeFunction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, fn.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("updating edge function: %w", err)
	}
	return result, nil
}

// Delete removes an edge function by ID. Returns ErrFunctionNotFound if not found.
func (s *PostgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_edge_functions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting edge function: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrFunctionNotFound, id)
	}
	return nil
}

// scanEdgeFunction reads a single row into an EdgeFunction, converting
// timeout_ms → time.Duration and env_vars JSONB → map[string]string.
func scanEdgeFunction(row pgx.Row) (*EdgeFunction, error) {
	var fn EdgeFunction
	var timeoutMs int
	var envJSON []byte

	err := row.Scan(
		&fn.ID, &fn.Name, &fn.EntryPoint, &fn.Source, &fn.CompiledJS,
		&timeoutMs, &envJSON, &fn.Public, &fn.CreatedAt, &fn.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	fn.Timeout = time.Duration(timeoutMs) * time.Millisecond
	fn.EnvVars = make(map[string]string)
	if len(envJSON) > 0 {
		if err := json.Unmarshal(envJSON, &fn.EnvVars); err != nil {
			return nil, fmt.Errorf("unmarshaling env vars: %w", err)
		}
	}
	return &fn, nil
}

// scanEdgeFunctions reads multiple rows into a slice of EdgeFunction pointers.
// Delegates to scanEdgeFunction for each row (pgx.Rows satisfies pgx.Row).
func scanEdgeFunctions(rows pgx.Rows) ([]*EdgeFunction, error) {
	var result []*EdgeFunction
	for rows.Next() {
		fn, err := scanEdgeFunction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning edge function row: %w", err)
		}
		result = append(result, fn)
	}
	if result == nil {
		result = []*EdgeFunction{}
	}
	return result, rows.Err()
}

// timeoutToMs converts a Duration to milliseconds, applying DefaultTimeout when zero.
func timeoutToMs(d time.Duration) int {
	if d <= 0 {
		return int(DefaultTimeout / time.Millisecond)
	}
	return int(d / time.Millisecond)
}

// marshalEnvVars converts env vars map to JSON bytes, defaulting nil to empty object.
func marshalEnvVars(envVars map[string]string) ([]byte, error) {
	if envVars == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(envVars)
}
