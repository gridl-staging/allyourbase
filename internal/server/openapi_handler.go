// Package server Provides HTTP handlers for the OpenAPI JSON specification and Swagger UI documentation with ETag-based caching.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/openapi"
	"github.com/allyourbase/ayb/internal/schema"
)

// openapiCache caches the generated OpenAPI JSON spec and its ETag.
// It invalidates automatically when the schema cache's BuiltAt changes.
type openapiCache struct {
	mu      sync.RWMutex
	data    []byte
	etag    string
	builtAt time.Time // tracks which schema version was used
}

func (c *openapiCache) set(data []byte, builtAt time.Time) string {
	h := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(h[:16]) + `"`
	c.mu.Lock()
	c.data = data
	c.etag = etag
	c.builtAt = builtAt
	c.mu.Unlock()
	return etag
}

func (c *openapiCache) get(builtAt time.Time) ([]byte, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data != nil && c.builtAt.Equal(builtAt) {
		return c.data, c.etag, true
	}
	return nil, "", false
}

// handleOpenAPIJSON is an HTTP handler that serves the OpenAPI specification as JSON. It uses ETag-based caching to support conditional requests, returning 304 Not Modified when the client's If-None-Match header matches the current ETag, or 503 Service Unavailable if the schema cache is not ready.
func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	if s.schema == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return
	}

	sc := s.schema.Get()
	if sc == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return
	}

	data, etag, ok := s.openapiJSONCache.get(sc.BuiltAt)
	if !ok {
		data, etag = s.regenerateOpenAPISpec(sc)
	}

	// Conditional 304.
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (s *Server) regenerateOpenAPISpec(sc *schema.SchemaCache) ([]byte, string) {
	data, err := openapi.Generate(sc, openapi.Options{BasePath: "/api"})
	if err != nil {
		s.logger.Error("openapi spec generation failed", "error", err)
		data = []byte(`{"openapi":"3.1.0","info":{"title":"AYB","version":"0.0.0"},"paths":{}}`)
	}
	etag := s.openapiJSONCache.set(data, sc.BuiltAt)
	return data, etag
}

var swaggerUITmpl = template.Must(template.New("swagger").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>AYB API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: "/api/openapi.json",
      dom_id: '#swagger-ui',
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: "BaseLayout"
    });
  </script>
</body>
</html>`))

func handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	swaggerUITmpl.Execute(w, nil)
}
