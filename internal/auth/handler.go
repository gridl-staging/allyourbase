// Package auth Handler implements HTTP endpoint handlers for authentication operations including credential-based login, passwordless flows, OAuth/SAML integrations, session management, and MFA enrollment.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// OAuthPublisher is the interface the auth handler uses to publish OAuth results
// to SSE clients. Implemented by realtime.Hub.
type OAuthPublisher interface {
	HasClient(id string) bool
	PublishOAuth(clientID string, event *OAuthEvent)
}

// OAuthEvent carries the result of an OAuth login back to the SSE client.
type OAuthEvent struct {
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	User         any    `json:"user,omitempty"`
	Error        string `json:"error,omitempty"`
	MFAPending   bool   `json:"mfa_pending,omitempty"`
	MFAToken     string `json:"mfa_token,omitempty"`
}

// AuthMetricsRecorder captures auth event metrics for successful auth actions.
type AuthMetricsRecorder interface {
	RecordAuthSignup(ctx context.Context)
	RecordAuthLogin(ctx context.Context)
}

// Handler serves auth HTTP endpoints.
// Handler serves HTTP endpoints for user authentication and authorization including registration, login, session management, password reset, email verification, OAuth/SAML flows, and multi-factor authentication.
type Handler struct {
	auth                     *Service
	samlSvc                  *SAMLService
	oauthAuthorize           oauthAuthorizationProvider
	oauthToken               oauthTokenProvider
	oauthRevoke              oauthRevokeProvider
	logger                   *slog.Logger
	oauthConfigMu            sync.RWMutex
	oauthClients             map[string]OAuthClientConfig
	oauthProviderURLs        map[string]OAuthProviderConfig // per-handler provider URL overrides
	oauthStoreProviderTokens map[string]bool
	oauthHTTPClient          *http.Client
	oauthStateStore          *OAuthStateStore
	oauthRedirectURL         string
	oauthPublisher           OAuthPublisher                                                                                 // nil when realtime hub not available
	oauthLoginFn             func(ctx context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error) // test-only override
	magicLinkEnabled         bool
	smsEnabled               bool
	anonymousAuthEnabled     bool
	anonymousRateLimiter     *RateLimiter
	totpEnabled              bool
	emailMFAEnabled          bool
	existingMFAOverride      *bool // test-only: override HasAnyMFA check for AAL2 guard
	metricsRecorder          AuthMetricsRecorder
}

// NewHandler creates a new auth handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	// Initialise per-handler provider URL config from the current global state
	// so that test overrides applied via SetProviderURLs before construction
	// are picked up, while still allowing per-handler overrides afterwards.
	oauthMu.RLock()
	urls := make(map[string]OAuthProviderConfig, len(oauthProviders))
	for k, v := range oauthProviders {
		urls[k] = v
	}
	oauthMu.RUnlock()
	return &Handler{
		auth:                     svc,
		oauthAuthorize:           svc,
		oauthToken:               svc,
		oauthRevoke:              svc,
		logger:                   logger,
		oauthClients:             make(map[string]OAuthClientConfig),
		oauthProviderURLs:        urls,
		oauthStoreProviderTokens: make(map[string]bool),
		oauthHTTPClient:          oauthHTTPClient,
		oauthStateStore:          NewOAuthStateStore(10 * time.Minute),
	}
}

