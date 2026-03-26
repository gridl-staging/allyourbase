package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
)

// SetProviderURLs overrides the OAuth endpoint URLs for a provider on this
// handler instance. This is used in tests to point the token/userinfo
// endpoints at a local fake server without mutating package-level globals.
func (h *Handler) SetProviderURLs(provider string, cfg OAuthProviderConfig) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	h.oauthProviderURLs[provider] = cfg
}

// SetOAuthProviderTenantID sets the tenant ID for a provider on this handler.
func (h *Handler) SetOAuthProviderTenantID(provider, tenantID string) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	cfg, ok := h.oauthProviderURLs[provider]
	if !ok {
		return
	}
	cfg.TenantID = strings.TrimSpace(tenantID)
	h.oauthProviderURLs[provider] = cfg
}

// SetOAuthProvider registers an OAuth provider with its client credentials.
func (h *Handler) SetOAuthProvider(provider string, client OAuthClientConfig) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	h.oauthClients[provider] = client
}

// SetOAuthProviderTokenStorage toggles provider token persistence for a provider.
func (h *Handler) SetOAuthProviderTokenStorage(provider string, enabled bool) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	if enabled {
		h.oauthStoreProviderTokens[provider] = true
		return
	}
	delete(h.oauthStoreProviderTokens, provider)
}

// UnsetOAuthProvider removes configured OAuth client credentials for a provider.
func (h *Handler) UnsetOAuthProvider(provider string) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	delete(h.oauthClients, provider)
	delete(h.oauthStoreProviderTokens, provider)
}

// RemoveOAuthProvider removes both credentials and URL config for a provider.
func (h *Handler) RemoveOAuthProvider(provider string) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	delete(h.oauthClients, provider)
	delete(h.oauthProviderURLs, provider)
	delete(h.oauthStoreProviderTokens, provider)
}

// GetProviderURLs returns this handler's current provider URL config.
func (h *Handler) GetProviderURLs(provider string) (OAuthProviderConfig, bool) {
	h.oauthConfigMu.RLock()
	defer h.oauthConfigMu.RUnlock()
	cfg, ok := h.oauthProviderURLs[provider]
	return cfg, ok
}

// ProviderTokenOAuthResolver returns a resolver closure for provider token refresh.
func (h *Handler) ProviderTokenOAuthResolver() ProviderTokenOAuthResolver {
	return func(provider string) (OAuthClientConfig, OAuthProviderConfig, bool) {
		client, ok := h.getOAuthClient(provider)
		if !ok {
			return OAuthClientConfig{}, OAuthProviderConfig{}, false
		}
		pc, ok := h.getOAuthProviderConfig(provider)
		if !ok {
			return OAuthClientConfig{}, OAuthProviderConfig{}, false
		}
		return client, pc, true
	}
}

// SetAppleSignInConfig configures Apple Sign-In with the private key params
// needed for dynamic client_secret JWT generation and id_token verification.
func (h *Handler) SetAppleSignInConfig(params AppleClientSecretParams) {
	h.oauthConfigMu.Lock()
	defer h.oauthConfigMu.Unlock()
	cfg, ok := h.oauthProviderURLs["apple"]
	if !ok {
		cfg = oauthProviders["apple"]
	}

	// Set up the token request mutator to generate a fresh client_secret JWT
	// for each token exchange request.
	cfg.TokenRequestMutator = func(ctx context.Context, provider string, client OAuthClientConfig, form url.Values, headers http.Header) error {
		secret, err := GenerateAppleClientSecret(params)
		if err != nil {
			return fmt.Errorf("generating Apple client_secret: %w", err)
		}
		form.Set("client_secret", secret)
		return nil
	}

	// Set up id_token verification using Apple's JWKS.
	jwksURL := appleJWKSURL
	fetcher := NewAppleJWKSFetcher(jwksURL, 24*time.Hour)
	cfg.IDTokenUserInfoParser = func(_ context.Context, idToken string) (*OAuthUserInfo, error) {
		return VerifyAppleIDToken(idToken, params.ClientID, fetcher)
	}

	h.oauthProviderURLs["apple"] = cfg
}

// SetOAuthRedirectURL sets the URL to redirect to after OAuth login.
func (h *Handler) SetOAuthRedirectURL(u string) {
	h.oauthRedirectURL = u
}

// SetOAuthPublisher sets the realtime hub for publishing OAuth results to SSE clients.
func (h *Handler) SetOAuthPublisher(pub OAuthPublisher) {
	h.oauthPublisher = pub
}

// SetSAMLService attaches the SAML service used by /auth/saml/* routes.
func (h *Handler) SetSAMLService(svc *SAMLService) {
	h.samlSvc = svc
}

// SetMetricsRecorder attaches a metrics recorder for auth events.
func (h *Handler) SetMetricsRecorder(recorder AuthMetricsRecorder) {
	h.metricsRecorder = recorder
}

