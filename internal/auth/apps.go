// Package auth Apps provides functions for managing registered applications: creating, retrieving, listing, updating, and deleting app records in the auth system.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	TenantID               *string   `json:"tenantId,omitempty"`
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

func normalizeAppListPagination(page, perPage int) (int, int, int) {
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
	return page, perPage, offset
}

func calculateTotalPages(totalItems, perPage int) int {
	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}
	return totalPages
}

// scanApps scans database rows into App values and returns them as a slice, or an error if scanning fails. An empty slice is returned if no rows are present.
func scanApps(rows pgx.Rows) ([]App, error) {
	var items []App
	for rows.Next() {
		a, scanErr := scanApp(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning app: %w", scanErr)
		}
		items = append(items, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating apps: %w", err)
	}
	if items == nil {
		items = []App{}
	}
	return items, nil
}

// ErrAppNotFound is returned when an app doesn't exist.
var ErrAppNotFound = errors.New("app not found")

// ErrAppNameRequired is returned when an app name is empty.
var ErrAppNameRequired = errors.New("app name is required")

// ErrAppOwnerNotFound is returned when the owner user doesn't exist.
var ErrAppOwnerNotFound = errors.New("owner user not found")

// ErrAppInvalidRateLimit is returned when rate limit values are negative.
var ErrAppInvalidRateLimit = errors.New("rate limit values must be non-negative")

// appColumns is the canonical column list for app SELECT queries.
const appColumns = `id, name, description, owner_user_id, tenant_id, rate_limit_rps, rate_limit_window_seconds, created_at, updated_at`

// scanApp scans a single app row using the canonical appColumns order.
func scanApp(row pgx.Row) (*App, error) {
	var a App
	err := row.Scan(
		&a.ID, &a.Name, &a.Description, &a.OwnerUserID, &a.TenantID,
		&a.RateLimitRPS, &a.RateLimitWindowSeconds, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateApp creates a new application.
func (s *Service) CreateApp(ctx context.Context, name, description, ownerUserID string) (*App, error) {
	if name == "" {
		return nil, ErrAppNameRequired
	}

	a, err := scanApp(s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_apps (name, description, owner_user_id)
		 VALUES ($1, $2, $3)
		 RETURNING `+appColumns,
		name, description, ownerUserID,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrAppOwnerNotFound
		}
		return nil, fmt.Errorf("creating app: %w", err)
	}

	s.logger.Info("app created", "app_id", a.ID, "name", name, "owner", ownerUserID)
	return a, nil
}

// GetApp retrieves an app by ID.
func (s *Service) GetApp(ctx context.Context, id string) (*App, error) {
	a, err := scanApp(s.pool.QueryRow(ctx,
		`SELECT `+appColumns+` FROM _ayb_apps WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAppNotFound
		}
		return nil, fmt.Errorf("getting app: %w", err)
	}
	return a, nil
}

// appTenantID returns the effective tenant identifier for an app.
// When tenant_id is set it returns that value (tenant-native path, legacy=false).
// When tenant_id is NULL it falls back to owner_user_id (legacy=true) so
// unmigrated apps remain resolvable during migration cutover.
func appTenantID(a *App) (id string, legacy bool) {
	if a.TenantID != nil {
		return *a.TenantID, false
	}
	return a.OwnerUserID, true
}

// ResolveAppTenant returns the tenant ID for an app, using the tenant-native
// path when tenant_id is set, and the legacy owner_user_id path otherwise.
// A warn log is emitted when the fallback is used so unmigrated traffic is
// measurable during cutover.
func (s *Service) ResolveAppTenant(ctx context.Context, appID string) (tenantID string, legacy bool, err error) {
	a, err := s.GetApp(ctx, appID)
	if err != nil {
		return "", false, err
	}
	id, isLegacy := appTenantID(a)
	if isLegacy {
		s.logger.Warn("app tenant fallback: using owner_user_id (not yet migrated)",
			slog.String("app_id", appID),
			slog.String("owner_user_id", a.OwnerUserID),
		)
	}
	return id, isLegacy, nil
}

// listApps returns a paginated list of apps, optionally filtered by tenant ID when tenantID is non-nil. Pagination parameters are normalized before execution.
func (s *Service) listApps(ctx context.Context, tenantID *string, page, perPage int) (*AppListResult, error) {
	page, perPage, offset := normalizeAppListPagination(page, perPage)

	countSQL := `SELECT COUNT(*) FROM _ayb_apps`
	listSQL := `SELECT ` + appColumns + ` FROM _ayb_apps ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	countArgs := []any{}
	listArgs := []any{perPage, offset}
	if tenantID != nil {
		countSQL += ` WHERE tenant_id = $1`
		listSQL = `SELECT ` + appColumns + ` FROM _ayb_apps WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		countArgs = []any{*tenantID}
		listArgs = []any{*tenantID, perPage, offset}
	}

	var totalItems int
	if err := s.pool.QueryRow(ctx, countSQL, countArgs...).Scan(&totalItems); err != nil {
		return nil, fmt.Errorf("counting apps: %w", err)
	}

	rows, err := s.pool.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, fmt.Errorf("listing apps: %w", err)
	}
	defer rows.Close()

	items, err := scanApps(rows)
	if err != nil {
		return nil, err
	}

	return &AppListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: calculateTotalPages(totalItems, perPage),
	}, nil
}

// ListApps returns a paginated list of all apps.
func (s *Service) ListApps(ctx context.Context, page, perPage int) (*AppListResult, error) {
	return s.listApps(ctx, nil, page, perPage)
}

// ListAppsByTenant returns a paginated list of apps belonging to tenantID.
func (s *Service) ListAppsByTenant(ctx context.Context, tenantID string, page, perPage int) (*AppListResult, error) {
	return s.listApps(ctx, &tenantID, page, perPage)
}

// UpdateApp updates an app's name, description, and rate limits.
func (s *Service) UpdateApp(ctx context.Context, id, name, description string, rateLimitRPS, rateLimitWindowSeconds int) (*App, error) {
	if name == "" {
		return nil, ErrAppNameRequired
	}
	if rateLimitRPS < 0 || rateLimitWindowSeconds < 0 {
		return nil, ErrAppInvalidRateLimit
	}

	a, err := scanApp(s.pool.QueryRow(ctx,
		`UPDATE _ayb_apps
		 SET name = $2, description = $3, rate_limit_rps = $4, rate_limit_window_seconds = $5, updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+appColumns,
		id, name, description, rateLimitRPS, rateLimitWindowSeconds,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAppNotFound
		}
		return nil, fmt.Errorf("updating app: %w", err)
	}

	s.logger.Info("app updated", "app_id", id, "name", name)
	return a, nil
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
