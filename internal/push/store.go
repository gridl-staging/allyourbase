// Package push Store persists push tokens and delivery records in Postgres, providing methods to manage device tokens and audit push notification deliveries.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists push tokens and delivery audit rows in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a pgx-backed push store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const deviceTokenColumns = `id, app_id, user_id, provider, platform, token, device_name,
	is_active, last_used, last_refreshed_at, created_at, updated_at`

const deliveryColumns = `id, device_token_id, job_id, app_id, user_id, provider, title, body,
	data_payload, status, error_code, error_message, provider_message_id, sent_at, created_at, updated_at`

// RegisterToken inserts or updates a device token for the user. If a token with the same app_id, provider, and token already exists, it updates the user_id, platform, device_name, and refresh timestamp instead. Returns the registered or updated token.
func (s *Store) RegisterToken(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*DeviceToken, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	platform = strings.ToLower(strings.TrimSpace(platform))

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_device_tokens (app_id, user_id, provider, platform, token, device_name, is_active, last_refreshed_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), true, NOW(), NOW())
		 ON CONFLICT (app_id, provider, token) DO UPDATE
		   SET user_id = EXCLUDED.user_id,
		       platform = EXCLUDED.platform,
		       device_name = EXCLUDED.device_name,
		       is_active = true,
		       last_refreshed_at = NOW(),
		       updated_at = NOW()
		 RETURNING `+deviceTokenColumns,
		appID, userID, provider, platform, token, strings.TrimSpace(deviceName),
	)

	dt, err := scanDeviceToken(row)
	if err != nil {
		return nil, fmt.Errorf("register token: %w", err)
	}
	return dt, nil
}

func (s *Store) GetToken(ctx context.Context, id string) (*DeviceToken, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+deviceTokenColumns+` FROM _ayb_device_tokens WHERE id = $1`, id,
	)
	dt, err := scanDeviceToken(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	return dt, nil
}

// ListUserTokens returns active device tokens for the specified user in the given app, ordered by creation time descending.
func (s *Store) ListUserTokens(ctx context.Context, appID, userID string) ([]*DeviceToken, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+deviceTokenColumns+`
		 FROM _ayb_device_tokens
		 WHERE app_id = $1 AND user_id = $2 AND is_active = true
		 ORDER BY created_at DESC`,
		appID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user tokens: %w", err)
	}
	defer rows.Close()

	items, err := scanDeviceTokens(rows)
	if err != nil {
		return nil, fmt.Errorf("list user tokens: %w", err)
	}
	return items, nil
}

// ListTokens returns device tokens matching the optional appID and userID filters. If includeInactive is false, only active tokens are returned. Results are ordered by creation time descending.
func (s *Store) ListTokens(ctx context.Context, appID, userID string, includeInactive bool) ([]*DeviceToken, error) {
	query := `SELECT ` + deviceTokenColumns + ` FROM _ayb_device_tokens WHERE 1=1`
	args := make([]any, 0, 3)
	argN := 1

	if appID != "" {
		query += fmt.Sprintf(" AND app_id = $%d", argN)
		args = append(args, appID)
		argN++
	}
	if userID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argN)
		args = append(args, userID)
		argN++
	}
	if !includeInactive {
		query += " AND is_active = true"
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()

	items, err := scanDeviceTokens(rows)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return items, nil
}

