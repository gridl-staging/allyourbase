// Package auth Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/auth/handler_oauth.go.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// handleOAuthRedirect redirects the user to the OAuth provider's authorization endpoint with a generated CSRF state token.
func (h *Handler) handleOAuthRedirect(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	client, ok := h.getOAuthClient(provider)
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
	pc, ok := h.getOAuthProviderConfig(provider)
	if !ok {
		h.logger.Error("OAuth provider URL config missing", "provider", provider)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	authURL, err := authorizationURLWithConfig(provider, client, callbackURL, state, pc)
	if err != nil {
		h.logger.Error("OAuth URL error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// handleSAMLLogin initiates SAML authentication by creating an authentication request and redirecting to the identity provider.
func (h *Handler) handleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	if h.samlSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, "SAML is not configured")
		return
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}
	relayState := strings.TrimSpace(r.URL.Query().Get("redirect_to"))
	if relayState != "" && strings.TrimSpace(h.oauthRedirectURL) != "" {
		validated, ok := validatedSAMLRelayRedirect(h.oauthRedirectURL, relayState)
		if !ok {
			httputil.WriteError(w, http.StatusBadRequest, "invalid redirect_to")
			return
		}
		relayState = validated
	}
	redirectURL, requestID, err := h.samlSvc.InitiateLogin(provider, relayState)
	if err != nil {
		if errors.Is(err, errSAMLProviderNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "SAML provider not configured")
			return
		}
		h.logger.Error("SAML initiate login error", "provider", provider, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "ayb_saml_req_" + provider,
		Value:    requestID,
		Path:     "/api/auth/saml/" + provider + "/acs",
		MaxAge:   int((5 * time.Minute).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, redirectURL.String(), http.StatusTemporaryRedirect)
}

// handleSAMLACS handles the SAML Assertion Consumer Service callback, validates the response, and returns tokens or redirects the user.
func (h *Handler) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	if h.samlSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, "SAML is not configured")
		return
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if err := r.ParseForm(); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid form data")
		return
	}
	requestID := strings.TrimSpace(r.FormValue("request_id"))
	if requestID == "" {
		if c, err := r.Cookie("ayb_saml_req_" + provider); err == nil {
			requestID = strings.TrimSpace(c.Value)
		}
	}
	user, accessToken, refreshToken, relayState, err := h.samlSvc.HandleCallback(r, provider, requestID)
	if err != nil {
		h.logger.Error("SAML callback error", "provider", provider, "error", err)
		httputil.WriteError(w, http.StatusUnauthorized, "invalid SAML response")
		return
	}
	if h.oauthRedirectURL != "" {
		fragment := url.Values{}
		if refreshToken == "" {
			fragment.Set("mfa_pending", "true")
			fragment.Set("mfa_token", accessToken)
		} else {
			fragment.Set("token", accessToken)
			fragment.Set("refreshToken", refreshToken)
		}
		destBase := h.oauthRedirectURL
		if relayState != "" {
			if validated, ok := validatedSAMLRelayRedirect(h.oauthRedirectURL, relayState); ok {
				destBase = validated
			} else {
				h.logger.Warn("discarding invalid SAML relay redirect", "redirect_to", relayState)
			}
		}
		dest := destBase + "#" + fragment.Encode()
		http.Redirect(w, r, dest, http.StatusTemporaryRedirect)
		return
	}
	writeAuthCallbackTokenResponse(w, user, accessToken, refreshToken)
}

// handleSAMLMetadata returns the service provider's SAML metadata XML.
func (h *Handler) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	if h.samlSvc == nil {
		httputil.WriteError(w, http.StatusNotFound, "SAML is not configured")
		return
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}
	metadata, err := h.samlSvc.SPMetadataXML(provider)
	if err != nil {
		if errors.Is(err, errSAMLProviderNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "SAML provider not configured")
			return
		}
		h.logger.Error("SAML metadata error", "provider", provider, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(metadata)
}

func oauthCallbackParam(r *http.Request, key string) string {
	if r.Method == http.MethodPost {
		return r.FormValue(key)
	}
	return r.URL.Query().Get(key)
}

type oauthCallbackRequest struct {
	provider    string
	client      OAuthClientConfig
	state       string
	code        string
	isSSEClient bool
}

