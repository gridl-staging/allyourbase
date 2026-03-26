// Package edgefunc PostgresLogStore implements the LogStore interface, providing methods to persist edge function execution logs in PostgreSQL and query them with filtering and pagination.
package edgefunc

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ LogStore = (*PostgresLogStore)(nil)

const logColumns = `id, function_id, invocation_id, status, duration_ms, stdout, stdout_bytes, error, response_status_code, request_method, request_path, trigger_type, trigger_id, parent_invocation_id, created_at`

// PostgresLogStore implements LogStore using a pgxpool connection.
type PostgresLogStore struct {
	pool *pgxpool.Pool
}

// NewPostgresLogStore creates a new PostgresLogStore.
func NewPostgresLogStore(pool *pgxpool.Pool) *PostgresLogStore {
	return &PostgresLogStore{pool: pool}
}

// WriteLog inserts a log entry into the _ayb_edge_function_logs table.
func (s *PostgresLogStore) WriteLog(ctx context.Context, entry *LogEntry) error {
	if entry.InvocationID == uuid.Nil {
		entry.InvocationID = uuid.New()
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_edge_function_logs (function_id, invocation_id, status, duration_ms, stdout, stdout_bytes, error, response_status_code, request_method, request_path, trigger_type, trigger_id, parent_invocation_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING `+logColumns,
		entry.FunctionID, entry.InvocationID, entry.Status, entry.DurationMs,
		nullableString(entry.Stdout), entry.StdoutBytes,
		nullableString(entry.Error), entry.ResponseStatusCode,
		nullableString(entry.RequestMethod), nullableString(entry.RequestPath),
		nullableString(entry.TriggerType), nullableString(entry.TriggerID),
		nullableString(entry.ParentInvocationID),
	)

	return scanLogEntryInto(row, entry)
}

// ListByFunction returns log entries for a function, ordered by created_at DESC (newest first).
func (s *PostgresLogStore) ListByFunction(ctx context.Context, functionID uuid.UUID, opts LogListOptions) ([]*LogEntry, error) {
	normalized, err := normalizeLogListOptions(opts)
	if err != nil {
		return nil, err
	}

	query, args := buildListByFunctionQuery(functionID, normalized)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing function logs: %w", err)
	}
	defer rows.Close()

	return scanLogEntries(rows)
}

// buildListByFunctionQuery constructs a parameterized SQL query and arguments for listing edge function logs by function ID with optional filtering by status, trigger type, and creation time range, and pagination via LIMIT and OFFSET.
func buildListByFunctionQuery(functionID uuid.UUID, opts LogListOptions) (string, []any) {
	where := []string{"function_id = $1"}
	args := []any{functionID}
	argN := 2

	if opts.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argN))
		args = append(args, opts.Status)
		argN++
	}
	if opts.TriggerType != "" {
		where = append(where, fmt.Sprintf("trigger_type = $%d", argN))
		args = append(args, opts.TriggerType)
		argN++
	}
	if opts.Since != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", argN))
		args = append(args, *opts.Since)
		argN++
	}
	if opts.Until != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", argN))
		args = append(args, *opts.Until)
		argN++
	}

	offset := (opts.Page - 1) * opts.PerPage
	query := `SELECT ` + logColumns + ` FROM _ayb_edge_function_logs
		 WHERE ` + strings.Join(where, " AND ") + `
		 ORDER BY created_at DESC
		 LIMIT $` + fmt.Sprintf("%d", argN) + ` OFFSET $` + fmt.Sprintf("%d", argN+1)
	args = append(args, opts.PerPage, offset)
	return query, args
}

// scanLogEntryInto scans a single row into an existing LogEntry.
func scanLogEntryInto(row pgx.Row, entry *LogEntry) error {
	var stdout, errStr, reqMethod, reqPath *string
	var triggerType, triggerID, parentInvocationID *string
	err := row.Scan(
		&entry.ID, &entry.FunctionID, &entry.InvocationID,
		&entry.Status, &entry.DurationMs,
		&stdout, &entry.StdoutBytes, &errStr, &entry.ResponseStatusCode, &reqMethod, &reqPath,
		&triggerType, &triggerID, &parentInvocationID,
		&entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("scanning log entry: %w", err)
	}
	entry.Stdout = derefString(stdout)
	entry.Error = derefString(errStr)
	entry.RequestMethod = derefString(reqMethod)
	entry.RequestPath = derefString(reqPath)
	entry.TriggerType = derefString(triggerType)
	entry.TriggerID = derefString(triggerID)
	entry.ParentInvocationID = derefString(parentInvocationID)
	return nil
}

// scanLogEntries reads multiple rows into a slice of LogEntry pointers.
// Delegates to scanLogEntryInto for each row (pgx.Rows satisfies pgx.Row).
func scanLogEntries(rows pgx.Rows) ([]*LogEntry, error) {
	var result []*LogEntry
	for rows.Next() {
		entry := &LogEntry{}
		if err := scanLogEntryInto(rows, entry); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if result == nil {
		result = []*LogEntry{}
	}
	return result, rows.Err()
}

// nullableString returns nil for empty strings (maps to SQL NULL).
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// derefString returns the string value or empty string for nil pointers.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