func (s *Store) RevokeToken(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_device_tokens SET is_active = false, updated_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeAllUserTokens(ctx context.Context, appID, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_device_tokens
		 SET is_active = false, updated_at = NOW()
		 WHERE app_id = $1 AND user_id = $2 AND is_active = true`,
		appID, userID,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke all user tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) DeleteToken(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_device_tokens WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_device_tokens SET last_used = NOW(), updated_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("update last used: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkInactive(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_device_tokens SET is_active = false, updated_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark inactive: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CleanupStaleTokens marks active device tokens as inactive if they have not been refreshed within the specified number of days. If staleDays is zero or negative, it defaults to 270 days. Returns the count of tokens marked as inactive.
func (s *Store) CleanupStaleTokens(ctx context.Context, staleDays int) (int64, error) {
	if staleDays <= 0 {
		staleDays = 270
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_device_tokens
		 SET is_active = false, updated_at = NOW()
		 WHERE is_active = true
		   AND last_refreshed_at < NOW() - make_interval(days => $1)`,
		staleDays,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup stale tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RecordDelivery inserts a new push delivery record. If no status is provided, it defaults to pending. The data payload is marshaled to JSON for storage. Returns the created delivery record with all fields populated.
func (s *Store) RecordDelivery(ctx context.Context, delivery *PushDelivery) (*PushDelivery, error) {
	if delivery == nil {
		return nil, fmt.Errorf("delivery is required")
	}
	status := delivery.Status
	if status == "" {
		status = DeliveryStatusPending
	}

	var dataPayload any
	if len(delivery.DataPayload) > 0 {
		raw, err := json.Marshal(delivery.DataPayload)
		if err != nil {
			return nil, fmt.Errorf("marshal data_payload: %w", err)
		}
		dataPayload = raw
	}

	row := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_push_deliveries
		 (device_token_id, job_id, app_id, user_id, provider, title, body, data_payload, status, error_code, error_message, provider_message_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''))
		 RETURNING `+deliveryColumns,
		delivery.DeviceTokenID,
		delivery.JobID,
		delivery.AppID,
		delivery.UserID,
		delivery.Provider,
		delivery.Title,
		delivery.Body,
		dataPayload,
		status,
		stringOrEmpty(delivery.ErrorCode),
		stringOrEmpty(delivery.ErrorMessage),
		stringOrEmpty(delivery.ProviderMessageID),
	)

	item, err := scanDelivery(row)
	if err != nil {
		return nil, fmt.Errorf("record delivery: %w", err)
	}
	return item, nil
}

func (s *Store) SetDeliveryJobID(ctx context.Context, deliveryID, jobID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_push_deliveries SET job_id = $2, updated_at = NOW() WHERE id = $1`,
		deliveryID, jobID,
	)
	if err != nil {
		return fmt.Errorf("set delivery job id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateDeliveryStatus updates a delivery record's status and optional error information. If status is 'sent', the sent_at timestamp is set to the current time.
func (s *Store) UpdateDeliveryStatus(ctx context.Context, id, status, errorCode, errorMsg, messageID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_push_deliveries
		 SET status = $2,
		     error_code = NULLIF($3, ''),
		     error_message = NULLIF($4, ''),
		     provider_message_id = NULLIF($5, ''),
		     sent_at = CASE WHEN $2 = 'sent' THEN NOW() ELSE sent_at END,
		     updated_at = NOW()
		 WHERE id = $1`,
		id, status, errorCode, errorMsg, messageID,
	)
	if err != nil {
		return fmt.Errorf("update delivery status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListDeliveries returns push deliveries matching the optional appID, userID, and status filters. Results are ordered by creation time descending and support pagination via limit and offset.
func (s *Store) ListDeliveries(ctx context.Context, appID, userID, status string, limit, offset int) ([]*PushDelivery, error) {
	query := `SELECT ` + deliveryColumns + ` FROM _ayb_push_deliveries WHERE 1=1`
	args := make([]any, 0, 5)
	argN := 1

	if appID != "" {
		query += fmt.Sprintf(" AND app_id = $%d", argN)
		args = append(args, appID)
		argN++
	}
	if userID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argN)
		args = append(args, userID)
		argN++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, status)
		argN++
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, limit)
		argN++
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argN)
		args = append(args, offset)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	items, err := scanDeliveries(rows)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	return items, nil
}

func (s *Store) GetDelivery(ctx context.Context, id string) (*PushDelivery, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+deliveryColumns+` FROM _ayb_push_deliveries WHERE id = $1`,
		id,
	)
	item, err := scanDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery: %w", err)
	}
	return item, nil
}

// scanDeviceToken scans a database row into a DeviceToken struct.
func scanDeviceToken(row pgx.Row) (*DeviceToken, error) {
	item := &DeviceToken{}
	err := row.Scan(
		&item.ID,
		&item.AppID,
		&item.UserID,
		&item.Provider,
		&item.Platform,
		&item.Token,
		&item.DeviceName,
		&item.IsActive,
		&item.LastUsed,
		&item.LastRefreshedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func scanDeviceTokens(rows pgx.Rows) ([]*DeviceToken, error) {
	items := make([]*DeviceToken, 0)
	for rows.Next() {
		item, err := scanDeviceToken(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// scanDelivery scans a database row into a PushDelivery struct, unmarshaling the JSON data payload if present.
func scanDelivery(row pgx.Row) (*PushDelivery, error) {
	item := &PushDelivery{}
	var rawPayload []byte
	err := row.Scan(
		&item.ID,
		&item.DeviceTokenID,
		&item.JobID,
		&item.AppID,
		&item.UserID,
		&item.Provider,
		&item.Title,
		&item.Body,
		&rawPayload,
		&item.Status,
		&item.ErrorCode,
		&item.ErrorMessage,
		&item.ProviderMessageID,
		&item.SentAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(rawPayload) > 0 {
		item.DataPayload = map[string]string{}
		if err := json.Unmarshal(rawPayload, &item.DataPayload); err != nil {
			return nil, fmt.Errorf("unmarshal data_payload: %w", err)
		}
	}
	return item, nil
}

func scanDeliveries(rows pgx.Rows) ([]*PushDelivery, error) {
	items := make([]*PushDelivery, 0)
	for rows.Next() {
		item, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
