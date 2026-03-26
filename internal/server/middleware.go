// Package server Provides HTTP middleware for request logging, CORS, security headers, rate limiting, IP allowlisting, and serves the embedded admin SPA with path rewriting.
package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/logging"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/ui"
	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger returns middleware that logs each request as structured JSON.
func requestLogger(loggerProvider func() *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := loggerProvider()
			if logger == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}
				fields := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"duration_ms", time.Since(start).Milliseconds(),
					"bytes", ww.BytesWritten(),
					"request_id", middleware.GetReqID(r.Context()),
					"remote", r.RemoteAddr,
				}
				// Include tenant_id in logs when tenant context or request tenant source is present.
				if tenantID := tenantIDFromContextOrRequest(r); tenantID != "" {
					fields = append(fields, "tenant_id", tenantID)
				}
				for k, v := range observability.TraceLogFields(r.Context()) {
					fields = append(fields, k, v)
				}
				logger.Info("request", fields...)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// wrapLoggerForDrainFanout returns a logger that sends every record to the log drain manager,
// while preserving the existing handler behavior.
func wrapLoggerForDrainFanout(base *slog.Logger, manager *logging.DrainManager) *slog.Logger {
	if base == nil || manager == nil {
		return base
	}
	return slog.New(&drainSlogHandler{next: base.Handler(), drainManager: manager})
}

// drainSlogHandler forwards slog records to an external drain manager.
type drainSlogHandler struct {
	next         slog.Handler
	drainManager *logging.DrainManager
	preAttrs     []slog.Attr
	groupPrefix  string
}

func (h *drainSlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle collects attributes from the slog record, applies group prefix namespacing, enqueues the resulting entry to the drain manager, and forwards the record to the next handler in the chain.
func (h *drainSlogHandler) Handle(ctx context.Context, record slog.Record) error {
	fields := make(map[string]any)
	// Include pre-set attrs from WithAttrs calls (e.g. slog.With("component", "auth")).
	for _, a := range h.preAttrs {
		key := a.Key
		if h.groupPrefix != "" {
			key = h.groupPrefix + "." + key
		}
		fields[key] = a.Value.Resolve().Any()
	}
	record.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if h.groupPrefix != "" {
			key = h.groupPrefix + "." + key
		}
		fields[key] = a.Value.Resolve().Any()
		return true
	})

	h.drainManager.Enqueue(logging.LogEntry{
		Timestamp: record.Time,
		Level:     strings.ToLower(record.Level.String()),
		Message:   record.Message,
		Source:    "app",
		Fields:    fields,
	})

	return h.next.Handle(ctx, record)
}

func (h *drainSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.preAttrs)+len(attrs))
	copy(combined, h.preAttrs)
	copy(combined[len(h.preAttrs):], attrs)
	return &drainSlogHandler{
		next:         h.next.WithAttrs(attrs),
		drainManager: h.drainManager,
		preAttrs:     combined,
		groupPrefix:  h.groupPrefix,
	}
}

func (h *drainSlogHandler) WithGroup(name string) slog.Handler {
	prefix := name
	if h.groupPrefix != "" {
		prefix = h.groupPrefix + "." + name
	}
	return &drainSlogHandler{
		next:         h.next.WithGroup(name),
		drainManager: h.drainManager,
		preAttrs:     h.preAttrs,
		groupPrefix:  prefix,
	}
}

// staticSPAHandler serves the embedded admin SPA with index.html fallback
// for client-side routing support. Files are served directly from the
// embedded FS to avoid http.FileServer's index.html redirect behavior.
func staticSPAHandler(adminPath string) http.HandlerFunc {
	adminPath = normalizedAdminPath(adminPath)
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, adminPath)
		path = strings.TrimPrefix(path, "/")

		// Explicit index requests should get rewritten SPA HTML.
		if path == "" || path == "index.html" {
			serveEmbeddedIndexHTML(w, adminPath)
			return
		}

		// Try exact file; fall back to index.html for SPA routing.
		if !serveEmbeddedFile(w, path, false) {
			serveEmbeddedIndexHTML(w, adminPath)
		}
	}
}

// serveEmbeddedFile writes a file from the embedded UI FS to w.
// Returns false if the file doesn't exist (caller should fall back).
func serveEmbeddedFile(w http.ResponseWriter, path string, mustExist bool) bool {
	f, err := ui.DistDirFS.Open(path)
	if err != nil {
		if mustExist {
			http.Error(w, "not found", http.StatusNotFound)
		}
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		if mustExist {
			http.Error(w, "not found", http.StatusNotFound)
		}
		return false
	}

	// Cache static assets (not index.html).
	if path != "index.html" {
		w.Header().Set("Cache-Control", "public, max-age=1209600")
	}
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
	return true
}

