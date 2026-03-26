// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/server/sites_runtime.go.
package server

import (
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sites"
	"github.com/allyourbase/ayb/internal/storage"
)

type siteRuntimeResolver interface {
	ResolveRuntimeSiteByCustomDomainID(ctx context.Context, customDomainID string) (*sites.RuntimeSite, error)
	ResolveRuntimeSiteBySlug(ctx context.Context, slug string) (*sites.RuntimeSite, error)
}

type siteRuntimeStorage interface {
	Download(ctx context.Context, bucket, name string) (io.ReadCloser, *storage.Object, error)
}

// TODO: Document Server.siteRuntimeMiddleware.
func (s *Server) siteRuntimeMiddleware(next http.Handler) http.Handler {
	if s.siteStore == nil || s.storageSvc == nil {
		return next
	}

	resolver, ok := s.siteStore.(siteRuntimeResolver)
	if !ok {
		return next
	}

	return buildSiteRuntimeMiddlewareWithAdminPath(
		resolver,
		s.storageSvc,
		configuredSiteRuntimeHostBase(s.cfg),
		normalizedAdminPath(s.cfg.Admin.Path),
	)(next)
}

func buildSiteRuntimeMiddleware(resolver siteRuntimeResolver, runtimeStorage siteRuntimeStorage, defaultHostBase string) func(http.Handler) http.Handler {
	return buildSiteRuntimeMiddlewareWithAdminPath(resolver, runtimeStorage, defaultHostBase, normalizedAdminPath("/admin"))
}

// TODO: Document buildSiteRuntimeMiddlewareWithAdminPath.
func buildSiteRuntimeMiddlewareWithAdminPath(
	resolver siteRuntimeResolver,
	runtimeStorage siteRuntimeStorage,
	defaultHostBase string,
	adminPath string,
) func(http.Handler) http.Handler {
	normalizedHostBase := normalizeRouteHostname(defaultHostBase)
	resolvedAdminPath := normalizedAdminPath(adminPath)
	if resolver == nil || runtimeStorage == nil || normalizedHostBase == "" {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldBypassSiteRuntime(r.URL.Path, resolvedAdminPath, r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			runtimeSite, found, err := resolveRuntimeSite(r.Context(), r, resolver, normalizedHostBase)
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "failed to resolve runtime site")
				return
			}
			if !found {
				next.ServeHTTP(w, r)
				return
			}

			handled, serveErr := serveRuntimeSiteRequest(w, r, runtimeStorage, runtimeSite)
			if serveErr != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "failed to serve site content")
				return
			}
			if handled {
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TODO: Document shouldBypassSiteRuntime.
func shouldBypassSiteRuntime(requestPath, adminPath, method string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return true
	}

	normalizedPath := strings.TrimSpace(requestPath)
	if normalizedPath == "" {
		normalizedPath = "/"
	}

	if pathIsOrWithin(normalizedPath, "/api") ||
		pathIsOrWithin(normalizedPath, adminPath) ||
		normalizedPath == "/health" ||
		normalizedPath == "/favicon.ico" {
		return true
	}
	return false
}

func pathIsOrWithin(requestPath, prefix string) bool {
	if prefix == "" || prefix == "/" {
		return requestPath == "/"
	}
	if requestPath == prefix {
		return true
	}
	return strings.HasPrefix(requestPath, prefix+"/")
}

// TODO: Document resolveRuntimeSite.
func resolveRuntimeSite(ctx context.Context, r *http.Request, resolver siteRuntimeResolver, defaultHostBase string) (*sites.RuntimeSite, bool, error) {
	if routeEntry, ok := CustomDomainRouteFromContext(r.Context()); ok && strings.TrimSpace(routeEntry.DomainID) != "" {
		runtimeSite, err := resolver.ResolveRuntimeSiteByCustomDomainID(ctx, routeEntry.DomainID)
		if err != nil {
			if errors.Is(err, sites.ErrSiteNotFound) || errors.Is(err, sites.ErrNoLiveDeploy) {
				return nil, false, nil
			}
			return nil, false, err
		}
		return runtimeSite, true, nil
	}

	hostSlug, ok := slugFromDerivedHost(normalizeRequestHost(r.Host), defaultHostBase)
	if !ok {
		return nil, false, nil
	}

	runtimeSite, err := resolver.ResolveRuntimeSiteBySlug(ctx, hostSlug)
	if err != nil {
		if errors.Is(err, sites.ErrSiteNotFound) || errors.Is(err, sites.ErrNoLiveDeploy) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return runtimeSite, true, nil
}

func normalizeRequestHost(rawHost string) string {
	host := strings.TrimSpace(rawHost)
	if host != "" {
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			host = parsedHost
		}
	}
	return strings.ToLower(host)
}

