package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"log/slog"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/caddyserver/certmagic"
)

// configureTLSDefaults sets certmagic global ACME defaults (CA URL, email)
// based on config. Must be called before certmagic.NewDefault().
func configureTLSDefaults(cfg *config.Config, logger *slog.Logger) {
	if cfg.Server.TLSStaging {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
		logger.Info("using Let's Encrypt staging CA")
	}

	if cfg.Server.TLSEmail != "" {
		certmagic.DefaultACME.Email = cfg.Server.TLSEmail
	}
}

// buildCertmagicConfig creates and configures a certmagic Config and Cache for use with
// the TLS listener and the CertManager. Must be called synchronously before the
// server-start goroutine so the CertManager is available at job handler registration time.
func buildCertmagicConfig(cfg *config.Config, logger *slog.Logger) (*certmagic.Config, *certmagic.Cache) {
	certDir := cfg.Server.TLSCertDir
	if certDir == "" {
		home, _ := os.UserHomeDir()
		certDir = filepath.Join(home, ".ayb", "certs")
	}

	configureTLSDefaults(cfg, logger)

	// Standard certmagic circular-init pattern: declare first, create cache with
	// closure that captures the variable (not the nil value), then assign.
	var magic *certmagic.Config
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			return magic, nil
		},
	})
	magic = certmagic.New(cache, certmagic.Config{})
	magic.Storage = &certmagic.FileStorage{Path: certDir}

	return magic, cache
}

// buildTLSListener uses a pre-configured certmagic.Config to obtain a Let's Encrypt
// certificate and returns a TLS net.Listener on port 443 plus an *http.Server for the
// port 80 HTTP-01 challenge responder / redirect. The caller must shut down the
// redirect server during graceful shutdown.
func buildTLSListener(ctx context.Context, cfg *config.Config, magic *certmagic.Config, logger *slog.Logger) (net.Listener, *http.Server, error) {
	logger.Info("obtaining TLS certificate", "domain", cfg.Server.TLSDomain)
	if err := magic.ManageSync(ctx, []string{cfg.Server.TLSDomain}); err != nil {
		return nil, nil, fmt.Errorf("obtaining TLS certificate for %s: %w", cfg.Server.TLSDomain, err)
	}

	tlsCfg := magic.TLSConfig()

	// Port 80: handle HTTP-01 ACME challenges and redirect everything else to https.
	domain := cfg.Server.TLSDomain
	handler := certmagic.DefaultACME.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "https://" + domain + r.RequestURI
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}))
	redirectSrv := &http.Server{
		Addr:              ":80",
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if err := redirectSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn("HTTP redirect listener error", "error", err)
		}
	}()

	ln, err := tls.Listen("tcp", fmt.Sprintf("%s:443", cfg.Server.Host), tlsCfg)
	if err != nil {
		_ = redirectSrv.Close()
		return nil, nil, fmt.Errorf("TLS listen on :443: %w", err)
	}
	return ln, redirectSrv, nil
}
