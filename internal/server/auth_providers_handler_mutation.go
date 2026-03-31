package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// handleAdminAuthProvidersUpdate updates one provider configuration at runtime.
func (s *Server) handleAdminAuthProvidersUpdate(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth is not enabled")
		return
	}

	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}

	var req updateAuthProviderRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	var err error
	if auth.IsBuiltInOAuthProviderName(provider) {
		err = s.updateBuiltInAuthProvider(provider, req)
	} else {
		err = s.updateOIDCAuthProvider(provider, req)
	}
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, ok := s.lookupOAuthProviderInfo(provider)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load updated provider")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, info)
}

// handleAdminAuthProvidersDelete removes one provider configuration at runtime.
func (s *Server) handleAdminAuthProvidersDelete(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth is not enabled")
		return
	}

	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}

	var err error
	if auth.IsBuiltInOAuthProviderName(provider) {
		err = s.deleteBuiltInAuthProvider(provider)
	} else {
		err = s.deleteOIDCAuthProvider(provider)
	}
	if err != nil {
		if errors.Is(err, errAuthProviderNotFound) {
			httputil.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteBuiltInAuthProvider(provider string) error {
	if s.cfg.Auth.OAuth != nil {
		delete(s.cfg.Auth.OAuth, provider)
	}
	applyRuntimeBuiltInProvider(s.authHandler, provider, config.OAuthProvider{})
	return nil
}

func (s *Server) deleteOIDCAuthProvider(provider string) error {
	_, runtimeExists := s.authHandler.GetProviderURLs(provider)
	configExists := false
	if s.cfg.Auth.OIDC != nil {
		_, configExists = s.cfg.Auth.OIDC[provider]
		delete(s.cfg.Auth.OIDC, provider)
	}
	if !runtimeExists && !configExists {
		return errAuthProviderNotFound
	}

	auth.UnregisterOIDCProvider(provider)
	s.authHandler.RemoveOAuthProvider(provider)
	return nil
}

// Updates a built-in OAuth provider's configuration at runtime by validating request fields for the specific provider type, applying the request, validating the result, and applying it to the auth handler.
func (s *Server) updateBuiltInAuthProvider(provider string, req updateAuthProviderRequest) error {
	if req.IssuerURL != nil || req.Scopes != nil || req.DisplayName != nil {
		return fmt.Errorf("issuer_url, scopes, and display_name are only supported for OIDC providers")
	}
	if provider != "microsoft" && req.TenantID != nil {
		return fmt.Errorf("tenant_id is only supported for microsoft")
	}
	if provider != "apple" && (req.TeamID != nil || req.KeyID != nil || req.PrivateKey != nil) {
		return fmt.Errorf("team_id, key_id, and private_key are only supported for apple")
	}
	if provider != "facebook" && req.FacebookAPIVersion != nil {
		return fmt.Errorf("facebook_api_version is only supported for facebook")
	}
	if provider != "gitlab" && req.GitLabBaseURL != nil {
		return fmt.Errorf("gitlab_base_url is only supported for gitlab")
	}

	if s.cfg.Auth.OAuth == nil {
		s.cfg.Auth.OAuth = make(map[string]config.OAuthProvider)
	}
	cfg := s.cfg.Auth.OAuth[provider]
	applyBuiltInProviderRequest(&cfg, req)
	if err := validateBuiltInProviderConfig(provider, cfg); err != nil {
		return err
	}

	s.cfg.Auth.OAuth[provider] = cfg
	applyRuntimeBuiltInProvider(s.authHandler, provider, cfg)
	return nil
}

// Applies non-nil fields from the request to the provider configuration, trimming whitespace from string values.
func applyBuiltInProviderRequest(cfg *config.OAuthProvider, req updateAuthProviderRequest) {
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.ClientID != nil {
		cfg.ClientID = strings.TrimSpace(*req.ClientID)
	}
	if req.ClientSecret != nil {
		cfg.ClientSecret = strings.TrimSpace(*req.ClientSecret)
	}
	if req.StoreProviderTokens != nil {
		cfg.StoreProviderTokens = *req.StoreProviderTokens
	}
	if req.TenantID != nil {
		cfg.TenantID = strings.TrimSpace(*req.TenantID)
	}
	if req.TeamID != nil {
		cfg.TeamID = strings.TrimSpace(*req.TeamID)
	}
	if req.KeyID != nil {
		cfg.KeyID = strings.TrimSpace(*req.KeyID)
	}
	if req.PrivateKey != nil {
		cfg.PrivateKey = strings.TrimSpace(*req.PrivateKey)
	}
	if req.FacebookAPIVersion != nil {
		cfg.FacebookAPIVersion = strings.TrimSpace(*req.FacebookAPIVersion)
	}
	if req.GitLabBaseURL != nil {
		cfg.GitLabBaseURL = strings.TrimSpace(*req.GitLabBaseURL)
	}
}

// Validates that a provider configuration has all required fields when enabled, returning an error if any required field is missing.
func validateBuiltInProviderConfig(provider string, cfg config.OAuthProvider) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.ClientID == "" {
		return fmt.Errorf("auth.oauth.%s.client_id is required when enabled", provider)
	}
	switch provider {
	case "apple":
		if cfg.TeamID == "" {
			return fmt.Errorf("auth.oauth.apple.team_id is required when enabled")
		}
		if cfg.KeyID == "" {
			return fmt.Errorf("auth.oauth.apple.key_id is required when enabled")
		}
		if cfg.PrivateKey == "" {
			return fmt.Errorf("auth.oauth.apple.private_key is required when enabled")
		}
	default:
		if cfg.ClientSecret == "" {
			return fmt.Errorf("auth.oauth.%s.client_secret is required when enabled", provider)
		}
	}
	return nil
}

// Applies a built-in OAuth provider configuration to the runtime auth handler
// using handler-local provider URLs so concurrent server instances do not
// fight over package-global OAuth provider state.
func applyRuntimeBuiltInProvider(h *auth.Handler, provider string, cfg config.OAuthProvider) {
	if pc, ok := auth.GetProviderConfigRaw(provider); ok {
		switch provider {
		case "facebook":
			version := strings.TrimSpace(cfg.FacebookAPIVersion)
			if version != "" {
				pc.AuthURL = "https://www.facebook.com/" + version + "/dialog/oauth"
				pc.TokenURL = "https://graph.facebook.com/" + version + "/oauth/access_token"
				pc.UserInfoURL = "https://graph.facebook.com/" + version + "/me?fields=id,name,email,picture"
			}
		case "gitlab":
			baseURL := strings.TrimRight(strings.TrimSpace(cfg.GitLabBaseURL), "/")
			if baseURL != "" {
				pc.AuthURL = baseURL + "/oauth/authorize"
				pc.TokenURL = baseURL + "/oauth/token"
				pc.UserInfoURL = baseURL + "/api/v4/user"
			}
		}
		h.SetProviderURLs(provider, pc)
	}
	if provider == "microsoft" {
		tenant := strings.TrimSpace(cfg.TenantID)
		if tenant == "" {
			tenant = "common"
		}
		h.SetOAuthProviderTenantID(provider, tenant)
	}

	if cfg.Enabled {
		h.SetOAuthProvider(provider, auth.OAuthClientConfig{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
		})
		h.SetOAuthProviderTokenStorage(provider, cfg.StoreProviderTokens)
		if provider == "apple" {
			h.SetAppleSignInConfig(auth.AppleClientSecretParams{
				TeamID:     cfg.TeamID,
				ClientID:   cfg.ClientID,
				KeyID:      cfg.KeyID,
				PrivateKey: cfg.PrivateKey,
			})
		}
		return
	}
	h.UnsetOAuthProvider(provider)
	h.SetOAuthProviderTokenStorage(provider, false)
}

// Updates an OIDC provider's configuration at runtime by applying request fields, validating the configuration, and registering or unregistering the provider with the auth handler as needed.
func (s *Server) updateOIDCAuthProvider(provider string, req updateAuthProviderRequest) error {
	if req.TenantID != nil || req.TeamID != nil || req.KeyID != nil || req.PrivateKey != nil ||
		req.FacebookAPIVersion != nil || req.GitLabBaseURL != nil {
		return fmt.Errorf("tenant_id, team_id, key_id, private_key, facebook_api_version, and gitlab_base_url are only supported for built-in providers")
	}

	if s.cfg.Auth.OIDC == nil {
		s.cfg.Auth.OIDC = make(map[string]config.OIDCProvider)
	}
	cfg := s.cfg.Auth.OIDC[provider]
	applyOIDCProviderRequest(&cfg, req)
	if err := validateOIDCProviderConfig(provider, cfg); err != nil {
		return err
	}

	if cfg.Enabled {
		cache := auth.NewOIDCDiscoveryCache(24 * time.Hour)
		if err := auth.RegisterOIDCProvider(provider, auth.OIDCProviderRegistration{
			IssuerURL:    cfg.IssuerURL,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Scopes:       cfg.Scopes,
			DisplayName:  cfg.DisplayName,
		}, cache); err != nil {
			return fmt.Errorf("registering OIDC provider: %w", err)
		}
		pc, ok := auth.GetProviderConfigRaw(provider)
		if !ok {
			return fmt.Errorf("OIDC provider %q was not registered", provider)
		}
		s.authHandler.SetProviderURLs(provider, pc)
		s.authHandler.SetOAuthProvider(provider, auth.OAuthClientConfig{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
		})
		s.authHandler.SetOAuthProviderTokenStorage(provider, false)
	} else {
		auth.UnregisterOIDCProvider(provider)
		s.authHandler.UnsetOAuthProvider(provider)
		s.authHandler.SetOAuthProviderTokenStorage(provider, false)
		s.authHandler.SetProviderURLs(provider, auth.OAuthProviderConfig{
			DiscoveryURL: cfg.IssuerURL,
			Scopes:       append([]string(nil), cfg.Scopes...),
		})
	}

	s.cfg.Auth.OIDC[provider] = cfg
	return nil
}

// Applies non-nil fields from the request to the OIDC provider configuration, trimming whitespace from string values and copying scope slices.
func applyOIDCProviderRequest(cfg *config.OIDCProvider, req updateAuthProviderRequest) {
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.IssuerURL != nil {
		cfg.IssuerURL = strings.TrimSpace(*req.IssuerURL)
	}
	if req.ClientID != nil {
		cfg.ClientID = strings.TrimSpace(*req.ClientID)
	}
	if req.ClientSecret != nil {
		cfg.ClientSecret = strings.TrimSpace(*req.ClientSecret)
	}
	if req.Scopes != nil {
		cfg.Scopes = append([]string(nil), (*req.Scopes)...)
	}
	if req.DisplayName != nil {
		cfg.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
}

func validateOIDCProviderConfig(provider string, cfg config.OIDCProvider) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.IssuerURL == "" {
		return fmt.Errorf("auth.oidc.%s.issuer_url is required when enabled", provider)
	}
	if cfg.ClientID == "" {
		return fmt.Errorf("auth.oidc.%s.client_id is required when enabled", provider)
	}
	if cfg.ClientSecret == "" {
		return fmt.Errorf("auth.oidc.%s.client_secret is required when enabled", provider)
	}
	return nil
}
