// Package auth Provides HTTP handlers for anonymous user account creation, linking credentials (email or OAuth), and unlinking OAuth providers.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DefaultAnonymousTTL is the default retention period for anonymous accounts
// that have not been linked. After this period, stale anonymous users are
// eligible for cleanup.
const DefaultAnonymousTTL = 30 * 24 * time.Hour

// DefaultAnonymousRateLimit is the default anonymous sign-in limit per IP per hour.
const DefaultAnonymousRateLimit = 30

const anonymousRateLimitWindow = time.Hour

// Sentinel errors for anonymous auth.
var (
	ErrNotAnonymous      = errors.New("account is not anonymous")
	ErrAlreadyLinked     = errors.New("account is already linked")
	ErrLinkConflict      = errors.New("email already belongs to another account")
	ErrAnonymousMFABlock = errors.New("link your account before enabling MFA")
)

// SetAnonymousAuthEnabled enables or disables the anonymous auth endpoint.
func (h *Handler) SetAnonymousAuthEnabled(enabled bool) {
	h.anonymousAuthEnabled = enabled
	if enabled && h.anonymousRateLimiter == nil {
		h.SetAnonymousRateLimit(DefaultAnonymousRateLimit)
	}
}

// SetAnonymousRateLimit sets the anonymous sign-in rate limit per hour.
func (h *Handler) SetAnonymousRateLimit(limit int) {
	if limit <= 0 {
		limit = DefaultAnonymousRateLimit
	}
	if h.anonymousRateLimiter != nil {
		h.anonymousRateLimiter.Stop()
	}
	h.anonymousRateLimiter = NewRateLimiter(limit, anonymousRateLimitWindow)
}

func (h *Handler) anonymousRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.anonymousAuthEnabled || h.anonymousRateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}
		h.anonymousRateLimiter.Middleware(next).ServeHTTP(w, r)
	})
}

