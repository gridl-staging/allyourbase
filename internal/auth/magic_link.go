package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrInvalidMagicLinkToken is returned when a magic link token is invalid or expired.
var ErrInvalidMagicLinkToken = errors.New("invalid or expired magic link token")

const (
	magicLinkTokenBytes = 32
	magicLinkDefaultDur = 10 * time.Minute
)

// SetMagicLinkDuration sets the magic link token validity duration.
func (s *Service) SetMagicLinkDuration(d time.Duration) {
	s.magicLinkDur = d
}

// MagicLinkDuration returns the configured magic link duration (or default).
func (s *Service) MagicLinkDuration() time.Duration {
	if s.magicLinkDur > 0 {
		return s.magicLinkDur
	}
	return magicLinkDefaultDur
}

// RequestMagicLink generates a magic link token and emails it.
// Always returns nil to prevent email enumeration.
func (s *Service) RequestMagicLink(ctx context.Context, email string) error {
	if s.mailer == nil {
		return nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateEmail(email); err != nil {
		return nil // don't leak validation errors
	}

	// Delete any existing magic link tokens for this email.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_magic_links WHERE email = $1`, email)

	// Generate token.
	raw := make([]byte, magicLinkTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generating magic link token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)

	dur := s.MagicLinkDuration()

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(dur),
	)
	if err != nil {
		return fmt.Errorf("inserting magic link token: %w", err)
	}

	actionURL := s.baseURL + "/auth/magic-link/confirm?token=" + plaintext
	vars := map[string]string{"AppName": s.appName, "ActionURL": actionURL}
	subject, html, text, err := s.renderAuthEmail(ctx, "auth.magic_link", vars)
	if err != nil {
		return fmt.Errorf("rendering magic link email: %w", err)
	}

	if err := s.mailer.Send(ctx, &mailer.Message{
		To:      email,
		Subject: subject,
		HTML:    html,
		Text:    text,
	}); err != nil {
		s.logger.Error("failed to send magic link email", "error", err, "email", email)
	}
	return nil
}

// ConfirmMagicLink validates a magic link token, finds or creates the user,
// and returns the user with access + refresh tokens.
func (s *Service) ConfirmMagicLink(ctx context.Context, token string) (*User, string, string, error) {
	hash := hashToken(token)

	// Atomically consume the token: DELETE ... RETURNING prevents double-use races
	// where two concurrent requests could both SELECT the same valid token.
	var email string
	err := s.pool.QueryRow(ctx,
		`DELETE FROM _ayb_magic_links
		 WHERE token_hash = $1 AND expires_at > NOW()
		 RETURNING email`,
		hash,
	).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrInvalidMagicLinkToken
		}
		return nil, "", "", fmt.Errorf("consuming magic link token: %w", err)
	}

	// Find existing user by email.
	var user User
	err = s.pool.QueryRow(ctx,
		`SELECT id, email, created_at, updated_at FROM _ayb_users WHERE LOWER(email) = $1`,
		strings.ToLower(email),
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		// Create new user with random password (same pattern as OAuth).
		randomPW := make([]byte, 32)
		if _, err := rand.Read(randomPW); err != nil {
			return nil, "", "", fmt.Errorf("generating random password: %w", err)
		}
		pwHash, err := hashPassword(base64.RawURLEncoding.EncodeToString(randomPW))
		if err != nil {
			return nil, "", "", fmt.Errorf("hashing placeholder password: %w", err)
		}

		err = s.pool.QueryRow(ctx,
			`INSERT INTO _ayb_users (email, password_hash, email_verified)
			 VALUES ($1, $2, true)
			 RETURNING id, email, created_at, updated_at`,
			email, pwHash,
		).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			// Handle race: another request might have created this user.
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				err2 := s.pool.QueryRow(ctx,
					`SELECT id, email, created_at, updated_at FROM _ayb_users WHERE LOWER(email) = $1`,
					strings.ToLower(email),
				).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
				if err2 != nil {
					return nil, "", "", fmt.Errorf("querying user after conflict: %w", err2)
				}
			} else {
				return nil, "", "", fmt.Errorf("inserting user: %w", err)
			}
		} else {
			s.logger.Info("user registered via magic link", "user_id", user.ID, "email", email)
		}
	} else if err != nil {
		return nil, "", "", fmt.Errorf("querying user: %w", err)
	}

	// Mark email as verified (they proved they own it by clicking the link).
	_, _ = s.pool.Exec(ctx,
		`UPDATE _ayb_users SET email_verified = true, updated_at = NOW()
		 WHERE id = $1 AND NOT email_verified`,
		user.ID,
	)

	// If user has MFA enrolled, return a pending token instead of full tokens.
	hasMFA, err := s.HasSMSMFA(ctx, user.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	if hasMFA {
		pendingToken, err := s.generateMFAPendingToken(&user)
		if err != nil {
			return nil, "", "", fmt.Errorf("generating MFA pending token: %w", err)
		}
		return &user, pendingToken, "", nil
	}

	return s.issueTokens(ctx, &user)
}