// SetExistingMFAOverride overrides the HasAnyMFA check for testing the AAL2 guard.
// When set, the handler uses this value instead of querying the database.
func (h *Handler) SetExistingMFAOverride(hasMFA bool) {
	h.existingMFAOverride = &hasMFA
}

// enforceAAL2ForExistingMFA checks whether the user has existing MFA factors
// and requires AAL2 if so. Returns true if the request was rejected (caller should return).
// Fails closed: DB errors return 500 rather than bypassing the AAL2 guard.
func (h *Handler) enforceAAL2ForExistingMFA(w http.ResponseWriter, r *http.Request, claims *Claims) bool {
	if claims == nil || claims.Subject == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return true
	}
	if claims.AAL == "aal2" {
		return false
	}
	var hasMFA bool
	if h.existingMFAOverride != nil {
		hasMFA = *h.existingMFAOverride
	} else {
		var err error
		hasMFA, _, err = h.auth.HasAnyMFA(r.Context(), claims.Subject)
		if err != nil {
			h.logger.Error("checking existing MFA for AAL2 guard", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return true
		}
	}
	if hasMFA {
		httputil.WriteError(w, http.StatusForbidden, "AAL2 session required to enroll additional MFA factors")
		return true
	}
	return false
}

// AuthSettings represents the current runtime state of auth feature toggles.
type AuthSettings struct {
	MagicLinkEnabled     bool `json:"magic_link_enabled"`
	SMSEnabled           bool `json:"sms_enabled"`
	EmailMFAEnabled      bool `json:"email_mfa_enabled"`
	AnonymousAuthEnabled bool `json:"anonymous_auth_enabled"`
	TOTPEnabled          bool `json:"totp_enabled"`
}

// GetAuthSettings returns the current auth feature toggle states.
func (h *Handler) GetAuthSettings() AuthSettings {
	return AuthSettings{
		MagicLinkEnabled:     h.magicLinkEnabled,
		SMSEnabled:           h.smsEnabled,
		EmailMFAEnabled:      h.emailMFAEnabled,
		AnonymousAuthEnabled: h.anonymousAuthEnabled,
		TOTPEnabled:          h.totpEnabled,
	}
}

// UpdateAuthSettings applies the given settings to the handler.
func (h *Handler) UpdateAuthSettings(s AuthSettings) {
	h.magicLinkEnabled = s.MagicLinkEnabled
	h.smsEnabled = s.SMSEnabled
	h.emailMFAEnabled = s.EmailMFAEnabled
	h.SetAnonymousAuthEnabled(s.AnonymousAuthEnabled)
	h.totpEnabled = s.TOTPEnabled
}

// OAuthProviderInfo represents the public metadata for a configured OAuth provider.
type OAuthProviderInfo struct {
	Name               string `json:"name"`
	Type               string `json:"type"` // "builtin" or "oidc"
	Enabled            bool   `json:"enabled"`
	ClientIDConfigured bool   `json:"client_id_configured"`
}

// ListOAuthProviders returns information about all registered OAuth providers
// and their configuration status on this handler.
func (h *Handler) ListOAuthProviders() []OAuthProviderInfo {
	h.oauthConfigMu.RLock()
	defer h.oauthConfigMu.RUnlock()

	// Collect all known providers from the URL config map.
	var providers []OAuthProviderInfo
	for name := range h.oauthProviderURLs {
		_, enabled := h.oauthClients[name]
		provType := "builtin"
		if !isBuiltInOAuthProviderName(name) {
			provType = "oidc"
		}
		clientConfigured := false
		if client, ok := h.oauthClients[name]; ok {
			clientConfigured = client.ClientID != ""
		}
		providers = append(providers, OAuthProviderInfo{
			Name:               name,
			Type:               provType,
			Enabled:            enabled,
			ClientIDConfigured: clientConfigured,
		})
	}
	// Sort by name for deterministic output.
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return providers
}

func (h *Handler) getOAuthClient(provider string) (OAuthClientConfig, bool) {
	h.oauthConfigMu.RLock()
	defer h.oauthConfigMu.RUnlock()
	client, ok := h.oauthClients[provider]
	return client, ok
}

func (h *Handler) getOAuthProviderConfig(provider string) (OAuthProviderConfig, bool) {
	h.oauthConfigMu.RLock()
	defer h.oauthConfigMu.RUnlock()
	cfg, ok := h.oauthProviderURLs[provider]
	return cfg, ok
}

// SetMagicLinkEnabled enables or disables the magic link endpoints.
func (h *Handler) SetMagicLinkEnabled(enabled bool) {
	h.magicLinkEnabled = enabled
}

// SetSMSEnabled enables or disables the SMS authentication endpoints.
func (h *Handler) SetSMSEnabled(enabled bool) {
	h.smsEnabled = enabled
}
