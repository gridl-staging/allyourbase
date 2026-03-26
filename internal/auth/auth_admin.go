package auth

import (
	"context"
	"fmt"
	"time"
)

// AdminUser is a user record with additional fields visible only to admins.
type AdminUser struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// UserListResult is a paginated list of admin users.
type UserListResult struct {
	Items      []AdminUser `json:"items"`
	Page       int         `json:"page"`
	PerPage    int         `json:"perPage"`
	TotalItems int         `json:"totalItems"`
	TotalPages int         `json:"totalPages"`
}

// ListUsers returns a paginated list of users (admin-only).
func (s *Service) ListUsers(ctx context.Context, page, perPage int, search string) (*UserListResult, error) {
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
	var rows []AdminUser

	if search != "" {
		pattern := "%" + search + "%"
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM _ayb_users WHERE email ILIKE $1`, pattern,
		).Scan(&totalItems)
		if err != nil {
			return nil, fmt.Errorf("counting users: %w", err)
		}

		dbRows, err := s.pool.Query(ctx,
			`SELECT id, COALESCE(email, ''), email_verified, created_at, updated_at
			 FROM _ayb_users WHERE email ILIKE $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			pattern, perPage, offset,
		)
		if err != nil {
			return nil, fmt.Errorf("querying users: %w", err)
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var u AdminUser
			if err := dbRows.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scanning user: %w", err)
			}
			rows = append(rows, u)
		}
		if err := dbRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating users: %w", err)
		}
	} else {
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM _ayb_users`,
		).Scan(&totalItems)
		if err != nil {
			return nil, fmt.Errorf("counting users: %w", err)
		}

		dbRows, err := s.pool.Query(ctx,
			`SELECT id, COALESCE(email, ''), email_verified, created_at, updated_at
			 FROM _ayb_users
			 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			perPage, offset,
		)
		if err != nil {
			return nil, fmt.Errorf("querying users: %w", err)
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var u AdminUser
			if err := dbRows.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scanning user: %w", err)
			}
			rows = append(rows, u)
		}
		if err := dbRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating users: %w", err)
		}
	}

	if rows == nil {
		rows = []AdminUser{}
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &UserListResult{
		Items:      rows,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// DeleteUser removes a user by ID, including all their sessions, apps, and
// app-scoped API keys. The _ayb_apps FK uses ON DELETE CASCADE from the user,
// but _ayb_api_keys.app_id uses ON DELETE RESTRICT to prevent silent privilege
// escalation. We must detach keys from the user's apps before the cascade can
// proceed.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Revoke active app-scoped keys and detach all keys from the user's apps.
	// This satisfies the ON DELETE RESTRICT FK on api_keys.app_id so that the
	// subsequent CASCADE delete of _ayb_apps rows can succeed.
	_, err = tx.Exec(ctx,
		`UPDATE _ayb_api_keys
		 SET revoked_at = COALESCE(revoked_at, NOW()), app_id = NULL
		 WHERE app_id IN (SELECT id FROM _ayb_apps WHERE owner_user_id = $1)`, id)
	if err != nil {
		return fmt.Errorf("detaching app keys before user delete: %w", err)
	}

	result, err := tx.Exec(ctx,
		`DELETE FROM _ayb_users WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing user delete: %w", err)
	}

	s.logger.Info("user deleted by admin", "user_id", id)
	return nil
}