// TODO: Document Handler.handleOAuthCallback.
func (h *Handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	callbackReq, ok := h.normalizeOAuthCallbackRequest(w, r)
	if !ok {
		return
	}

	info, tokenResp, ok := h.exchangeAndEnrichOAuthCallbackUser(w, r, callbackReq)
	if !ok {
		return
	}

	user, accessToken, refreshToken, ok := h.loginAndPersistOAuthCallback(w, r, callbackReq, info, tokenResp)
	if !ok {
		return
	}

	h.dispatchOAuthCallbackResponse(w, r, callbackReq, user, accessToken, refreshToken)
}

// TODO: Document Handler.normalizeOAuthCallbackRequest.
func (h *Handler) normalizeOAuthCallbackRequest(w http.ResponseWriter, r *http.Request) (*oauthCallbackRequest, bool) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid form data")
			return nil, false
		}
	}

	req := &oauthCallbackRequest{
		provider: chi.URLParam(r, "provider"),
	}
	var ok bool
	req.client, ok = h.getOAuthClient(req.provider)
	if !ok {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, fmt.Sprintf("OAuth provider %q not configured", req.provider),
			"https://allyourbase.io/guide/authentication#oauth")
		return nil, false
	}

	errMsg := oauthCallbackParam(r, "error")
	if errMsg != "" {
		desc := oauthCallbackParam(r, "error_description")
		h.logger.Warn("OAuth provider error", "provider", req.provider, "error", errMsg, "description", desc)
		state := oauthCallbackParam(r, "state")
		if h.oauthPublisher != nil && h.oauthPublisher.HasClient(state) {
			h.oauthPublisher.PublishOAuth(state, &OAuthEvent{Error: "OAuth authentication was denied or failed"})
			h.writeOAuthCompletePage(w)
			return nil, false
		}
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "OAuth authentication was denied or failed",
			"https://allyourbase.io/guide/authentication#oauth")
		return nil, false
	}

	req.state = oauthCallbackParam(r, "state")
	req.isSSEClient = h.oauthPublisher != nil && h.oauthPublisher.HasClient(req.state)
	if !h.oauthStateStore.Validate(req.state) {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid or expired OAuth state",
			"https://allyourbase.io/guide/authentication#oauth")
		return nil, false
	}

	req.code = oauthCallbackParam(r, "code")
	if req.code == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "missing authorization code",
			"https://allyourbase.io/guide/authentication#oauth")
		return nil, false
	}

	return req, true
}

// TODO: Document Handler.exchangeAndEnrichOAuthCallbackUser.
func (h *Handler) exchangeAndEnrichOAuthCallbackUser(w http.ResponseWriter, r *http.Request, callbackReq *oauthCallbackRequest) (*OAuthUserInfo, oauthTokenResponse, bool) {
	callbackURL := oauthCallbackURL(r, callbackReq.provider)
	pc, ok := h.getOAuthProviderConfig(callbackReq.provider)
	if !ok {
		h.logger.Error("OAuth provider URL config missing", "provider", callbackReq.provider)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return nil, oauthTokenResponse{}, false
	}

	ctx := withOAuthExpectedNonce(r.Context(), callbackReq.state)
	info, tokenResp, err := exchangeCodeWithTokenResponse(ctx, callbackReq.provider, callbackReq.client, callbackReq.code, callbackURL, pc, h.oauthHTTPClient)
	if err != nil {
		h.logger.Error("OAuth code exchange error", "provider", callbackReq.provider, "error", err)
		if callbackReq.isSSEClient {
			h.oauthPublisher.PublishOAuth(callbackReq.state, &OAuthEvent{
				Error: "failed to authenticate with provider",
			})
			h.writeOAuthCompletePage(w)
			return nil, oauthTokenResponse{}, false
		}
		httputil.WriteErrorWithDocURL(w, http.StatusBadGateway, "failed to authenticate with provider",
			"https://allyourbase.io/guide/authentication#oauth")
		return nil, oauthTokenResponse{}, false
	}

	if callbackReq.provider == "apple" && r.Method == http.MethodPost {
		if userJSON := r.FormValue("user"); userJSON != "" {
			if appleUser, parseErr := ParseAppleUserPayload(userJSON); parseErr == nil {
				if info.Name == "" && appleUser.Name != "" {
					info.Name = appleUser.Name
				}
				if info.Email == "" && appleUser.Email != "" {
					info.Email = appleUser.Email
				}
			}
		}
	}
	return info, tokenResp, true
}

