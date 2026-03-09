package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
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

// Handler serves auth HTTP endpoints.
type Handler struct {
	auth              *Service
	oauthAuthorize    oauthAuthorizationProvider
	oauthToken        oauthTokenProvider
	oauthRevoke       oauthRevokeProvider
	logger            *slog.Logger
	oauthClients      map[string]OAuthClientConfig
	oauthProviderURLs map[string]OAuthProviderConfig // per-handler provider URL overrides
	oauthHTTPClient   *http.Client
	oauthStateStore   *OAuthStateStore
	oauthRedirectURL  string
	oauthPublisher    OAuthPublisher // nil when realtime hub not available
	magicLinkEnabled  bool
	smsEnabled        bool
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
		auth:              svc,
		oauthAuthorize:    svc,
		oauthToken:        svc,
		oauthRevoke:       svc,
		logger:            logger,
		oauthClients:      make(map[string]OAuthClientConfig),
		oauthProviderURLs: urls,
		oauthHTTPClient:   oauthHTTPClient,
		oauthStateStore:   NewOAuthStateStore(10 * time.Minute),
	}
}

// SetProviderURLs overrides the OAuth endpoint URLs for a provider on this
// handler instance. This is used in tests to point the token/userinfo
// endpoints at a local fake server without mutating package-level globals.
func (h *Handler) SetProviderURLs(provider string, cfg OAuthProviderConfig) {
	h.oauthProviderURLs[provider] = cfg
}

// SetOAuthProvider registers an OAuth provider with its client credentials.
func (h *Handler) SetOAuthProvider(provider string, client OAuthClientConfig) {
	h.oauthClients[provider] = client
}

// SetOAuthRedirectURL sets the URL to redirect to after OAuth login.
func (h *Handler) SetOAuthRedirectURL(u string) {
	h.oauthRedirectURL = u
}

// SetOAuthPublisher sets the realtime hub for publishing OAuth results to SSE clients.
func (h *Handler) SetOAuthPublisher(pub OAuthPublisher) {
	h.oauthPublisher = pub
}

// SetMagicLinkEnabled enables or disables the magic link endpoints.
func (h *Handler) SetMagicLinkEnabled(enabled bool) {
	h.magicLinkEnabled = enabled
}

// SetSMSEnabled enables or disables the SMS authentication endpoints.
func (h *Handler) SetSMSEnabled(enabled bool) {
	h.smsEnabled = enabled
}

// Routes returns a chi.Router with auth endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/register", h.handleRegister)
	r.Post("/login", h.handleLogin)
	r.Post("/refresh", h.handleRefresh)
	r.Post("/logout", h.handleLogout)
	r.With(RequireAuth(h.auth)).Get("/me", h.handleMe)
	r.With(RequireAuth(h.auth)).Delete("/me", h.handleDeleteMe)
	r.Post("/password-reset", h.handlePasswordReset)
	r.Post("/password-reset/confirm", h.handlePasswordResetConfirm)
	r.Post("/verify", h.handleVerifyEmail)
	r.With(RequireAuth(h.auth)).Post("/verify/resend", h.handleResendVerification)
	r.Post("/magic-link", h.handleMagicLinkRequest)
	r.Post("/magic-link/confirm", h.handleMagicLinkConfirm)
	r.Get("/oauth/{provider}", h.handleOAuthRedirect)
	r.Get("/oauth/{provider}/callback", h.handleOAuthCallback)
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
}

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
}

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

type passwordResetRequest struct {
	Email string `json:"email"`
}

type passwordResetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

func (h *Handler) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always return 200 to prevent email enumeration.
	if err := h.auth.RequestPasswordReset(r.Context(), req.Email); err != nil {
		h.logger.Error("password reset error", "error", err)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "if that email exists, a reset link has been sent"})
}

func (h *Handler) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req passwordResetConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}
	if req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "password is required")
		return
	}

	err := h.auth.ConfirmPasswordReset(r.Context(), req.Token, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidResetToken):
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired reset token",
				"https://allyourbase.io/guide/authentication")
		case errors.Is(err, ErrValidation):
			msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
			httputil.WriteError(w, http.StatusBadRequest, msg)
		default:
			h.logger.Error("password reset confirm error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "password has been reset"})
}

