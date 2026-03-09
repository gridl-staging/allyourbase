package auth

import (
	"context"
	"mime"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
)

// oauthTokenProvider is the subset of auth.Service used by the OAuth token handler.
type oauthTokenProvider interface {
	ValidateOAuthClientCredentials(ctx context.Context, clientID, clientSecret string) (*OAuthClient, error)
	ExchangeAuthorizationCode(ctx context.Context, code, clientID, redirectURI, codeVerifier string) (*OAuthTokenResponse, error)
	ClientCredentialsGrant(ctx context.Context, clientID, scope string, allowedTables []string) (*OAuthTokenResponse, error)
	RefreshOAuthToken(ctx context.Context, refreshToken, clientID string) (*OAuthTokenResponse, error)
}

func (h *Handler) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if !isFormURLEncoded(r.Header.Get("Content-Type")) {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "Content-Type must be application/x-www-form-urlencoded")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "invalid form body")
		return
	}

	grantType := r.PostForm.Get("grant_type")
	if grantType == "" {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "grant_type is required")
		return
	}

	switch grantType {
	case "authorization_code", "client_credentials", "refresh_token":
	default:
		writeOAuthError(w, http.StatusBadRequest, OAuthErrUnsupportedGrantType, "unsupported grant_type")
		return
	}

	clientID, clientSecret, oauthErr := extractOAuthClientCredentials(r)
	if oauthErr != nil {
		writeOAuthError(w, oauthErrorStatus(oauthErr.Code), oauthErr.Code, oauthErr.Description)
		return
	}

	client, err := h.oauthToken.ValidateOAuthClientCredentials(r.Context(), clientID, clientSecret)
	if err != nil {
		if oauthServiceErr, ok := err.(*OAuthError); ok {
			writeOAuthError(w, oauthErrorStatus(oauthServiceErr.Code), oauthServiceErr.Code, oauthServiceErr.Description)
			return
		}
		h.logger.Error("oauth token client authentication failed", "error", err, "client_id", clientID)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var resp *OAuthTokenResponse
	switch grantType {
	case "authorization_code":
		resp, err = h.handleOAuthTokenAuthorizationCodeGrant(r, clientID)
	case "client_credentials":
		resp, err = h.handleOAuthTokenClientCredentialsGrant(r, client)
	case "refresh_token":
		resp, err = h.handleOAuthTokenRefreshGrant(r, clientID)
	}

	if err != nil {
		if oauthServiceErr, ok := err.(*OAuthError); ok {
			writeOAuthError(w, oauthErrorStatus(oauthServiceErr.Code), oauthServiceErr.Code, oauthServiceErr.Description)
			return
		}
		h.logger.Error("oauth token exchange failed", "error", err, "grant_type", grantType, "client_id", clientID)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleOAuthTokenAuthorizationCodeGrant(r *http.Request, clientID string) (*OAuthTokenResponse, error) {
	code := r.PostForm.Get("code")
	if code == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "code is required")
	}

	redirectURI := r.PostForm.Get("redirect_uri")
	if redirectURI == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "redirect_uri is required")
	}

	codeVerifier := r.PostForm.Get("code_verifier")
	if codeVerifier == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "code_verifier is required")
	}

	return h.oauthToken.ExchangeAuthorizationCode(r.Context(), code, clientID, redirectURI, codeVerifier)
}

func (h *Handler) handleOAuthTokenClientCredentialsGrant(r *http.Request, client *OAuthClient) (*OAuthTokenResponse, error) {
	if client.ClientType != OAuthClientTypeConfidential {
		return nil, NewOAuthError(OAuthErrUnauthorizedClient, "client_credentials is only allowed for confidential clients")
	}

	scope := r.PostForm.Get("scope")
	if scope == "" {
		return nil, NewOAuthError(OAuthErrInvalidScope, "scope is required")
	}
	if !IsScopeSubset(scope, client.Scopes) {
		return nil, NewOAuthError(OAuthErrInvalidScope, "requested scope is not allowed for this client")
	}

	return h.oauthToken.ClientCredentialsGrant(
		r.Context(),
		client.ClientID,
		scope,
		parseAllowedTables(r.PostForm["allowed_tables"]),
	)
}

func (h *Handler) handleOAuthTokenRefreshGrant(r *http.Request, clientID string) (*OAuthTokenResponse, error) {
	refreshToken := r.PostForm.Get("refresh_token")
	if refreshToken == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "refresh_token is required")
	}
	return h.oauthToken.RefreshOAuthToken(r.Context(), refreshToken, clientID)
}

func extractOAuthClientCredentials(r *http.Request) (clientID, clientSecret string, oauthErr *OAuthError) {
	basicHeader := r.Header.Get("Authorization")
	basicClientID, basicClientSecret, hasBasic := r.BasicAuth()
	bodyClientID := strings.TrimSpace(r.PostForm.Get("client_id"))
	bodyClientSecret := r.PostForm.Get("client_secret")

	if hasBasic || strings.HasPrefix(strings.ToLower(basicHeader), "basic ") {
		if bodyClientID != "" || bodyClientSecret != "" {
			return "", "", NewOAuthError(OAuthErrInvalidRequest, "multiple client authentication methods are not allowed")
		}
		if !hasBasic || strings.TrimSpace(basicClientID) == "" {
			return "", "", NewOAuthError(OAuthErrInvalidClient, "invalid client authentication")
		}
		return basicClientID, basicClientSecret, nil
	}

	if bodyClientID == "" {
		return "", "", NewOAuthError(OAuthErrInvalidClient, "client_id is required")
	}

	return bodyClientID, bodyClientSecret, nil
}

func isFormURLEncoded(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/x-www-form-urlencoded"
}

func oauthErrorStatus(code string) int {
	if code == OAuthErrInvalidClient {
		return http.StatusUnauthorized
	}
	return http.StatusBadRequest
}
