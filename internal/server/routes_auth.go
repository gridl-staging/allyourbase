// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/server/routes_auth.go.
package server

import (
	"context"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// registerAuthRoutes configures auth providers, constructs the auth handler,
// sets up rate limiters, and mounts the auth route group.
func (s *Server) registerAuthRoutes(r chi.Router) {
	if s.authSvc == nil {
		return
	}

	authHandler := auth.NewHandler(s.authSvc, s.logger)
	s.removeUnconfiguredOIDCProviders(authHandler)
	oidcEnabled := s.registerConfiguredOIDCProviders(authHandler)
	s.configureSAMLProviders(authHandler)
	s.configureOAuthProviders(authHandler, oidcEnabled)
	s.configureAuthCapabilities(authHandler)
	s.authHandler = authHandler

	s.configureAuthRateLimiters()
	s.mountAuthRouteGroup(r, authHandler)
}

func (s *Server) removeUnconfiguredOIDCProviders(authHandler *auth.Handler) {
	for _, provider := range authHandler.ListOAuthProviders() {
		if provider.Type == "oidc" {
			if _, configured := s.cfg.Auth.OIDC[provider.Name]; !configured {
				authHandler.RemoveOAuthProvider(provider.Name)
			}
		}
	}
}

// TODO: Document Server.registerConfiguredOIDCProviders.
func (s *Server) registerConfiguredOIDCProviders(authHandler *auth.Handler) map[string]config.OIDCProvider {
	oidcEnabled := make(map[string]config.OIDCProvider)
	if len(s.cfg.Auth.OIDC) == 0 {
		return oidcEnabled
	}

	cache := auth.NewOIDCDiscoveryCache(24 * time.Hour)
	for name, providerConfig := range s.cfg.Auth.OIDC {
		// Clear stale provider state for this name before re-registering.
		auth.UnregisterOIDCProvider(name)
		authHandler.RemoveOAuthProvider(name)
		if !providerConfig.Enabled {
			authHandler.UnsetOAuthProvider(name)
			authHandler.SetOAuthProviderTokenStorage(name, false)
			authHandler.SetProviderURLs(name, auth.OAuthProviderConfig{
				DiscoveryURL: providerConfig.IssuerURL,
				Scopes:       append([]string(nil), providerConfig.Scopes...),
			})
			continue
		}
		if err := auth.RegisterOIDCProvider(name, auth.OIDCProviderRegistration{
			IssuerURL:    providerConfig.IssuerURL,
			ClientID:     providerConfig.ClientID,
			ClientSecret: providerConfig.ClientSecret,
			Scopes:       providerConfig.Scopes,
			DisplayName:  providerConfig.DisplayName,
		}, cache); err != nil {
			s.logger.Error("registering OIDC provider", "provider", name, "error", err)
			continue
		}
		oidcEnabled[name] = providerConfig
	}
	return oidcEnabled
}

// TODO: Document Server.configureSAMLProviders.
func (s *Server) configureSAMLProviders(authHandler *auth.Handler) {
	samlSvc, samlErr := auth.NewSAMLService(s.cfg.PublicBaseURL(), auth.DefaultSAMLDataDir(), s.authSvc, s.logger)
	if samlErr != nil {
		s.logger.Error("initializing SAML service", "error", samlErr)
		return
	}

	for _, providerConfig := range s.cfg.Auth.SAMLProviders {
		if !providerConfig.Enabled {
			continue
		}
		if err := samlSvc.UpsertProvider(context.Background(), providerConfig); err != nil {
			s.logger.Error("registering SAML provider", "name", providerConfig.Name, "error", err)
		}
	}
	authHandler.SetSAMLService(samlSvc)
	s.samlSvc = samlSvc
}

// TODO: Document Server.configureOAuthProviders.
func (s *Server) configureOAuthProviders(authHandler *auth.Handler, oidcEnabled map[string]config.OIDCProvider) {
	// Configure OAuth providers from config.
	for name, providerConfig := range s.cfg.Auth.OAuth {
		applyRuntimeBuiltInProvider(authHandler, name, providerConfig)
	}
	for name, providerConfig := range oidcEnabled {
		if providerURLs, ok := auth.GetProviderConfigRaw(name); ok {
			authHandler.SetProviderURLs(name, providerURLs)
		}
		authHandler.SetOAuthProvider(name, auth.OAuthClientConfig{
			ClientID:     providerConfig.ClientID,
			ClientSecret: providerConfig.ClientSecret,
		})
		authHandler.SetOAuthProviderTokenStorage(name, false)
	}
	s.authSvc.SetProviderTokenResolver(authHandler.ProviderTokenOAuthResolver())
	if s.cfg.Auth.OAuthRedirectURL != "" {
		authHandler.SetOAuthRedirectURL(s.cfg.Auth.OAuthRedirectURL)
	}
	authHandler.SetOAuthPublisher(s.hub)
	if s.infraMetrics != nil {
		authHandler.SetMetricsRecorder(s.infraMetrics)
	}
}

// TODO: Document Server.configureAuthCapabilities.
func (s *Server) configureAuthCapabilities(authHandler *auth.Handler) {
	if s.cfg.Auth.MagicLinkEnabled {
		authHandler.SetMagicLinkEnabled(true)
	}
	if s.cfg.Auth.SMSEnabled {
		authHandler.SetSMSEnabled(true)
	}
	if s.cfg.Auth.EmailMFAEnabled {
		authHandler.SetEmailMFAEnabled(true)
	}
	if s.cfg.Auth.AnonymousAuthEnabled {
		anonRateLimit := s.cfg.Auth.AnonymousRateLimit
		if anonRateLimit <= 0 {
			anonRateLimit = auth.DefaultAnonymousRateLimit
		}
		authHandler.SetAnonymousRateLimit(anonRateLimit)
		authHandler.SetAnonymousAuthEnabled(true)
	}
	if s.cfg.Auth.TOTPEnabled {
		authHandler.SetTOTPEnabled(true)
	}
}

func (s *Server) configureAuthRateLimiters() {
	rl := s.cfg.Auth.RateLimit
	if rl <= 0 {
		rl = 10
	}
	s.authRL = auth.NewRateLimiter(rl, time.Minute)
	authSensitiveLimit, authSensitiveWindow := 10, time.Minute
	if parsedLimit, parsedWindow, err := config.ParseRateLimitSpec(s.cfg.Auth.RateLimitAuth); err != nil {
		s.logger.Warn("invalid auth.rate_limit_auth; using default", "value", s.cfg.Auth.RateLimitAuth, "error", err)
	} else {
		authSensitiveLimit, authSensitiveWindow = parsedLimit, parsedWindow
	}
	s.authSensitiveRL = auth.NewRateLimiter(authSensitiveLimit, authSensitiveWindow)
}

func (s *Server) mountAuthRouteGroup(r chi.Router, authHandler *auth.Handler) {
	r.Route("/auth", func(r chi.Router) {
		r.Use(authRouteRateLimitMiddleware(s.authRL, s.authSensitiveRL))
		r.Use(middleware.AllowContentType("application/json", "application/x-www-form-urlencoded"))
		r.Mount("/", authHandler.Routes())
	})
}
