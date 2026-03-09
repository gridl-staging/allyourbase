package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// App represents a registered application.
type App struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Description            string    `json:"description"`
	OwnerUserID            string    `json:"ownerUserId"`
	RateLimitRPS           int       `json:"rateLimitRps"`
	RateLimitWindowSeconds int       `json:"rateLimitWindowSeconds"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

// AppListResult is a paginated list of apps.
type AppListResult struct {
	Items      []App `json:"items"`
	Page       int   `json:"page"`
	PerPage    int   `json:"perPage"`
	TotalItems int   `json:"totalItems"`
	TotalPages int   `json:"totalPages"`
}

// ErrAppNotFound is returned when an app doesn't exist.
var ErrAppNotFound = errors.New("app not found")

// ErrAppNameRequired is returned when an app name is empty.
var ErrAppNameRequired = errors.New("app name is required")

// ErrAppOwnerNotFound is returned when the owner user doesn't exist.
var ErrAppOwnerNotFound = errors.New("owner user not found")

// ErrAppInvalidRateLimit is returned when rate limit values are negative.
var ErrAppInvalidRateLimit = errors.New("rate limit values must be non-negative")

// CreateApp creates a new application.
func (s *Service) CreateApp(ctx context.Context, name, description, ownerUserID string) (*App, error) {
	if name == "" {
		return nil, ErrAppNameRequired
	}

	var app App
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_apps (name, description, owner_user_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, name, description, owner_user_id, rate_limit_rps, rate_limit_window_seconds, created_at, updated_at`,
		name, description, ownerUserID,
	).Scan(&app.ID, &app.Name, &app.Description, &app.OwnerUserID,
		&app.RateLimitRPS, &app.RateLimitWindowSeconds, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrAppOwnerNotFound
		}
		return nil, fmt.Errorf("creating app: %w", err)
	}

	s.logger.Info("app created", "app_id", app.ID, "name", name, "owner", ownerUserID)
	return &app, nil
}

// GetApp retrieves an app by ID.
func (s *Service) GetApp(ctx context.Context, id string) (*App, error) {
	var app App
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description, owner_user_id, rate_limit_rps, rate_limit_window_seconds, created_at, updated_at
		 FROM _ayb_apps WHERE id = $1`,
		id,
	).Scan(&app.ID, &app.Name, &app.Description, &app.OwnerUserID,
		&app.RateLimitRPS, &app.RateLimitWindowSeconds, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAppNotFound
		}
		return nil, fmt.Errorf("getting app: %w", err)
	}
	return &app, nil
}

// ListApps returns a paginated list of all apps.
func (s *Service) ListApps(ctx context.Context, page, perPage int) (*AppListResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var totalItems int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_apps`).Scan(&totalItems)
	if err != nil {
		return nil, fmt.Errorf("counting apps: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, owner_user_id, rate_limit_rps, rate_limit_window_seconds, created_at, updated_at
		 FROM _ayb_apps ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing apps: %w", err)
	}
	defer rows.Close()

	var items []App
	for rows.Next() {
		var a App
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.OwnerUserID,
			&a.RateLimitRPS, &a.RateLimitWindowSeconds, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning app: %w", err)
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating apps: %w", err)
	}
	if items == nil {
		items = []App{}
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &AppListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// UpdateApp updates an app's name, description, and rate limits.
func (s *Service) UpdateApp(ctx context.Context, id, name, description string, rateLimitRPS, rateLimitWindowSeconds int) (*App, error) {
	if name == "" {
		return nil, ErrAppNameRequired
	}
	if rateLimitRPS < 0 || rateLimitWindowSeconds < 0 {
		return nil, ErrAppInvalidRateLimit
	}

	var app App
	err := s.pool.QueryRow(ctx,
		`UPDATE _ayb_apps
		 SET name = $2, description = $3, rate_limit_rps = $4, rate_limit_window_seconds = $5, updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, name, description, owner_user_id, rate_limit_rps, rate_limit_window_seconds, created_at, updated_at`,
		id, name, description, rateLimitRPS, rateLimitWindowSeconds,
	).Scan(&app.ID, &app.Name, &app.Description, &app.OwnerUserID,
		&app.RateLimitRPS, &app.RateLimitWindowSeconds, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAppNotFound
		}
		return nil, fmt.Errorf("updating app: %w", err)
	}

	s.logger.Info("app updated", "app_id", id, "name", name)
	return &app, nil
}

// DeleteApp deletes an app by ID. All API keys scoped to this app are revoked
// and detached in a single UPDATE, then the app row is deleted. The FK is
// ON DELETE RESTRICT (migration 018), so the detach step is mandatory.
//
// The transaction serializes with concurrent key creation via PostgreSQL's FK
// share-lock: an INSERT with app_id takes a SHARE lock on the apps row,
// blocking this DELETE until the INSERT commits. If a new key is committed
// first, the detach UPDATE in the next retry will catch it.
func (s *Service) DeleteApp(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Revoke active keys and detach all keys from this app.
	// COALESCE preserves existing revoked_at timestamps on already-revoked keys.
	// Setting app_id = NULL satisfies the ON DELETE RESTRICT FK constraint.
	_, err = tx.Exec(ctx,
		`UPDATE _ayb_api_keys
		 SET revoked_at = COALESCE(revoked_at, NOW()), app_id = NULL
		 WHERE app_id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoking app keys: %w", err)
	}

	result, err := tx.Exec(ctx, `DELETE FROM _ayb_apps WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting app: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAppNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing delete: %w", err)
	}

	s.logger.Info("app deleted", "app_id", id)
	return nil
}
