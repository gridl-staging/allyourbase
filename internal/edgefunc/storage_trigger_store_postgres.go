package edgefunc

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ StorageTriggerStore = (*StorageTriggerPostgresStore)(nil)

const storageTriggerColumns = `id, function_id, bucket, event_types, prefix_filter, suffix_filter, enabled, created_at, updated_at`

// StorageTriggerPostgresStore implements StorageTriggerStore using PostgreSQL.
type StorageTriggerPostgresStore struct {
	pool *pgxpool.Pool
}

// NewStorageTriggerPostgresStore creates a new Postgres-backed storage trigger store.
func NewStorageTriggerPostgresStore(pool *pgxpool.Pool) *StorageTriggerPostgresStore {
	return &StorageTriggerPostgresStore{pool: pool}
}

// CreateStorageTrigger inserts a new storage trigger record.
func (s *StorageTriggerPostgresStore) CreateStorageTrigger(ctx context.Context, trigger *StorageTrigger) (*StorageTrigger, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_edge_storage_triggers (function_id, bucket, event_types, prefix_filter, suffix_filter, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+storageTriggerColumns,
		trigger.FunctionID, trigger.Bucket, trigger.EventTypes,
		nullableString(trigger.PrefixFilter), nullableString(trigger.SuffixFilter), trigger.Enabled,
	)
	return scanStorageTrigger(row)
}

// GetStorageTrigger retrieves a storage trigger by ID.
func (s *StorageTriggerPostgresStore) GetStorageTrigger(ctx context.Context, id string) (*StorageTrigger, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+storageTriggerColumns+` FROM _ayb_edge_storage_triggers WHERE id = $1`, id,
	)
	return scanStorageTrigger(row)
}

// ListStorageTriggers returns all storage triggers for a function.
func (s *StorageTriggerPostgresStore) ListStorageTriggers(ctx context.Context, functionID string) ([]*StorageTrigger, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+storageTriggerColumns+` FROM _ayb_edge_storage_triggers
		 WHERE function_id = $1 ORDER BY created_at`, functionID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing storage triggers: %w", err)
	}
	defer rows.Close()
	return scanStorageTriggers(rows)
}

// ListStorageTriggersByBucket returns all enabled storage triggers for a bucket.
func (s *StorageTriggerPostgresStore) ListStorageTriggersByBucket(ctx context.Context, bucket string) ([]*StorageTrigger, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+storageTriggerColumns+` FROM _ayb_edge_storage_triggers
		 WHERE bucket = $1 AND enabled = true ORDER BY created_at`, bucket,
	)
	if err != nil {
		return nil, fmt.Errorf("listing storage triggers by bucket: %w", err)
	}
	defer rows.Close()
	return scanStorageTriggers(rows)
}

// UpdateStorageTrigger updates a storage trigger.
func (s *StorageTriggerPostgresStore) UpdateStorageTrigger(ctx context.Context, trigger *StorageTrigger) (*StorageTrigger, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE _ayb_edge_storage_triggers SET
			bucket = $2, event_types = $3, prefix_filter = $4, suffix_filter = $5,
			enabled = $6, updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+storageTriggerColumns,
		trigger.ID, trigger.Bucket, trigger.EventTypes,
		nullableString(trigger.PrefixFilter), nullableString(trigger.SuffixFilter), trigger.Enabled,
	)

	result, err := scanStorageTrigger(row)
	if errors.Is(err, ErrStorageTriggerNotFound) {
		return nil, ErrStorageTriggerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("updating storage trigger: %w", err)
	}
	return result, nil
}

// DeleteStorageTrigger deletes a storage trigger by ID.
func (s *StorageTriggerPostgresStore) DeleteStorageTrigger(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_edge_storage_triggers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting storage trigger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrStorageTriggerNotFound
	}
	return nil
}

// scanStorageTrigger scans a single storage trigger row.
func scanStorageTrigger(row pgx.Row) (*StorageTrigger, error) {
	var t StorageTrigger
	var prefix, suffix *string

	err := row.Scan(
		&t.ID, &t.FunctionID, &t.Bucket, &t.EventTypes,
		&prefix, &suffix, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrStorageTriggerNotFound
		}
		return nil, fmt.Errorf("scanning storage trigger: %w", err)
	}

	t.PrefixFilter = derefString(prefix)
	t.SuffixFilter = derefString(suffix)

	return &t, nil
}

// scanStorageTriggers scans multiple storage trigger rows.
func scanStorageTriggers(rows pgx.Rows) ([]*StorageTrigger, error) {
	var result []*StorageTrigger
	for rows.Next() {
		t, err := scanStorageTrigger(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning storage trigger row: %w", err)
		}
		result = append(result, t)
	}
	if result == nil {
		result = []*StorageTrigger{}
	}
	return result, rows.Err()
}