func (h *Handler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	err := h.auth.ConfirmEmail(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidVerifyToken) {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired verification token",
				"https://allyourbase.io/guide/authentication")
			return
		}
		h.logger.Error("email verification error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *Handler) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.auth.SendVerificationEmail(r.Context(), claims.Subject, claims.Email); err != nil {
		h.logger.Error("resend verification error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "verification email sent"})
}

// requireSMSEnabled is middleware that returns 404 when SMS is not configured.
func (h *Handler) requireSMSEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.smsEnabled {
			httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS MFA is not enabled",
				"https://allyourbase.io/guide/authentication#sms")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	return httputil.DecodeJSON(w, r, v)
}

type magicLinkRequest struct {
	Email string `json:"email"`
}

func (h *Handler) handleMagicLinkRequest(w http.ResponseWriter, r *http.Request) {
	if !h.magicLinkEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "magic link authentication is not enabled",
			"https://allyourbase.io/guide/authentication#magic-link")
		return
	}

	var req magicLinkRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Email == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always return 200 to prevent email enumeration.
	if err := h.auth.RequestMagicLink(r.Context(), req.Email); err != nil {
		h.logger.Error("magic link request error", "error", err)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "if valid, a login link has been sent"})
}

func (h *Handler) handleMagicLinkConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.magicLinkEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "magic link authentication is not enabled",
			"https://allyourbase.io/guide/authentication#magic-link")
		return
	}

	var req tokenRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	user, accessToken, refreshToken, err := h.auth.ConfirmMagicLink(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidMagicLinkToken) {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired magic link token",
				"https://allyourbase.io/guide/authentication#magic-link")
			return
		}
		h.logger.Error("magic link confirm error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if refreshToken == "" {
		httputil.WriteJSON(w, http.StatusOK, mfaPendingResponse{
			MFAPending: true,
			MFAToken:   accessToken,
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{Token: accessToken, RefreshToken: refreshToken, User: user})
}

func (h *Handler) handleOAuthRedirect(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	client, ok := h.oauthClients[provider]
	if !ok {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, fmt.Sprintf("OAuth provider %q not configured", provider),
			"https://allyourbase.io/guide/authentication#oauth")
		return
	}

	// If state is provided and corresponds to an active SSE client, use it
	// directly (popup flow). Otherwise, generate a new state token.
	state := r.URL.Query().Get("state")
	if state != "" && h.oauthPublisher != nil && h.oauthPublisher.HasClient(state) {
		// Register the SSE clientId as a valid CSRF state in the state store
		// so the callback can validate it the same way.
		h.oauthStateStore.RegisterExternalState(state)
	} else {
		var err error
		state, err = h.oauthStateStore.Generate()
		if err != nil {
			h.logger.Error("OAuth state generation error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	callbackURL := oauthCallbackURL(r, provider)
	authURL, err := AuthorizationURL(provider, client, callbackURL, state)
	if err != nil {
		h.logger.Error("OAuth URL error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *Handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	client, ok := h.oauthClients[provider]
	if !ok {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, fmt.Sprintf("OAuth provider %q not configured", provider),
			"https://allyourbase.io/guide/authentication#oauth")
		return
	}

	// Check for provider-side errors.
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		desc := r.URL.Query().Get("error_description")
		h.logger.Warn("OAuth provider error", "provider", provider, "error", errMsg, "description", desc)
		state := r.URL.Query().Get("state")
		// If this was a popup flow, publish the error via SSE and show close page.
		if h.oauthPublisher != nil && h.oauthPublisher.HasClient(state) {
			h.oauthPublisher.PublishOAuth(state, &OAuthEvent{
				Error: "OAuth authentication was denied or failed",
			})
			h.writeOAuthCompletePage(w)
			return
		}
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "OAuth authentication was denied or failed",
			"https://allyourbase.io/guide/authentication#oauth")
		return
	}

	// Validate CSRF state.
	state := r.URL.Query().Get("state")
	isSSEClient := h.oauthPublisher != nil && h.oauthPublisher.HasClient(state)
	if !h.oauthStateStore.Validate(state) {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired OAuth state",
			"https://allyourbase.io/guide/authentication#oauth")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "missing authorization code",
			"https://allyourbase.io/guide/authentication#oauth")
		return
	}

	// Exchange code for user info using this handler's per-instance provider
	// config and HTTP client (so tests can override without touching globals).
	callbackURL := oauthCallbackURL(r, provider)
	pc, ok := h.oauthProviderURLs[provider]
	if !ok {
		h.logger.Error("OAuth provider URL config missing", "provider", provider)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	info, err := exchangeCode(r.Context(), provider, client, code, callbackURL, pc, h.oauthHTTPClient)
	if err != nil {
		h.logger.Error("OAuth code exchange error", "provider", provider, "error", err)
		if isSSEClient {
			h.oauthPublisher.PublishOAuth(state, &OAuthEvent{
				Error: "failed to authenticate with provider",
			})
			h.writeOAuthCompletePage(w)
			return
		}
		httputil.WriteErrorWithDocURL(w, http.StatusBadGateway, "failed to authenticate with provider",
			"https://allyourbase.io/guide/authentication#oauth")
		return
	}

	// Find or create user + issue tokens.
	user, accessToken, refreshToken, err := h.auth.OAuthLogin(r.Context(), provider, info)
	if err != nil {
		h.logger.Error("OAuth login error", "provider", provider, "error", err)
		if isSSEClient {
			h.oauthPublisher.PublishOAuth(state, &OAuthEvent{Error: "internal error"})
			h.writeOAuthCompletePage(w)
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// When MFA is required, OAuthLogin returns a pending token with empty refresh token.
	isMFAPending := refreshToken == ""

	// SSE popup flow: publish tokens via SSE and show auto-close page.
	if isSSEClient {
		evt := &OAuthEvent{
			Token:        accessToken,
			RefreshToken: refreshToken,
			User:         user,
		}
		if isMFAPending {
			evt.MFAPending = true
			evt.MFAToken = accessToken
			evt.Token = ""
			evt.User = nil
		}
		h.oauthPublisher.PublishOAuth(state, evt)
		h.writeOAuthCompletePage(w)
		return
	}

	// If a redirect URL is configured, redirect with tokens in hash fragment.
	if h.oauthRedirectURL != "" {
		if isMFAPending {
			fragment := url.Values{
				"mfa_pending": {"true"},
				"mfa_token":   {accessToken},
			}
			dest := h.oauthRedirectURL + "#" + fragment.Encode()
			http.Redirect(w, r, dest, http.StatusTemporaryRedirect)
			return
		}
		fragment := url.Values{
			"token":        {accessToken},
			"refreshToken": {refreshToken},
		}
		dest := h.oauthRedirectURL + "#" + fragment.Encode()
		http.Redirect(w, r, dest, http.StatusTemporaryRedirect)
		return
	}

	// No redirect URL — return JSON directly.
	if isMFAPending {
		httputil.WriteJSON(w, http.StatusOK, mfaPendingResponse{
			MFAPending: true,
			MFAToken:   accessToken,
		})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// oauthCompletePage is served in the popup after OAuth completes.
// The SDK receives data via SSE; this page just provides visual feedback and auto-closes.
const oauthCompletePage = `<!DOCTYPE html>
<html><head><title>Authentication Complete</title></head>
<body>
<p>Authentication complete. You can close this window.</p>
<script>window.close();</script>
</body></html>`

func (h *Handler) writeOAuthCompletePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(oauthCompletePage))
}

// oauthCallbackURL derives the OAuth callback URL from the current request.
func oauthCallbackURL(r *http.Request, provider string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	return fmt.Sprintf("%s://%s/api/auth/oauth/%s/callback", scheme, r.Host, provider)
}
