package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/examples"
)

// serveDemoApp starts a Go HTTP server that serves pre-built static assets
// from the embedded FS and reverse-proxies /api requests to the AYB server.
// Blocks until SIGINT/SIGTERM is received.
func serveDemoApp(name string, port int, aybServerURL string) error {
	distFS, err := examples.DemoDist(name)
	if err != nil {
		return fmt.Errorf("loading demo assets: %w", err)
	}

	target, err := url.Parse(aybServerURL)
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}

	mux := http.NewServeMux()

	// Reverse-proxy /api to the AYB server.
	// FlushInterval: -1 enables continuous flushing, required for SSE (Server-Sent Events).
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded()
		},
		FlushInterval: -1,
	}
	mux.Handle("/api/", proxy)

	// Serve pre-built static files with SPA fallback.
	mux.HandleFunc("/", demoFileHandler(distFS))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Graceful shutdown on signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		signal.Stop(sigCh)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("demo server: %w", err)
	}
	return nil
}

// demoFileHandler returns an http.HandlerFunc that serves files from the given
// FS with SPA index.html fallback for client-side routing.
func demoFileHandler(distFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Clean the path and strip leading slash.
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Try to serve the exact file; fall back to index.html for SPA routing.
		if path == "" || !serveDemoFile(w, distFS, path) {
			serveDemoFile(w, distFS, "index.html")
		}
	}
}

// serveDemoFile writes a file from the demo dist FS to w.
// Returns false if the file doesn't exist (caller should fall back).
func serveDemoFile(w http.ResponseWriter, distFS fs.FS, path string) bool {
	f, err := distFS.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
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