// Routes returns a chi.Router with auth endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.attachRequestMetadata)
	r.Post("/register", h.handleRegister)
	r.Post("/login", h.handleLogin)
	r.With(h.anonymousRateLimitMiddleware).Post("/anonymous", h.handleAnonymousSignIn)
	r.With(RequireAuth(h.auth)).Post("/link/email", h.handleLinkEmail)
	r.With(RequireAuth(h.auth)).Post("/link/oauth", h.handleLinkOAuth)
	r.With(RequireAuth(h.auth)).Delete("/link/oauth", h.handleUnlinkOAuth)
	r.Post("/refresh", h.handleRefresh)
	r.Post("/logout", h.handleLogout)
	r.With(RequireAuth(h.auth)).Get("/me", h.handleMe)
	r.With(RequireAuth(h.auth)).Get("/sessions", h.handleListSessions)
	r.With(RequireAuth(h.auth)).Delete("/sessions", h.handleDeleteSessions)
	r.With(RequireAuth(h.auth)).Delete("/sessions/{id}", h.handleDeleteSession)
	r.With(RequireAuth(h.auth)).Delete("/me", h.handleDeleteMe)
	r.Post("/password-reset", h.handlePasswordReset)
	r.Post("/password-reset/confirm", h.handlePasswordResetConfirm)
	r.Post("/verify", h.handleVerifyEmail)
	r.With(RequireAuth(h.auth)).Post("/verify/resend", h.handleResendVerification)
	r.Post("/magic-link", h.handleMagicLinkRequest)
	r.Post("/magic-link/confirm", h.handleMagicLinkConfirm)
	r.Get("/saml/{provider}/login", h.handleSAMLLogin)
	r.Post("/saml/{provider}/acs", h.handleSAMLACS)
	r.Get("/saml/{provider}/metadata", h.handleSAMLMetadata)
	r.Get("/oauth/{provider}", h.handleOAuthRedirect)
	r.Get("/oauth/{provider}/callback", h.handleOAuthCallback)
	r.Post("/oauth/{provider}/callback", h.handleOAuthCallback) // Apple form_post
	r.Post("/token", h.handleOAuthToken)
	r.Post("/revoke", h.handleOAuthRevoke)
	r.With(RequireAuth(h.auth)).Get("/authorize", h.handleOAuthAuthorize)
	r.With(RequireAuth(h.auth)).Post("/authorize/consent", h.handleOAuthConsent)
	r.Post("/sms", h.handleSMSRequest)
	r.Post("/sms/confirm", h.handleSMSConfirm)

	// MFA endpoints — gated behind smsEnabled check before auth middleware.
	r.Route("/mfa/sms", func(mfa chi.Router) {
		mfa.Use(h.requireSMSEnabled)
		mfa.With(RequireAuth(h.auth)).Post("/enroll", h.handleMFAEnroll)
		mfa.With(RequireAuth(h.auth)).Post("/enroll/confirm", h.handleMFAEnrollConfirm)
		mfa.With(RequireMFAPending(h.auth)).Post("/challenge", h.handleMFAChallenge)
		mfa.With(RequireMFAPending(h.auth)).Post("/verify", h.handleMFAVerify)
	})

	// TOTP MFA endpoints.
	r.Route("/mfa/totp", func(mfa chi.Router) {
		mfa.Use(h.requireTOTPEnabled)
		mfa.With(RequireAuth(h.auth)).Post("/enroll", h.handleTOTPEnroll)
		mfa.With(RequireAuth(h.auth)).Post("/enroll/confirm", h.handleTOTPEnrollConfirm)
		mfa.With(RequireMFAPending(h.auth)).Post("/challenge", h.handleTOTPChallenge)
		mfa.With(RequireMFAPending(h.auth)).Post("/verify", h.handleTOTPVerify)
	})

	// Email MFA endpoints.
	r.Route("/mfa/email", func(mfa chi.Router) {
		mfa.Use(h.requireEmailMFAEnabled)
		mfa.With(RequireAuth(h.auth)).Post("/enroll", h.handleEmailMFAEnroll)
		mfa.With(RequireAuth(h.auth)).Post("/enroll/confirm", h.handleEmailMFAEnrollConfirm)
		mfa.With(RequireMFAPending(h.auth)).Post("/challenge", h.handleEmailMFAChallenge)
		mfa.With(RequireMFAPending(h.auth)).Post("/verify", h.handleEmailMFAVerify)
	})

	// Backup code endpoints.
	r.Route("/mfa/backup", func(mfa chi.Router) {
		mfa.With(RequireAuth(h.auth), RequireAAL2).Post("/generate", h.handleBackupCodeGenerate)
		mfa.With(RequireAuth(h.auth), RequireAAL2).Post("/regenerate", h.handleBackupCodeRegenerate)
		mfa.With(RequireAuth(h.auth)).Get("/count", h.handleBackupCodeCount)
		mfa.With(RequireMFAPending(h.auth)).Post("/verify", h.handleBackupCodeVerify)
	})

	// MFA factor listing (accepts both regular auth and MFA pending tokens).
	r.With(RequireAuthOrMFAPending(h.auth)).Get("/mfa/factors", h.handleMFAFactors)

	// API key management (requires JWT auth — not API key auth, to prevent key bootstrapping).
	r.Route("/api-keys", func(r chi.Router) {
		r.Use(RequireAuth(h.auth))
		r.Post("/", h.handleCreateAPIKey)
		r.Get("/", h.handleListAPIKeys)
		r.Delete("/{id}", h.handleRevokeAPIKey)
	})

	return r
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type authResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	User         *User  `json:"user"`
}

