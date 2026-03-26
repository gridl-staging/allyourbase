package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/argon2"

	"crypto/rand"
	"encoding/base64"
)

// Register creates a new user and returns the user, an access token, and a refresh token.
func (s *Service) Register(ctx context.Context, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	ctx, span := otel.Tracer("ayb/auth").Start(ctx, "auth.register",
		trace.WithAttributes(semconv.EnduserID(email)),
	)
	defer span.End()
	if err := validateAuthEmail(email); err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", err
	}
	if err := validatePassword(password, s.minPwLen); err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", err
	}

	hash, err := hashPassword(password)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("hashing password: %w", err)
	}

	if s.hookDispatcher != nil {
		_, ipAddress := requestMetadataFromContext(ctx)
		if err := s.hookDispatcher.BeforeSignUp(ctx, email, nil, ipAddress); err != nil {
			if errors.Is(err, ErrHookRejected) {
				observability.RecordSpanError(span, ErrHookRejected)
				return nil, "", "", ErrHookRejected
			}
			observability.RecordSpanError(span, err)
			return nil, "", "", fmt.Errorf("before_sign_up hook failed: %w", err)
		}
	}

	var user User
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, is_anonymous, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			observability.RecordSpanError(span, ErrEmailTaken)
			return nil, "", "", ErrEmailTaken
		}
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("inserting user: %w", err)
	}

	s.logger.Info("user registered", "user_id", user.ID, "email", user.Email)

	// Send verification email (best-effort, don't block registration).
	if err := s.SendVerificationEmail(ctx, user.ID, user.Email); err != nil {
		s.logger.Error("failed to send verification email on register", "error", err)
	}

	userOut, accessToken, refreshToken, err := s.issueTokens(ctx, &user)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", err
	}
	if s.hookDispatcher != nil {
		s.hookDispatcher.AfterSignUp(ctx, user.ID, user.Email, nil)
	}
	return userOut, accessToken, refreshToken, nil
}

// CreateUser creates a user without issuing tokens.
// Used by CLI commands that need to bootstrap users before the server starts.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, email, password string, minPasswordLength int) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateAuthEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword(password, minPasswordLength); err != nil {
		return nil, err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	var user User
	err = pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, is_anonymous, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	return &user, nil
}

func validateAuthEmail(email string) error {
	if err := httputil.ValidateEmailLoose(email); err != nil {
		return fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	return nil
}

func validatePassword(password string, minLen int) error {
	if len(password) == 0 {
		return fmt.Errorf("%w: password is required", ErrValidation)
	}
	if minLen < 1 {
		minLen = 8
	}
	if len(password) < minLen {
		return fmt.Errorf("%w: password must be at least %d characters", ErrValidation, minLen)
	}
	return nil
}

// hashPassword hashes a password using argon2id and returns a PHC-format string.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}
