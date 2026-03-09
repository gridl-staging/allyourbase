package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
)

// oauthAuthorizationProvider is the subset of auth.Service used by OAuth provider-mode handlers.
type oauthAuthorizationProvider interface {
	GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error)
	HasConsent(ctx context.Context, userID, clientID, scope string, allowedTables []string) (bool, error)
	SaveConsent(ctx context.Context, userID, clientID, scope string, allowedTables []string) error
	CreateAuthorizationCode(ctx context.Context, clientID, userID, redirectURI, scope string, allowedTables []string, codeChallenge, codeChallengeMethod, state string) (string, error)
}

type oauthAuthorizeRequest struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	AllowedTables       []string
}

type oauthConsentRequest struct {
	Decision            string   `json:"decision"`
	ResponseType        string   `json:"response_type"`
	ClientID            string   `json:"client_id"`
	RedirectURI         string   `json:"redirect_uri"`
	Scope               string   `json:"scope"`
	State               string   `json:"state"`
	CodeChallenge       string   `json:"code_challenge"`
	CodeChallengeMethod string   `json:"code_challenge_method"`
	AllowedTables       []string `json:"allowed_tables"`
}

// oauthRedirectResponse is returned when Accept: application/json is set and
// the handler would normally issue a 302 redirect. The SPA consent page uses
// this to know where to redirect the browser.
type oauthRedirectResponse struct {
	RequiresConsent bool   `json:"requires_consent"`
	RedirectTo      string `json:"redirect_to"`
}

// wantsJSON returns true if the request prefers a JSON response.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

type oauthConsentPromptResponse struct {
	RequiresConsent     bool     `json:"requires_consent"`
	ClientID            string   `json:"client_id"`
	ClientName          string   `json:"client_name"`
	RedirectURI         string   `json:"redirect_uri"`
	Scope               string   `json:"scope"`
	State               string   `json:"state"`
	CodeChallenge       string   `json:"code_challenge"`
	CodeChallengeMethod string   `json:"code_challenge_method"`
	AllowedTables       []string `json:"allowed_tables,omitempty"`
}