// TODO: Document slugFromDerivedHost.
func slugFromDerivedHost(host, hostBase string) (string, bool) {
	host = normalizeRouteHostname(host)
	hostBase = normalizeRouteHostname(hostBase)
	if host == "" || hostBase == "" || host == hostBase {
		return "", false
	}

	suffix := "." + hostBase
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}

	slug := strings.TrimSuffix(host, suffix)
	if slug == "" || strings.Contains(slug, ".") {
		return "", false
	}
	return slug, true
}

// TODO: Document configuredSiteRuntimeHostBase.
func configuredSiteRuntimeHostBase(cfg *config.Config) string {
	if cfg == nil {
		return "localhost"
	}

	publicBaseURL := strings.TrimSpace(cfg.PublicBaseURL())
	if publicBaseURL == "" {
		return "localhost"
	}

	parsedURL, err := url.Parse(publicBaseURL)
	if err != nil {
		return "localhost"
	}

	host := normalizeRequestHost(parsedURL.Host)
	if host == "" {
		return "localhost"
	}
	switch host {
	case "127.0.0.1", "::1":
		return "localhost"
	}
	return host
}

// TODO: Document serveRuntimeSiteRequest.
func serveRuntimeSiteRequest(w http.ResponseWriter, r *http.Request, runtimeStorage siteRuntimeStorage, runtimeSite *sites.RuntimeSite) (bool, error) {
	requestObjectPath, ok := normalizeRuntimeObjectPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return true, nil
	}

	served, err := serveRuntimeObject(r.Context(), w, runtimeStorage, runtimeSite.SiteID, runtimeSite.LiveDeployID, requestObjectPath)
	if err != nil {
		return true, err
	}
	if served {
		return true, nil
	}

	if !runtimeSite.SPAMode {
		http.NotFound(w, r)
		return true, nil
	}
	if requestObjectPath == "index.html" {
		http.NotFound(w, r)
		return true, nil
	}

	served, err = serveRuntimeObject(r.Context(), w, runtimeStorage, runtimeSite.SiteID, runtimeSite.LiveDeployID, "index.html")
	if err != nil {
		return true, err
	}
	if served {
		return true, nil
	}

	http.NotFound(w, r)
	return true, nil
}

// TODO: Document serveRuntimeObject.
func serveRuntimeObject(ctx context.Context, w http.ResponseWriter, runtimeStorage siteRuntimeStorage, siteID, deployID, relativeObjectPath string) (bool, error) {
	objectName, err := runtimeObjectName(siteID, deployID, relativeObjectPath)
	if err != nil {
		return false, nil
	}

	body, object, err := runtimeStorage.Download(ctx, sitesStorageBucketName, objectName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	defer body.Close()

	setRuntimeCacheHeader(w, relativeObjectPath)
	setRuntimeContentType(w, object, relativeObjectPath)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
	return true, nil
}

func normalizeRuntimeObjectPath(requestPath string) (string, bool) {
	cleanCandidate := strings.TrimSpace(strings.ReplaceAll(strings.TrimPrefix(requestPath, "/"), "\\", "/"))
	if cleanCandidate == "" {
		return "index.html", true
	}

	normalized := path.Clean(cleanCandidate)
	if normalized == "." {
		return "index.html", true
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", false
	}
	return normalized, true
}

func runtimeObjectName(siteID, deployID, relativeObjectPath string) (string, error) {
	if relativeObjectPath == "" {
		return "", errors.New("relative object path is required")
	}

	deployPrefix := path.Join("sites", siteID, deployID)
	objectName := path.Join(deployPrefix, relativeObjectPath)
	if !strings.HasPrefix(objectName, deployPrefix+"/") {
		return "", errors.New("runtime object path must stay in deploy prefix")
	}
	return objectName, nil
}

func setRuntimeCacheHeader(w http.ResponseWriter, relativeObjectPath string) {
	if relativeObjectPath == "index.html" {
		w.Header().Set("Cache-Control", "private, no-cache")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=1209600")
}

func setRuntimeContentType(w http.ResponseWriter, object *storage.Object, relativeObjectPath string) {
	if object != nil && object.ContentType != "" {
		w.Header().Set("Content-Type", object.ContentType)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(relativeObjectPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
}