// serveEmbeddedIndexHTML reads the embedded index.html file, rewrites its paths with rewriteAdminIndexHTML, and writes the result to w with appropriate HTTP headers.
func serveEmbeddedIndexHTML(w http.ResponseWriter, adminPath string) {
	f, err := ui.DistDirFS.Open("index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if ct := mime.TypeByExtension(".html"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, rewriteAdminIndexHTML(string(raw), adminPath))
}

// rewriteAdminIndexHTML modifies HTML by rewriting relative asset and admin paths to be prefixed with adminPath, enabling the embedded SPA to serve correctly from a non-root URL.
func rewriteAdminIndexHTML(html string, adminPath string) string {
	adminBase := adminPathWithTrailingSlash(adminPath)
	replacer := strings.NewReplacer(
		`="/assets/`, `="`+adminBase+`assets/`,
		`='/assets/`, `='`+adminBase+`assets/`,
		`="/admin/`, `="`+adminBase,
		`='/admin/`, `='`+adminBase,
		`url(/assets/`, `url(`+adminBase+`assets/`,
		`url('/assets/`, `url('`+adminBase+`assets/`,
		`url("/assets/`, `url("`+adminBase+`assets/`,
		`url(/admin/`, `url(`+adminBase,
		`url('/admin/`, `url('`+adminBase,
		`url("/admin/`, `url("`+adminBase,
	)
	return replacer.Replace(html)
}

// corsMiddleware returns middleware that sets CORS headers.
// Per the spec, Access-Control-Allow-Origin must be either "*" or a single
// origin. When multiple origins are configured, the middleware echoes back
// only the matching origin and adds Vary: Origin so caches key correctly.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	wildcard := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if wildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				if _, ok := originSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions && !passesThroughCORSPreflight(r) {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func passesThroughCORSPreflight(r *http.Request) bool {
	return r != nil && strings.TrimSpace(r.Header.Get("Tus-Resumable")) != ""
}

// securityHeaders adds standard security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// requestLogMiddleware records each request as a RequestLogEntry via the async RequestLogger.
func requestLogMiddleware(rl *RequestLogger, drainManagerProvider func() *logging.DrainManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			entry := RequestLogEntry{
				Method:       r.Method,
				Path:         r.URL.Path,
				StatusCode:   status,
				DurationMS:   time.Since(start).Milliseconds(),
				RequestSize:  normalizedRequestSize(r.ContentLength),
				ResponseSize: int64(ww.BytesWritten()),
				RequestID:    middleware.GetReqID(r.Context()),
			}

			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				entry.IPAddress = host
			} else {
				entry.IPAddress = r.RemoteAddr
			}

			if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
				entry.UserID = claims.Subject
				entry.APIKeyID = claims.APIKeyID
			}

			// Include tenant_id in request logs when tenant context or request tenant source is present.
			if tenantID := tenantIDFromContextOrRequest(r); tenantID != "" {
				entry.TenantID = tenantID
			}

			if rl != nil {
				rl.Log(entry)
			}
			if dm := drainManagerProvider(); dm != nil {
				drainEntry := logEntryToDrain(entry)
				for k, v := range observability.TraceLogFields(r.Context()) {
					drainEntry.Fields[k] = v
				}
				dm.Enqueue(drainEntry)
			}
		})
	}
}

func normalizedRequestSize(contentLength int64) int64 {
	if contentLength < 0 {
		return 0
	}
	return contentLength
}

// logEntryToDrain converts a RequestLogEntry to a logging.LogEntry, mapping HTTP request metadata and excluding empty optional fields.
func logEntryToDrain(entry RequestLogEntry) logging.LogEntry {
	fields := map[string]any{
		"method":        entry.Method,
		"path":          entry.Path,
		"status":        entry.StatusCode,
		"duration_ms":   entry.DurationMS,
		"request_size":  entry.RequestSize,
		"response_size": entry.ResponseSize,
	}
	if entry.UserID != "" {
		fields["user_id"] = entry.UserID
	}
	if entry.APIKeyID != "" {
		fields["api_key_id"] = entry.APIKeyID
	}
	if entry.RequestID != "" {
		fields["request_id"] = entry.RequestID
	}
	if entry.IPAddress != "" {
		fields["ip_address"] = entry.IPAddress
	}
	if entry.TenantID != "" {
		fields["tenant_id"] = entry.TenantID
	}

	return logging.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     "info",
		Message:   fmt.Sprintf("%s %s", entry.Method, entry.Path),
		Source:    "request",
		Fields:    fields,
	}
}