// CreateAnonymousUser inserts a user with is_anonymous=true, null email/password,
// and returns the user with access + refresh tokens.
func (s *Service) CreateAnonymousUser(ctx context.Context) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	var user User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash, is_anonymous)
		 VALUES (NULL, NULL, true)
		 RETURNING id, COALESCE(email, ''), is_anonymous, created_at, updated_at`,
	).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, "", "", fmt.Errorf("inserting anonymous user: %w", err)
	}

	s.logger.Info("anonymous user created", "user_id", user.ID)
	return s.issueTokens(ctx, &user)
}

// CleanupAnonymousUsers deletes anonymous accounts older than the given TTL
// that have not been linked. Returns the number of deleted users.
// Associated data (refresh tokens, MFA factors, etc.) should be cascade-deleted
// via foreign key constraints in the schema.
func (s *Service) CleanupAnonymousUsers(ctx context.Context, ttl time.Duration) (int64, error) {
	if s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}

	cutoff := time.Now().Add(-ttl)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_users
		 WHERE is_anonymous = true AND linked_at IS NULL AND created_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cleaning up anonymous users: %w", err)
	}

	count := tag.RowsAffected()
	if count > 0 {
		s.logger.Info("cleaned up stale anonymous users", "count", count, "ttl", ttl.String())
	}
	return count, nil
}

// LinkEmail converts an anonymous user to a credentialed user by setting email + password.
// Preserves the user ID. Fails if user is not anonymous or email is taken.
func (s *Service) LinkEmail(ctx context.Context, userID, email, password string) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateAuthEmail(email); err != nil {
		return nil, "", "", err
	}
	if err := validatePassword(password, s.minPwLen); err != nil {
		return nil, "", "", err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, "", "", fmt.Errorf("hashing password: %w", err)
	}

	var user User
	err = s.pool.QueryRow(ctx,
		`UPDATE _ayb_users
		 SET email = $2, password_hash = $3, is_anonymous = false, linked_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND is_anonymous = true
		 RETURNING id, COALESCE(email, ''), is_anonymous, linked_at, created_at, updated_at`,
		userID, email, hash,
	).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.LinkedAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, "", "", ErrLinkConflict
		}
		if errors.Is(err, pgx.ErrNoRows) {
			// No rows matched: user not anonymous, already linked, or does not exist.
			return nil, "", "", ErrNotAnonymous
		}
		return nil, "", "", fmt.Errorf("linking anonymous user email: %w", err)
	}

	s.logger.Info("anonymous user linked email", "user_id", userID, "email", email)

	// Send verification email (best-effort).
	if s.mailer != nil {
		if err := s.SendVerificationEmail(ctx, user.ID, user.Email); err != nil {
			s.logger.Error("failed to send verification email on link", "error", err)
		}
	}

	return s.issueTokens(ctx, &user)
}

// ErrOAuthLinkConflict indicates the OAuth identity already belongs to another user.
var ErrOAuthLinkConflict = errors.New("OAuth identity already belongs to another account")

// LinkOAuth converts an anonymous user to a credentialed user by linking an OAuth identity.
// Preserves the user ID. Fails if user is not anonymous or the OAuth identity is already linked.
// Uses a transaction to ensure the user update and OAuth account insert are atomic.
func (s *Service) LinkOAuth(ctx context.Context, userID, provider string, info *OAuthUserInfo) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if this OAuth identity is already linked to another user.
	var existingUserID string
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM _ayb_oauth_accounts
		 WHERE provider = $1 AND provider_user_id = $2`,
		provider, info.ProviderUserID,
	).Scan(&existingUserID)
	if err == nil {
		if existingUserID == userID {
			return nil, "", "", ErrAlreadyLinked
		}
		return nil, "", "", ErrOAuthLinkConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", "", fmt.Errorf("checking existing OAuth link: %w", err)
	}

	// If the OAuth identity has an email, check for email conflicts.
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if email != "" {
		var conflictID string
		err = tx.QueryRow(ctx,
			`SELECT id FROM _ayb_users WHERE LOWER(email) = $1 AND id != $2`, email, userID,
		).Scan(&conflictID)
		if err == nil {
			return nil, "", "", ErrLinkConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", fmt.Errorf("checking email conflict: %w", err)
		}
	}

	// Flip anonymous flag and set email if provided.
	var user User
	if email != "" {
		err = tx.QueryRow(ctx,
			`UPDATE _ayb_users
			 SET email = $2, is_anonymous = false, linked_at = NOW(), updated_at = NOW()
			 WHERE id = $1 AND is_anonymous = true
			 RETURNING id, COALESCE(email, ''), is_anonymous, linked_at, created_at, updated_at`,
			userID, email,
		).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.LinkedAt, &user.CreatedAt, &user.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx,
			`UPDATE _ayb_users
			 SET is_anonymous = false, linked_at = NOW(), updated_at = NOW()
			 WHERE id = $1 AND is_anonymous = true
			 RETURNING id, COALESCE(email, ''), is_anonymous, linked_at, created_at, updated_at`,
			userID,
		).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.LinkedAt, &user.CreatedAt, &user.UpdatedAt)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, "", "", ErrLinkConflict
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrNotAnonymous
		}
		return nil, "", "", fmt.Errorf("linking anonymous user OAuth: %w", err)
	}

	// Insert the OAuth identity link within the same transaction.
	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name)
		 VALUES ($1, $2, $3, $4, $5)`,
		user.ID, provider, info.ProviderUserID, info.Email, info.Name,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Unique constraint on (provider, provider_user_id) — race with concurrent link.
			return nil, "", "", ErrOAuthLinkConflict
		}
		return nil, "", "", fmt.Errorf("inserting OAuth account link: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", "", fmt.Errorf("committing OAuth link: %w", err)
	}

	s.logger.Info("anonymous user linked OAuth", "user_id", userID, "provider", provider)
	return s.issueTokens(ctx, &user)
}

// --- Handlers ---

type linkEmailRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// is an HTTP handler that creates a new anonymous user and returns access and refresh tokens. It returns HTTP 201 if successful, or 404 if anonymous authentication is disabled, or 500 if user creation fails.
func (h *Handler) handleAnonymousSignIn(w http.ResponseWriter, r *http.Request) {
	if !h.anonymousAuthEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "anonymous auth is not enabled",
			"https://allyourbase.io/guide/authentication#anonymous")
		return
	}

	user, accessToken, refreshToken, err := h.auth.CreateAnonymousUser(r.Context())
	if err != nil {
		h.logger.Error("anonymous sign-in error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// is an HTTP handler that converts an anonymous user to a credentialed user by setting email and password. It requires the request to be authenticated as an anonymous account, validates the provided credentials, and returns updated access and refresh tokens. It returns 409 if the email is already in use, 403 if the account is not anonymous, 400 for invalid credentials, or 500 on internal errors.
func (h *Handler) handleLinkEmail(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if !claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, "only anonymous accounts can link credentials")
		return
	}

	var req linkEmailRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "password is required")
		return
	}

	user, accessToken, refreshToken, err := h.auth.LinkEmail(r.Context(), claims.Subject, req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrLinkConflict):
			httputil.WriteError(w, http.StatusConflict, "email already belongs to another account")
		case errors.Is(err, ErrNotAnonymous):
			httputil.WriteError(w, http.StatusForbidden, "account is not anonymous or already linked")
		case errors.Is(err, ErrValidation):
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteError(w, http.StatusBadRequest, msg)
		default:
			h.logger.Error("link email error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

type linkOAuthRequest struct {
	Provider    string `json:"provider"`
	AccessToken string `json:"access_token"`
}

// is an HTTP handler that links an OAuth provider identity to an anonymous user account. It requires the request to be authenticated as an anonymous account, validates the provider is configured, fetches user information from the OAuth provider using the provided access token, and returns updated access and refresh tokens. It returns 409 if the identity or email is already in use, 403 if the account is not anonymous, 400 for missing or invalid parameters, 502 if fetching OAuth user info fails, or 500 on internal errors.
func (h *Handler) handleLinkOAuth(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if !claims.IsAnonymous {
		httputil.WriteError(w, http.StatusForbidden, "only anonymous accounts can link credentials")
		return
	}

	var req linkOAuthRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.AccessToken == "" {
		httputil.WriteError(w, http.StatusBadRequest, "access_token is required")
		return
	}

	// Validate that the provider is configured on this handler.
	pc, ok := h.getOAuthProviderConfig(req.Provider)
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("OAuth provider %q not configured", req.Provider))
		return
	}

	// Fetch user info from the provider using the access token.
	info, err := fetchUserInfo(r.Context(), req.Provider, pc.UserInfoURL, req.AccessToken, h.oauthHTTPClient)
	if err != nil {
		h.logger.Error("link OAuth: failed to fetch user info", "provider", req.Provider, "error", err)
		httputil.WriteError(w, http.StatusBadGateway, "failed to fetch user info from provider")
		return
	}

	user, accessToken, refreshToken, err := h.auth.LinkOAuth(r.Context(), claims.Subject, req.Provider, info)
	if err != nil {
		switch {
		case errors.Is(err, ErrOAuthLinkConflict):
			httputil.WriteError(w, http.StatusConflict, "OAuth identity already belongs to another account")
		case errors.Is(err, ErrLinkConflict):
			httputil.WriteError(w, http.StatusConflict, "email already belongs to another account")
		case errors.Is(err, ErrNotAnonymous):
			httputil.WriteError(w, http.StatusForbidden, "account is not anonymous or already linked")
		case errors.Is(err, ErrAlreadyLinked):
			httputil.WriteError(w, http.StatusConflict, "OAuth identity already linked to this account")
		default:
			h.logger.Error("link OAuth error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

type unlinkOAuthRequest struct {
	Provider string `json:"provider"`
}

// is an HTTP handler that removes an OAuth provider identity link from a user's account. It requires authentication, validates the provider name, and returns HTTP 204 No Content on success. It returns 401 if not authenticated, 400 for missing provider, 404 if the OAuth link is not found, or 500 on internal errors.
func (h *Handler) handleUnlinkOAuth(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req unlinkOAuthRequest
	if !decodeBody(w, r, &req) {
		return
	}
	req.Provider = strings.TrimSpace(req.Provider)
	if req.Provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}

	if err := h.auth.UnlinkOAuth(r.Context(), claims.Subject, req.Provider); err != nil {
		switch {
		case errors.Is(err, ErrOAuthLinkNotFound):
			httputil.WriteError(w, http.StatusNotFound, "OAuth identity link not found")
		case errors.Is(err, ErrValidation):
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteError(w, http.StatusBadRequest, msg)
		default:
			h.logger.Error("unlink OAuth error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