func (h *Handler) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeOAuthError(w, http.StatusUnauthorized, OAuthErrInvalidRequest, "authenticated user session is required")
		return
	}

	req := parseOAuthAuthorizeRequest(r)
	client, oauthErr, status, err := h.validateOAuthAuthorizeRequest(r.Context(), req)
	if oauthErr != nil {
		writeOAuthError(w, status, oauthErr.Code, oauthErr.Description)
		return
	}
	if err != nil {
		h.logger.Error("oauth authorize validation error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	hasConsent, err := h.oauthAuthorize.HasConsent(r.Context(), claims.Subject, req.ClientID, req.Scope, req.AllowedTables)
	if err != nil {
		h.logger.Error("oauth consent lookup error", "error", err, "client_id", req.ClientID, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !hasConsent {
		httputil.WriteJSON(w, http.StatusOK, oauthConsentPromptResponse{
			RequiresConsent:     true,
			ClientID:            client.ClientID,
			ClientName:          client.Name,
			RedirectURI:         req.RedirectURI,
			Scope:               req.Scope,
			State:               req.State,
			CodeChallenge:       req.CodeChallenge,
			CodeChallengeMethod: req.CodeChallengeMethod,
			AllowedTables:       req.AllowedTables,
		})
		return
	}

	code, err := h.oauthAuthorize.CreateAuthorizationCode(
		r.Context(), req.ClientID, claims.Subject, req.RedirectURI,
		req.Scope, req.AllowedTables, req.CodeChallenge, req.CodeChallengeMethod, req.State,
	)
	if err != nil {
		if oauthServiceErr, ok := err.(*OAuthError); ok {
			writeOAuthError(w, http.StatusBadRequest, oauthServiceErr.Code, oauthServiceErr.Description)
			return
		}
		h.logger.Error("oauth authorization code issue error", "error", err, "client_id", req.ClientID, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	redirectURI := buildOAuthRedirectURI(req.RedirectURI, map[string]string{
		"code":  code,
		"state": req.State,
	})
	if wantsJSON(r) {
		httputil.WriteJSON(w, http.StatusOK, oauthRedirectResponse{
			RequiresConsent: false,
			RedirectTo:      redirectURI,
		})
		return
	}
	http.Redirect(w, r, redirectURI, http.StatusFound)
}

func (h *Handler) handleOAuthConsent(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		writeOAuthError(w, http.StatusUnauthorized, OAuthErrInvalidRequest, "authenticated user session is required")
		return
	}

	var req oauthConsentRequest
	if !decodeBody(w, r, &req) {
		return
	}

	if req.Decision != "approve" && req.Decision != "deny" {
		writeOAuthError(w, http.StatusBadRequest, OAuthErrInvalidRequest, "decision must be approve or deny")
		return
	}

	authReq := oauthAuthorizeRequest{
		ResponseType:        req.ResponseType,
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		Scope:               req.Scope,
		State:               req.State,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		AllowedTables:       req.AllowedTables,
	}

	_, oauthErr, status, err := h.validateOAuthAuthorizeRequest(r.Context(), authReq)
	if oauthErr != nil {
		writeOAuthError(w, status, oauthErr.Code, oauthErr.Description)
		return
	}
	if err != nil {
		h.logger.Error("oauth consent validation error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if req.Decision == "deny" {
		denyURI := buildOAuthRedirectURI(authReq.RedirectURI, map[string]string{
			"error":             OAuthErrAccessDenied,
			"error_description": "resource owner denied access",
			"state":             authReq.State,
		})
		if wantsJSON(r) {
			httputil.WriteJSON(w, http.StatusOK, oauthRedirectResponse{RedirectTo: denyURI})
			return
		}
		http.Redirect(w, r, denyURI, http.StatusFound)
		return
	}

	if err := h.oauthAuthorize.SaveConsent(r.Context(), claims.Subject, authReq.ClientID, authReq.Scope, authReq.AllowedTables); err != nil {
		if oauthServiceErr, ok := err.(*OAuthError); ok {
			writeOAuthError(w, http.StatusBadRequest, oauthServiceErr.Code, oauthServiceErr.Description)
			return
		}
		h.logger.Error("oauth consent save error", "error", err, "client_id", authReq.ClientID, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	code, err := h.oauthAuthorize.CreateAuthorizationCode(
		r.Context(), authReq.ClientID, claims.Subject, authReq.RedirectURI,
		authReq.Scope, authReq.AllowedTables, authReq.CodeChallenge, authReq.CodeChallengeMethod, authReq.State,
	)
	if err != nil {
		if oauthServiceErr, ok := err.(*OAuthError); ok {
			writeOAuthError(w, http.StatusBadRequest, oauthServiceErr.Code, oauthServiceErr.Description)
			return
		}
		h.logger.Error("oauth authorization code issue error after consent", "error", err, "client_id", authReq.ClientID, "user_id", claims.Subject)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	redirectURI := buildOAuthRedirectURI(authReq.RedirectURI, map[string]string{
		"code":  code,
		"state": authReq.State,
	})
	if wantsJSON(r) {
		httputil.WriteJSON(w, http.StatusOK, oauthRedirectResponse{RedirectTo: redirectURI})
		return
	}
	http.Redirect(w, r, redirectURI, http.StatusFound)
}

func (h *Handler) validateOAuthAuthorizeRequest(ctx context.Context, req oauthAuthorizeRequest) (*OAuthClient, *OAuthError, int, error) {
	if req.ResponseType != "code" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "response_type must be code"), http.StatusBadRequest, nil
	}
	if req.ClientID == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "client_id is required"), http.StatusBadRequest, nil
	}
	if req.RedirectURI == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "redirect_uri is required"), http.StatusBadRequest, nil
	}
	if req.Scope == "" {
		return nil, NewOAuthError(OAuthErrInvalidScope, "scope is required"), http.StatusBadRequest, nil
	}
	if req.State == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "state is required"), http.StatusBadRequest, nil
	}
	if req.CodeChallenge == "" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "code_challenge is required"), http.StatusBadRequest, nil
	}
	if req.CodeChallengeMethod != "S256" {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "code_challenge_method must be S256"), http.StatusBadRequest, nil
	}

	client, err := h.oauthAuthorize.GetOAuthClient(ctx, req.ClientID)
	if err != nil {
		if errors.Is(err, ErrOAuthClientNotFound) {
			return nil, NewOAuthError(OAuthErrInvalidClient, "invalid client_id"), http.StatusUnauthorized, nil
		}
		return nil, nil, http.StatusInternalServerError, err
	}
	if client.RevokedAt != nil {
		return nil, NewOAuthError(OAuthErrInvalidClient, "client has been revoked"), http.StatusUnauthorized, nil
	}

	if !MatchRedirectURI(req.RedirectURI, client.RedirectURIs) {
		return nil, NewOAuthError(OAuthErrInvalidRequest, "redirect_uri does not match registered URI"), http.StatusBadRequest, nil
	}
	if !IsScopeSubset(req.Scope, client.Scopes) {
		return nil, NewOAuthError(OAuthErrInvalidScope, "requested scope is not allowed for this client"), http.StatusBadRequest, nil
	}

	return client, nil, http.StatusOK, nil
}

func parseOAuthAuthorizeRequest(r *http.Request) oauthAuthorizeRequest {
	q := r.URL.Query()
	return oauthAuthorizeRequest{
		ResponseType:        q.Get("response_type"),
		ClientID:            q.Get("client_id"),
		RedirectURI:         q.Get("redirect_uri"),
		Scope:               q.Get("scope"),
		State:               q.Get("state"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: q.Get("code_challenge_method"),
		AllowedTables:       parseAllowedTables(q["allowed_tables"]),
	}
}

func parseAllowedTables(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			table := strings.TrimSpace(part)
			if table == "" {
				continue
			}
			if _, ok := seen[table]; ok {
				continue
			}
			seen[table] = struct{}{}
			out = append(out, table)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildOAuthRedirectURI(base string, params map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	httputil.WriteJSON(w, status, &OAuthError{Code: code, Description: description})
}
