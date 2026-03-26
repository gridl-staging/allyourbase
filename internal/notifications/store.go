package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists notifications in Postgres.
type Store struct {
	db notificationDB
}

type notificationDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// NewStore creates a pgx-backed notification store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{db: pool}
}

const notifColumns = `id, user_id, title, body, metadata, channel, read_at, created_at`

// Create inserts a new notification and returns the full row.
func (s *Store) Create(ctx context.Context, userID, title, body string, metadata map[string]any, channel string) (*Notification, error) {
	if channel == "" {
		channel = "general"
	}
	if metadata == nil {
		metadata = map[string]any{}
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	row := s.db.QueryRow(ctx,
		`INSERT INTO _ayb_notifications (user_id, title, body, metadata, channel)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+notifColumns,
		userID, title, body, metaJSON, channel,
	)
	return scanNotification(row)
}

// ListByUser returns paginated notifications for a user, newest first.
func (s *Store) ListByUser(ctx context.Context, userID string, unreadOnly bool, page, perPage int) ([]*Notification, int, error) {
	offset := (page - 1) * perPage

	// Build WHERE clause.
	where := `WHERE user_id = $1`
	args := []any{userID}
	if unreadOnly {
		where += ` AND read_at IS NULL`
	}

	// Count query.
	var total int
	countSQL := `SELECT COUNT(*) FROM _ayb_notifications ` + where
	if err := s.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	// Data query.
	dataSQL := fmt.Sprintf(
		`SELECT %s FROM _ayb_notifications %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		notifColumns, where, len(args)+1, len(args)+2,
	)
	args = append(args, perPage, offset)

	rows, err := s.db.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var items []*Notification
	for rows.Next() {
		n, err := scanNotificationRows(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notifications: %w", err)
	}

	return items, total, nil
}

// GetByID returns a single notification by ID, scoped to the user.
func (s *Store) GetByID(ctx context.Context, id, userID string) (*Notification, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+notifColumns+` FROM _ayb_notifications WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	n, err := scanNotification(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return n, nil
}

// MarkRead sets read_at on a single unread notification.
// Returns ErrNotFound if the notification doesn't exist or is already read.
func (s *Store) MarkRead(ctx context.Context, id, userID string) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE _ayb_notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead sets read_at on all unread notifications for a user.
func (s *Store) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	tag, err := s.db.Exec(ctx,
		`UPDATE _ayb_notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return tag.RowsAffected(), nil
}

// scanNotification scans a single row into a Notification.
func scanNotification(row pgx.Row) (*Notification, error) {
	var n Notification
	var metaJSON []byte
	var readAt *time.Time
	err := row.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &metaJSON, &n.Channel, &readAt, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	n.ReadAt = readAt
	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &n.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &n, nil
}

// scanNotificationRows scans from a pgx.Rows iterator.
func scanNotificationRows(rows pgx.Rows) (*Notification, error) {
	var n Notification
	var metaJSON []byte
	var readAt *time.Time
	err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &metaJSON, &n.Channel, &readAt, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	n.ReadAt = readAt
	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &n.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &n, nil
}
