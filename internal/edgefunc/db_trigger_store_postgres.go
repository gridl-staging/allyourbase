package edgefunc

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ DBTriggerStore = (*DBTriggerPostgresStore)(nil)

const dbTriggerColumns = `id, function_id, table_name, schema_name, events, filter_columns, enabled, created_at, updated_at`

// DBTriggerPostgresStore implements DBTriggerStore using PostgreSQL.
type DBTriggerPostgresStore struct {
	pool *pgxpool.Pool
}

// NewDBTriggerPostgresStore creates a new Postgres-backed DB trigger store.
func NewDBTriggerPostgresStore(pool *pgxpool.Pool) *DBTriggerPostgresStore {
	return &DBTriggerPostgresStore{pool: pool}
}

// CreateDBTrigger inserts a new DB trigger record.
func (s *DBTriggerPostgresStore) CreateDBTrigger(ctx context.Context, trigger *DBTrigger) (*DBTrigger, error) {
	// Convert []DBTriggerEvent to []string for Postgres TEXT[]
	events := dbTriggerEventsToStrings(trigger.Events)

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_edge_db_triggers (function_id, table_name, schema_name, events, filter_columns, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+dbTriggerColumns,
		trigger.FunctionID, trigger.TableName, trigger.Schema,
		events, nullableStringSlice(trigger.FilterColumns), trigger.Enabled,
	)
	return scanDBTrigger(row)
}

// GetDBTrigger retrieves a DB trigger by ID.
func (s *DBTriggerPostgresStore) GetDBTrigger(ctx context.Context, id string) (*DBTrigger, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+dbTriggerColumns+` FROM _ayb_edge_db_triggers WHERE id = $1`, id,
	)
	return scanDBTrigger(row)
}

// ListDBTriggers returns all DB triggers for a function.
func (s *DBTriggerPostgresStore) ListDBTriggers(ctx context.Context, functionID string) ([]*DBTrigger, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+dbTriggerColumns+` FROM _ayb_edge_db_triggers
		 WHERE function_id = $1 ORDER BY created_at`, functionID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing db triggers: %w", err)
	}
	defer rows.Close()
	return scanDBTriggers(rows)
}

// ListDBTriggersByTable returns all enabled DB triggers for a specific table.
func (s *DBTriggerPostgresStore) ListDBTriggersByTable(ctx context.Context, schema, tableName string) ([]*DBTrigger, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+dbTriggerColumns+` FROM _ayb_edge_db_triggers
		 WHERE schema_name = $1 AND table_name = $2 AND enabled = true
		 ORDER BY created_at`, schema, tableName,
	)
	if err != nil {
		return nil, fmt.Errorf("listing db triggers by table: %w", err)
	}
	defer rows.Close()
	return scanDBTriggers(rows)
}

// UpdateDBTrigger updates a DB trigger.
func (s *DBTriggerPostgresStore) UpdateDBTrigger(ctx context.Context, trigger *DBTrigger) (*DBTrigger, error) {
	events := dbTriggerEventsToStrings(trigger.Events)

	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_edge_db_triggers SET
			table_name = $2, schema_name = $3, events = $4, filter_columns = $5,
			enabled = $6, updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+dbTriggerColumns,
		trigger.ID, trigger.TableName, trigger.Schema,
		events, nullableStringSlice(trigger.FilterColumns), trigger.Enabled,
	)

	result, err := scanDBTrigger(row)
	if errors.Is(err, ErrDBTriggerNotFound) {
		return nil, ErrDBTriggerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("updating db trigger: %w", err)
	}
	return result, nil
}

// DeleteDBTrigger deletes a DB trigger by ID.
func (s *DBTriggerPostgresStore) DeleteDBTrigger(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_edge_db_triggers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting db trigger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDBTriggerNotFound
	}
	return nil
}

// InstallTrigger creates the PostgreSQL trigger on the target table.
func (s *DBTriggerPostgresStore) InstallTrigger(ctx context.Context, trigger *DBTrigger) error {
	if _, err := s.pool.Exec(ctx, InstallTriggerSQL(trigger)); err != nil {
		return fmt.Errorf("install trigger %s on %s.%s: %w",
			trigger.ID, trigger.Schema, trigger.TableName, err)
	}
	return nil
}

// RemoveTrigger drops the PostgreSQL trigger from the target table.
func (s *DBTriggerPostgresStore) RemoveTrigger(ctx context.Context, trigger *DBTrigger) error {
	if _, err := s.pool.Exec(ctx, RemoveTriggerSQL(trigger)); err != nil {
		return fmt.Errorf("remove trigger %s on %s.%s: %w",
			trigger.ID, trigger.Schema, trigger.TableName, err)
	}
	return nil
}

// SetTriggerEnabled enables or disables the PostgreSQL trigger on the target table.
func (s *DBTriggerPostgresStore) SetTriggerEnabled(ctx context.Context, trigger *DBTrigger, enabled bool) error {
	sql := DisableTriggerSQL(trigger)
	if enabled {
		sql = EnableTriggerSQL(trigger)
	}
	if _, err := s.pool.Exec(ctx, sql); err != nil {
		state := "disable"
		if enabled {
			state = "enable"
		}
		return fmt.Errorf("%s trigger %s on %s.%s: %w",
			state, trigger.ID, trigger.Schema, trigger.TableName, err)
	}
	return nil
}

// scanDBTrigger scans a single DB trigger row.
func scanDBTrigger(row pgx.Row) (*DBTrigger, error) {
	var t DBTrigger
	var events []string
	var filterColumns []string

	err := row.Scan(
		&t.ID, &t.FunctionID, &t.TableName, &t.Schema,
		&events, &filterColumns, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDBTriggerNotFound
		}
		return nil, fmt.Errorf("scanning db trigger: %w", err)
	}

	t.Events = stringsToDBTriggerEvents(events)
	t.FilterColumns = filterColumns

	return &t, nil
}

// scanDBTriggers scans multiple DB trigger rows.
func scanDBTriggers(rows pgx.Rows) ([]*DBTrigger, error) {
	var result []*DBTrigger
	for rows.Next() {
		t, err := scanDBTrigger(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning db trigger row: %w", err)
		}
		result = append(result, t)
	}
	if result == nil {
		result = []*DBTrigger{}
	}
	return result, rows.Err()
}

// dbTriggerEventsToStrings converts typed events to string slice for Postgres.
func dbTriggerEventsToStrings(events []DBTriggerEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = string(ev)
	}
	return out
}

// stringsToDBTriggerEvents converts string slice from Postgres to typed events.
func stringsToDBTriggerEvents(events []string) []DBTriggerEvent {
	out := make([]DBTriggerEvent, len(events))
	for i, ev := range events {
		out[i] = DBTriggerEvent(ev)
	}
	return out
}

// nullableStringSlice returns nil if the slice is empty (stores NULL in Postgres).
func nullableStringSlice(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
