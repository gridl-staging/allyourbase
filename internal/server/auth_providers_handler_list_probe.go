package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// handleAdminAuthProvidersList returns the list of configured OAuth/OIDC providers.
func (s *Server) handleAdminAuthProvidersList(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth is not enabled")
		return
	}
	providers := s.listOAuthProviders()
	httputil.WriteJSON(w, http.StatusOK, authProviderListResponse{Providers: providers})
}

type testProviderResult struct {
	Success  bool   `json:"success"`
	Provider string `json:"provider"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// handleAdminAuthProvidersTest tests connectivity for a configured provider.
func (s *Server) handleAdminAuthProvidersTest(w http.ResponseWriter, r *http.Request) {
	if s.authHandler == nil {
		httputil.WriteError(w, http.StatusNotFound, "auth is not enabled")
		return
	}

	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		httputil.WriteError(w, http.StatusBadRequest, "provider is required")
		return
	}

	// Check if the provider exists at all (has URL config registered).
	_, hasURLs := s.authHandler.GetProviderURLs(provider)
	if !hasURLs {
		httputil.WriteError(w, http.StatusNotFound, "auth provider not found")
		return
	}

	// Check if the provider is enabled and has credentials.
	info, ok := s.lookupOAuthProviderInfo(provider)
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "auth provider not found")
		return
	}
	if !info.Enabled || !info.ClientIDConfigured {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    fmt.Sprintf("provider %q is not configured or not enabled", provider),
		})
		return
	}

	// Test connectivity based on provider type.
	ctx := r.Context()
	if auth.IsBuiltInOAuthProviderName(provider) {
		s.testBuiltInProvider(ctx, w, provider)
	} else {
		s.testOIDCProvider(ctx, w, provider)
	}
}

func (s *Server) testBuiltInProvider(ctx context.Context, w http.ResponseWriter, provider string) {
	pc, ok := s.authHandler.GetProviderURLs(provider)
	if !ok || pc.AuthURL == "" {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    "provider configuration missing authorization endpoint",
		})
		return
	}
	authURL := resolveAuthURL(pc)
	s.testEndpointReachability(ctx, w, provider, authURL, "authorization endpoint")
}

// Tests connectivity to an OIDC provider by fetching its discovery document or checking the authorization endpoint reachability.
func (s *Server) testOIDCProvider(ctx context.Context, w http.ResponseWriter, provider string) {
	pc, ok := s.authHandler.GetProviderURLs(provider)
	if !ok {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    "provider URL configuration not found",
		})
		return
	}

	// For OIDC providers, test by fetching the discovery document.
	issuerURL := pc.DiscoveryURL
	if issuerURL == "" {
		// Fall back to checking the auth URL reachability.
		if pc.AuthURL != "" {
			s.testEndpointReachability(ctx, w, provider, pc.AuthURL, "authorization endpoint")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    "OIDC provider has no discovery URL or authorization endpoint configured",
		})
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	_, err := auth.FetchOIDCDiscovery(issuerURL, client)
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    fmt.Sprintf("OIDC discovery failed: %v", err),
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, testProviderResult{
		Success:  true,
		Provider: provider,
		Message:  "OIDC discovery document is valid and reachable",
	})
}

// Tests if an HTTP endpoint is reachable by sending HEAD or GET requests and checking the response status code.
func (s *Server) testEndpointReachability(ctx context.Context, w http.ResponseWriter, provider, url, label string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client := &http.Client{
		Transport: http.DefaultTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	statusCode, err := endpointStatusCode(ctx, client, http.MethodHead, url)
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    fmt.Sprintf("%s unreachable: %v", label, err),
		})
		return
	}

	if statusCode == http.StatusMethodNotAllowed || statusCode == http.StatusNotImplemented {
		statusCode, err = endpointStatusCode(ctx, client, http.MethodGet, url)
	}
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    fmt.Sprintf("%s unreachable: %v", label, err),
		})
		return
	}

	if statusCode == http.StatusNotFound || statusCode >= http.StatusInternalServerError {
		httputil.WriteJSON(w, http.StatusOK, testProviderResult{
			Success:  false,
			Provider: provider,
			Error:    fmt.Sprintf("%s returned %d", label, statusCode),
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, testProviderResult{
		Success:  true,
		Provider: provider,
		Message:  fmt.Sprintf("%s is reachable", label),
	})
}

func endpointStatusCode(ctx context.Context, client *http.Client, method, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func resolveAuthURL(pc auth.OAuthProviderConfig) string {
	authURL := strings.TrimSpace(pc.AuthURL)
	tenant := strings.TrimSpace(pc.TenantID)
	if tenant == "" {
		tenant = "common"
	}
	return strings.ReplaceAll(authURL, "{tenant}", tenant)
}

func (s *Server) lookupOAuthProviderInfo(provider string) (auth.OAuthProviderInfo, bool) {
	for _, p := range s.listOAuthProviders() {
		if p.Name == provider {
			return p, true
		}
	}
	return auth.OAuthProviderInfo{}, false
}

func (s *Server) listOAuthProviders() []auth.OAuthProviderInfo {
	providers := s.authHandler.ListOAuthProviders()
	for i := range providers {
		applyConfiguredProviderStatus(s.cfg, &providers[i])
	}
	return providers
}

// Updates a provider info struct by looking up the provider in the configuration and setting its enabled status, client ID configured flag, and type.
func applyConfiguredProviderStatus(cfg *config.Config, info *auth.OAuthProviderInfo) {
	if cfg == nil || info == nil {
		return
	}
	if oauthCfg, ok := cfg.Auth.OAuth[info.Name]; ok {
		info.Enabled = oauthCfg.Enabled
		info.ClientIDConfigured = strings.TrimSpace(oauthCfg.ClientID) != ""
		info.Type = "builtin"
		return
	}
	if oidcCfg, ok := cfg.Auth.OIDC[info.Name]; ok {
		info.Enabled = oidcCfg.Enabled
		info.ClientIDConfigured = strings.TrimSpace(oidcCfg.ClientID) != ""
		info.Type = "oidc"
	}
}
