package matview

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const registrationColumns = `id, schema_name, view_name, refresh_mode,
	last_refresh_at, last_refresh_duration_ms, last_refresh_status, last_refresh_error,
	created_at, updated_at`

// Store handles registry persistence and database-level refresh operations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new materialized view store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func scanRegistration(row pgx.Row) (*Registration, error) {
	var r Registration
	var status *string
	if err := row.Scan(
		&r.ID,
		&r.SchemaName,
		&r.ViewName,
		&r.RefreshMode,
		&r.LastRefreshAt,
		&r.LastRefreshDurationMs,
		&status,
		&r.LastRefreshError,
		&r.CreatedAt,
		&r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if status != nil {
		s := RefreshStatus(*status)
		r.LastRefreshStatus = &s
	}
	return &r, nil
}

func scanRegistrations(rows pgx.Rows) ([]Registration, error) {
	result := make([]Registration, 0)
	for rows.Next() {
		var r Registration
		var status *string
		if err := rows.Scan(
			&r.ID,
			&r.SchemaName,
			&r.ViewName,
			&r.RefreshMode,
			&r.LastRefreshAt,
			&r.LastRefreshDurationMs,
			&status,
			&r.LastRefreshError,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if status != nil {
			s := RefreshStatus(*status)
			r.LastRefreshStatus = &s
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// Register inserts a matview registration after validating identifiers and view existence.
func (s *Store) Register(ctx context.Context, schemaName, viewName string, mode RefreshMode) (*Registration, error) {
	if err := ValidateIdentifier(schemaName); err != nil {
		return nil, err
	}
	if err := ValidateIdentifier(viewName); err != nil {
		return nil, err
	}
	if err := validateRefreshMode(mode); err != nil {
		return nil, err
	}

	exists, _, err := s.MatviewState(ctx, schemaName, viewName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s.%s", ErrNotMaterializedView, schemaName, viewName)
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_matview_refreshes (schema_name, view_name, refresh_mode)
		 VALUES ($1, $2, $3)
		 RETURNING `+registrationColumns,
		schemaName, viewName, mode,
	)
	reg, err := scanRegistration(row)
	if err != nil {
		return nil, classifyDBErr(err)
	}
	return reg, nil
}

// Update changes the refresh mode for a registration.
func (s *Store) Update(ctx context.Context, id string, mode RefreshMode) (*Registration, error) {
	if err := validateRefreshMode(mode); err != nil {
		return nil, err
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_matview_refreshes
		 SET refresh_mode = $2, updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+registrationColumns,
		id, mode,
	)
	reg, err := scanRegistration(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrRegistrationNotFound, id)
	}
	if err != nil {
		return nil, classifyDBErr(err)
	}
	return reg, nil
}

// Delete removes a registration.
func (s *Store) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_matview_refreshes WHERE id = $1`, id)
	if err != nil {
		return classifyDBErr(err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrRegistrationNotFound, id)
	}
	return nil
}

// List returns all registered materialized views.
func (s *Store) List(ctx context.Context) ([]Registration, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+registrationColumns+`
		 FROM _ayb_matview_refreshes
		 ORDER BY schema_name, view_name`,
	)
	if err != nil {
		return nil, classifyDBErr(err)
	}
	defer rows.Close()

	regs, err := scanRegistrations(rows)
	if err != nil {
		return nil, classifyDBErr(err)
	}
	return regs, nil
}

// Get returns a registration by ID.
func (s *Store) Get(ctx context.Context, id string) (*Registration, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+registrationColumns+` FROM _ayb_matview_refreshes WHERE id = $1`,
		id,
	)
	reg, err := scanRegistration(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrRegistrationNotFound, id)
	}
	if err != nil {
		return nil, classifyDBErr(err)
	}
	return reg, nil
}

// GetByName returns a registration by schema/view name.
func (s *Store) GetByName(ctx context.Context, schemaName, viewName string) (*Registration, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+registrationColumns+`
		 FROM _ayb_matview_refreshes
		 WHERE schema_name = $1 AND view_name = $2`,
		schemaName, viewName,
	)
	reg, err := scanRegistration(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s.%s", ErrRegistrationNotFound, schemaName, viewName)
	}
	if err != nil {
		return nil, classifyDBErr(err)
	}
	return reg, nil
}

// UpdateRefreshStatus writes last refresh metadata.
func (s *Store) UpdateRefreshStatus(ctx context.Context, id string, status RefreshStatus, durationMs int, errText *string) error {
	var dur *int
	if durationMs >= 0 {
		dur = &durationMs
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_matview_refreshes
		 SET last_refresh_at = NOW(),
		     last_refresh_duration_ms = $2,
		     last_refresh_status = $3,
		     last_refresh_error = $4,
		     updated_at = NOW()
		 WHERE id = $1`,
		id, dur, status, errText,
	)
	return classifyDBErr(err)
}

// MatviewState returns whether the target exists as a materialized view and whether it's populated.
func (s *Store) MatviewState(ctx context.Context, schemaName, viewName string) (exists bool, populated bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT c.relispopulated
		 FROM pg_class c
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind = 'm'`,
		schemaName, viewName,
	).Scan(&populated)
	if err == pgx.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, classifyDBErr(err)
	}
	return true, populated, nil
}

// HasConcurrentUniqueIndex verifies CONCURRENTLY prerequisites for index shape.
func (s *Store) HasConcurrentUniqueIndex(ctx context.Context, schemaName, viewName string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM pg_index ix
			JOIN pg_class c ON c.oid = ix.indrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1
			  AND c.relname = $2
			  AND ix.indisunique = true
			  AND ix.indpred IS NULL
			  AND ix.indexprs IS NULL
		)`,
		schemaName, viewName,
	).Scan(&exists)
	if err != nil {
		return false, classifyDBErr(err)
	}
	return exists, nil
}

// LockedRefresh acquires an advisory lock, executes the refresh SQL, and releases
// the lock — all on a single dedicated connection from the pool. This is critical
// because PostgreSQL advisory locks are session-level (connection-level), so lock,
// refresh, and unlock MUST happen on the same connection.
func (s *Store) LockedRefresh(ctx context.Context, lockKey, sql string) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	var locked bool
	err = conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtext($1))`,
		lockKey,
	).Scan(&locked)
	if err != nil {
		return classifyDBErr(err)
	}
	if !locked {
		return ErrRefreshInProgress
	}
	// Release lock on the SAME connection. Use context.Background() so the unlock
	// succeeds even if the caller's context was cancelled during refresh.
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, lockKey)
	}()

	_, err = conn.Exec(ctx, sql)
	return classifyDBErr(err)
}

func classifyDBErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return fmt.Errorf("%w: %w", ErrDuplicateRegistration, err)
		}
	}
	return err
}
