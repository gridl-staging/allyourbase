package edgefunc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time interface check.
var _ DBTriggerEventStore = (*DBTriggerEventPostgresStore)(nil)

// DBTriggerEventPostgresStore implements DBTriggerEventStore using PostgreSQL.
type DBTriggerEventPostgresStore struct {
	pool *pgxpool.Pool
}

// NewDBTriggerEventPostgresStore creates a new Postgres-backed event queue store.
func NewDBTriggerEventPostgresStore(pool *pgxpool.Pool) *DBTriggerEventPostgresStore {
	return &DBTriggerEventPostgresStore{pool: pool}
}

// ClaimPendingEvents atomically claims up to limit pending events for processing
// using FOR UPDATE SKIP LOCKED to avoid contention. Also recovers stale "processing"
// events that were abandoned after a worker crash (stuck for > 5 minutes).
func (s *DBTriggerEventPostgresStore) ClaimPendingEvents(ctx context.Context, limit int) ([]*DBTriggerQueueEvent, error) {
	rows, err := s.pool.Query(ctx,
		`UPDATE _ayb_edge_trigger_events SET
			status = 'processing',
			attempts = attempts + 1,
			processed_at = NOW()
		WHERE id IN (
			SELECT id FROM _ayb_edge_trigger_events
			WHERE (
				(status IN ('pending', 'failed') AND attempts < $1)
				OR (status = 'processing' AND processed_at < NOW() - INTERVAL '5 minutes')
			)
			ORDER BY created_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, trigger_id, table_name, schema_name, operation, row_id, payload, status, attempts, created_at, processed_at`,
		MaxDBTriggerRetries, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claiming pending events: %w", err)
	}
	defer rows.Close()

	var events []*DBTriggerQueueEvent
	for rows.Next() {
		var e DBTriggerQueueEvent
		var payload []byte
		var rowID *string

		err := rows.Scan(
			&e.ID, &e.TriggerID, &e.TableName, &e.SchemaName,
			&e.Operation, &rowID, &payload, &e.Status, &e.Attempts,
			&e.CreatedAt, &e.ProcessedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		e.RowID = derefString(rowID)
		if payload != nil {
			e.Payload = json.RawMessage(payload)
		}
		events = append(events, &e)
	}
	if events == nil {
		events = []*DBTriggerQueueEvent{}
	}
	return events, rows.Err()
}

// MarkCompleted marks an event as successfully processed.
func (s *DBTriggerEventPostgresStore) MarkCompleted(ctx context.Context, eventID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_edge_trigger_events SET status = 'completed', processed_at = NOW()
		 WHERE id = $1`, eventID,
	)
	if err != nil {
		return fmt.Errorf("marking event completed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("event %s not found", eventID)
	}
	return nil
}

// MarkFailed marks an event as failed, making it eligible for retry by
// ClaimPendingEvents (which filters by attempts < MaxDBTriggerRetries).
func (s *DBTriggerEventPostgresStore) MarkFailed(ctx context.Context, eventID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_edge_trigger_events SET
			status = 'failed',
			processed_at = NOW()
		 WHERE id = $1`, eventID,
	)
	if err != nil {
		return fmt.Errorf("marking event failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("event %s not found", eventID)
	}
	return nil
}