// --- Rate limiting and IP allowlist middleware ---

func newIPAllowlist(section string, entries []string, logger *slog.Logger) *httputil.IPAllowlist {
	allowlist, err := httputil.NewIPAllowlist(entries)
	if err != nil {
		logger.Error("invalid ip allowlist configuration", "section", section, "error", err)
		return nil
	}
	return allowlist
}

// apiRouteAllowlistMiddleware returns HTTP middleware that applies IP allowlists
// to API routes, using adminAllowlist for paths under /api/admin and
// serverAllowlist for all other API paths.
func apiRouteAllowlistMiddleware(serverAllowlist, adminAllowlist *httputil.IPAllowlist) func(http.Handler) http.Handler {
	serverMiddleware := apiRouteMiddleware(serverAllowlist)
	adminMiddleware := apiRouteMiddleware(adminAllowlist)

	return func(next http.Handler) http.Handler {
		serverHandler := serverMiddleware(next)
		adminHandler := adminMiddleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAdminAPIPath(r.URL.Path) {
				adminHandler.ServeHTTP(w, r)
				return
			}
			serverHandler.ServeHTTP(w, r)
		})
	}
}

func isAdminAPIPath(path string) bool {
	return path == "/api/admin" || strings.HasPrefix(path, "/api/admin/")
}

func apiRouteMiddleware(allowlist *httputil.IPAllowlist) func(http.Handler) http.Handler {
	if allowlist == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return allowlist.Middleware
}

// authRouteRateLimitMiddleware returns HTTP middleware that applies rate limiting
// to authentication routes, using a stricter limiter for sensitive endpoints
// like login and register, and a more lenient limiter for other auth paths.
func authRouteRateLimitMiddleware(general, sensitive *auth.RateLimiter) func(http.Handler) http.Handler {
	if general == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	generalMiddleware := general.Middleware
	sensitiveMiddleware := generalMiddleware
	if sensitive != nil {
		sensitiveMiddleware = sensitive.Middleware
	}

	return func(next http.Handler) http.Handler {
		sensitiveHandler := sensitiveMiddleware(next)
		generalHandler := generalMiddleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSensitiveAuthPath(r.URL.Path) {
				sensitiveHandler.ServeHTTP(w, r)
				return
			}
			generalHandler.ServeHTTP(w, r)
		})
	}
}

func isSensitiveAuthPath(path string) bool {
	switch path {
	case "/api/auth/login", "/api/auth/register", "/api/auth/magic-link", "/api/auth/sms", "/api/auth/sms/confirm":
		return true
	}
	if strings.HasPrefix(path, "/api/auth/mfa/") && strings.HasSuffix(path, "/verify") {
		return true
	}
	return false
}

// APIRouteRateLimitMiddleware returns HTTP middleware that applies per-request
// rate limiting based on authentication status, using the authenticated limiter
// for JWT or API-key bearers and the anonymous limiter for unauthenticated
// requests. Sets X-RateLimit headers in all responses.
func APIRouteRateLimitMiddleware(authenticated, anonymous *auth.RateLimiter, authLimit, anonymousLimit int) func(http.Handler) http.Handler {
	if authenticated == nil && anonymous == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rl := anonymous
			limit := anonymousLimit
			key := httputil.ClientIP(r)
			if claims := auth.ClaimsFromContext(r.Context()); claims != nil && claims.Subject != "" && authenticated != nil {
				rl = authenticated
				limit = authLimit
				key = claims.Subject
			}
			if rl == nil {
				next.ServeHTTP(w, r)
				return
			}

			allowed, remaining, resetTime := rl.Allow(key)
			if !handleRateLimitDecision(w, limit, allowed, remaining, resetTime) {
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TODO: Document handleRateLimitDecision.
func handleRateLimitDecision(w http.ResponseWriter, limit int, allowed bool, remaining int, resetTime time.Time) bool {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	if allowed {
		return true
	}

	retryAfter := int(time.Until(resetTime).Seconds()) + 1
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	httputil.WriteError(w, http.StatusTooManyRequests, "too many requests")
	return false
}