// handleRegister creates a new user account with email and password, returning tokens and user info on success.
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if !decodeBody(w, r, &req) {
		return
	}

	user, token, refreshToken, err := h.auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			// Strip the "validation error: " sentinel prefix from user-facing message.
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, msg,
				"https://allyourbase.io/guide/authentication")
		case errors.Is(err, ErrEmailTaken):
			httputil.WriteErrorWithDocURL(w, http.StatusConflict, "email already registered",
				"https://allyourbase.io/guide/authentication")
		default:
			h.logger.Error("register error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, authResponse{Token: token, RefreshToken: refreshToken, User: user})
	if h.metricsRecorder != nil {
		h.metricsRecorder.RecordAuthSignup(r.Context())
	}
}

// handleLogin authenticates a user by email and password, returning tokens and user info or a pending MFA token if factors are enrolled.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if !decodeBody(w, r, &req) {
		return
	}

	user, token, refreshToken, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
				"invalid email or password",
				"https://allyourbase.io/guide/authentication")
			return
		}
		h.logger.Error("login error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// When MFA is required, Login() returns a pending token with empty refresh token.
	if refreshToken == "" {
		httputil.WriteJSON(w, http.StatusOK, mfaPendingResponse{
			MFAPending: true,
			MFAToken:   token,
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{Token: token, RefreshToken: refreshToken, User: user})
	if h.metricsRecorder != nil {
		h.metricsRecorder.RecordAuthLogin(r.Context())
	}
}

// handleMe returns the profile information of the authenticated user.
func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.auth.UserByID(r.Context(), claims.Subject)
	if err != nil {
		h.logger.Error("user lookup error", "error", err, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, user)
}

// handleDeleteMe permanently deletes the authenticated user's account and all associated data.
func (h *Handler) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.auth.DeleteUser(r.Context(), claims.Subject); err != nil {
		h.logger.Error("account deletion error", "error", err, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}

	h.logger.Info("user deleted own account", "user_id", claims.Subject, "email", claims.Email)
	w.WriteHeader(http.StatusNoContent)
}

// handleRefresh exchanges a refresh token for new access and refresh tokens.
func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		httputil.WriteError(w, http.StatusBadRequest, "refreshToken is required")
		return
	}

	user, accessToken, refreshToken, err := h.auth.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized,
				"invalid or expired refresh token",
				"https://allyourbase.io/guide/authentication")
			return
		}
		h.logger.Error("refresh error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{Token: accessToken, RefreshToken: refreshToken, User: user})
}

// handleLogout invalidates a refresh token, ending the associated session.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		httputil.WriteError(w, http.StatusBadRequest, "refreshToken is required")
		return
	}

	if err := h.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		h.logger.Error("logout error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListSessions returns all active sessions for the authenticated user, excluding the current session.
func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	sessions, err := h.auth.ListSessions(r.Context(), claims.Subject, claims.SessionID)
	if err != nil {
		h.logger.Error("list sessions error", "error", err, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, sessions)
}

// handleDeleteSession revokes a specific session by ID if it belongs to the authenticated user.
func (h *Handler) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "session id is required")
		return
	}
	if !httputil.IsValidUUID(sessionID) {
		httputil.WriteError(w, http.StatusBadRequest, "invalid session id format")
		return
	}

	err := h.auth.RevokeSession(r.Context(), claims.Subject, sessionID)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			httputil.WriteError(w, http.StatusNotFound, "session not found")
		case errors.Is(err, ErrSessionForbidden):
			httputil.WriteError(w, http.StatusForbidden, "cannot revoke another user's session")
		default:
			h.logger.Error("revoke session error", "error", err, "user_id", claims.Subject, "session_id", sessionID)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteSessions revokes all sessions for the authenticated user except the current one.
func (h *Handler) handleDeleteSessions(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if r.URL.Query().Get("all_except_current") != "true" {
		httputil.WriteError(w, http.StatusBadRequest, "all_except_current=true is required")
		return
	}
	if claims.SessionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "current session id is unavailable")
		return
	}

	err := h.auth.RevokeAllExceptCurrent(r.Context(), claims.Subject, claims.SessionID)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			httputil.WriteError(w, http.StatusBadRequest, "current session id is required")
			return
		}
		h.logger.Error("revoke all except current error", "error", err, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) attachRequestMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent := strings.TrimSpace(r.Header.Get("User-Agent"))
		ipAddress := strings.TrimSpace(clientIP(r))
		ctx := contextWithRequestMetadata(r.Context(), userAgent, ipAddress)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	return httputil.DecodeJSON(w, r, v)
}
