package server

import (
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/allyourbase/ayb/ui"
	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger returns middleware that logs each request as structured JSON.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"duration_ms", time.Since(start).Milliseconds(),
					"bytes", ww.BytesWritten(),
					"request_id", middleware.GetReqID(r.Context()),
					"remote", r.RemoteAddr,
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// staticSPAHandler serves the embedded admin SPA with index.html fallback
// for client-side routing support. Files are served directly from the
// embedded FS to avoid http.FileServer's index.html redirect behavior.
func staticSPAHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Strip admin prefix to get relative path within dist/.
		path := r.URL.Path
		if idx := strings.LastIndex(path, "/admin/"); idx != -1 {
			path = path[idx+len("/admin/"):]
		}

		// Try exact file; fall back to index.html for SPA routing.
		if path == "" || !serveEmbeddedFile(w, path, false) {
			serveEmbeddedFile(w, "index.html", true)
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

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
