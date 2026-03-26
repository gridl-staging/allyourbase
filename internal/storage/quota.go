package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrQuotaExceeded is returned when a user's storage quota would be exceeded.
var ErrQuotaExceeded = errors.New("storage quota exceeded")

// ErrQuotaUserNotFound is returned when a quota operation targets a missing user.
var ErrQuotaUserNotFound = errors.New("quota user not found")

// QuotaInfo holds a user's storage usage and quota information.
type QuotaInfo struct {
	BytesUsed  int64 `json:"usage_bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
	QuotaMB    *int  `json:"quota_mb"` // nil = using system default
}

// CheckQuota verifies that adding additionalBytes would not exceed the user's quota.
// Returns ErrQuotaExceeded if the upload would push them over.
func (s *Service) CheckQuota(ctx context.Context, userID string, additionalBytes int64) error {
	if s.pool == nil || userID == "" {
		return nil
	}

	var bytesUsed int64
	var quotaMB *int

	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(su.bytes_used, 0), u.storage_quota_mb
		 FROM _ayb_users u
		 LEFT JOIN _ayb_storage_usage su ON su.user_id = u.id
		 WHERE u.id = $1`, userID,
	).Scan(&bytesUsed, &quotaMB)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // user not found — skip quota enforcement
		}
		return fmt.Errorf("checking quota: %w", err)
	}

	quotaBytes := s.defaultQuotaBytes
	if quotaMB != nil {
		quotaBytes = int64(*quotaMB) * 1024 * 1024
	}

	if quotaBytes <= 0 {
		return nil // unlimited
	}

	if bytesUsed+additionalBytes > quotaBytes {
		return ErrQuotaExceeded
	}
	return nil
}

// IncrementUsage atomically adds bytes to a user's storage usage counter.
// Creates the usage row if it doesn't exist.
func (s *Service) IncrementUsage(ctx context.Context, userID string, bytes int64) error {
	if s.pool == nil || userID == "" || bytes <= 0 {
		return nil
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_storage_usage (user_id, bytes_used, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (user_id) DO UPDATE
		 SET bytes_used = _ayb_storage_usage.bytes_used + $2, updated_at = NOW()`,
		userID, bytes,
	)
	if err != nil {
		return fmt.Errorf("incrementing storage usage: %w", err)
	}
	return nil
}

// DecrementUsage atomically subtracts bytes from a user's storage usage counter.
// Uses GREATEST to prevent negative values.
func (s *Service) DecrementUsage(ctx context.Context, userID string, bytes int64) error {
	if s.pool == nil || userID == "" || bytes <= 0 {
		return nil
	}

	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_storage_usage
		 SET bytes_used = GREATEST(bytes_used - $2, 0), updated_at = NOW()
		 WHERE user_id = $1`,
		userID, bytes,
	)
	if err != nil {
		return fmt.Errorf("decrementing storage usage: %w", err)
	}
	return nil
}

// GetUsage returns storage usage and quota information for a user.
func (s *Service) GetUsage(ctx context.Context, userID string) (*QuotaInfo, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}

	var bytesUsed int64
	var quotaMB *int

	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(su.bytes_used, 0), u.storage_quota_mb
		 FROM _ayb_users u
		 LEFT JOIN _ayb_storage_usage su ON su.user_id = u.id
		 WHERE u.id = $1`, userID,
	).Scan(&bytesUsed, &quotaMB)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrQuotaUserNotFound, userID)
		}
		return nil, fmt.Errorf("getting usage: %w", err)
	}

	quotaBytes := s.defaultQuotaBytes
	if quotaMB != nil {
		quotaBytes = int64(*quotaMB) * 1024 * 1024
	}

	return &QuotaInfo{
		BytesUsed:  bytesUsed,
		QuotaBytes: quotaBytes,
		QuotaMB:    quotaMB,
	}, nil
}

// SetUserQuota sets a per-user quota override. Pass nil to remove the override
// (revert to system default).
func (s *Service) SetUserQuota(ctx context.Context, userID string, quotaMB *int) error {
	if s.pool == nil {
		return fmt.Errorf("database pool is not configured")
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_users SET storage_quota_mb = $1 WHERE id = $2`,
		quotaMB, userID,
	)
	if err != nil {
		return fmt.Errorf("setting user quota: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrQuotaUserNotFound, userID)
	}
	return nil
}