// TODO: Document Handler.loginAndPersistOAuthCallback.
func (h *Handler) loginAndPersistOAuthCallback(w http.ResponseWriter, r *http.Request, callbackReq *oauthCallbackRequest, info *OAuthUserInfo, tokenResp oauthTokenResponse) (*User, string, string, bool) {
	user, accessToken, refreshToken, err := h.oauthLogin(r.Context(), callbackReq.provider, info)
	if err != nil {
		h.logger.Error("OAuth login error", "provider", callbackReq.provider, "error", err)
		if callbackReq.isSSEClient {
			h.oauthPublisher.PublishOAuth(callbackReq.state, &OAuthEvent{Error: "internal error"})
			h.writeOAuthCompletePage(w)
			return nil, "", "", false
		}
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return nil, "", "", false
	}

	if h.shouldStoreProviderTokens(callbackReq.provider) && strings.TrimSpace(tokenResp.RefreshToken) != "" {
		expiresAt := providerTokenExpiresAt(time.Now(), tokenResp)
		if err := h.auth.StoreProviderTokens(
			r.Context(),
			user.ID,
			callbackReq.provider,
			tokenResp.AccessToken,
			tokenResp.RefreshToken,
			tokenResp.TokenType,
			tokenResp.Scope,
			expiresAt,
		); err != nil {
			h.logger.Error("failed to persist provider tokens", "provider", callbackReq.provider, "user_id", user.ID, "error", err)
			if callbackReq.isSSEClient {
				h.oauthPublisher.PublishOAuth(callbackReq.state, &OAuthEvent{Error: "internal error"})
				h.writeOAuthCompletePage(w)
				return nil, "", "", false
			}
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return nil, "", "", false
		}
	}
	return user, accessToken, refreshToken, true
}

// TODO: Document Handler.dispatchOAuthCallbackResponse.
func (h *Handler) dispatchOAuthCallbackResponse(w http.ResponseWriter, r *http.Request, callbackReq *oauthCallbackRequest, user *User, accessToken, refreshToken string) {
	isMFAPending := refreshToken == ""

	if callbackReq.isSSEClient {
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
		h.oauthPublisher.PublishOAuth(callbackReq.state, evt)
		h.writeOAuthCompletePage(w)
		return
	}

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
	writeAuthCallbackTokenResponse(w, user, accessToken, refreshToken)
}

const oauthCompletePage = `<!DOCTYPE html><html><head><title>Authentication Complete</title></head><body><p>Authentication complete. You can close this window.</p><script>window.close();</script></body></html>`

func (h *Handler) writeOAuthCompletePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(oauthCompletePage))
}

func writeAuthCallbackTokenResponse(w http.ResponseWriter, user *User, accessToken, refreshToken string) {
	if refreshToken == "" {
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

// TODO: Document validatedSAMLRelayRedirect.
func validatedSAMLRelayRedirect(baseRedirectURL, relayState string) (string, bool) {
	baseRaw := strings.TrimSpace(baseRedirectURL)
	if baseRaw == "" {
		return "", false
	}
	baseURL, err := url.Parse(baseRaw)
	if err != nil || !baseURL.IsAbs() {
		return "", false
	}

	candidateRaw := strings.TrimSpace(relayState)
	if candidateRaw == "" {
		return baseURL.String(), true
	}
	candidate, err := url.Parse(candidateRaw)
	if err != nil {
		return "", false
	}

	if !candidate.IsAbs() {
		if candidate.User != nil || !strings.HasPrefix(candidate.Path, "/") {
			return "", false
		}
		resolved := *baseURL
		resolved.User = nil
		resolved.Path = candidate.Path
		resolved.RawPath = ""
		resolved.RawQuery = candidate.RawQuery
		resolved.Fragment = ""
		return resolved.String(), true
	}

	if candidate.User != nil || !sameRedirectOrigin(baseURL, candidate) {
		return "", false
	}
	candidate.Fragment = ""
	return candidate.String(), true
}

func sameRedirectOrigin(base, candidate *url.URL) bool {
	if !strings.EqualFold(base.Scheme, candidate.Scheme) {
		return false
	}
	if !strings.EqualFold(base.Hostname(), candidate.Hostname()) {
		return false
	}
	return urlPortForOrigin(base) == urlPortForOrigin(candidate)
}

func urlPortForOrigin(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func (h *Handler) shouldStoreProviderTokens(provider string) bool {
	h.oauthConfigMu.RLock()
	defer h.oauthConfigMu.RUnlock()
	return h.oauthStoreProviderTokens[provider]
}

func (h *Handler) oauthLogin(ctx context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error) {
	if h.oauthLoginFn != nil {
		return h.oauthLoginFn(ctx, provider, info)
	}
	return h.auth.OAuthLogin(ctx, provider, info)
}
