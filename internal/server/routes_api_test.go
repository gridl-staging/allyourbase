package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func apiRoutesContentTypeTestServer() *Server {
	holder := schema.NewCacheHolder(nil, nil)
	holder.SetForTesting(apiRoutesContentTypeSchema())

	s := &Server{
		cfg:    config.Default(),
		schema: holder,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		pool:   new(pgxpool.Pool),
	}

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		s.registerAPIRoutes(r)
	})
	s.router = r
	return s
}

func apiRoutesContentTypeSchema() *schema.SchemaCache {
	return &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.users": {
				Schema: "public",
				Name:   "users",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "integer", IsPrimaryKey: true},
					{Name: "email", TypeName: "text"},
				},
				PrimaryKey: []string{"id"},
			},
		},
		Functions: map[string]*schema.Function{},
		Schemas:   []string{"public"},
	}
}

func TestRegisterAPIRoutes_NonImportCRUDRejectsCSV(t *testing.T) {
	t.Parallel()
	s := apiRoutesContentTypeTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users", strings.NewReader("id,email\n1,a@b.com\n"))
	req.Header.Set("Content-Type", "text/csv")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestRegisterAPIRoutes_ImportAcceptsCSV(t *testing.T) {
	t.Parallel()
	s := apiRoutesContentTypeTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/import", strings.NewReader("bogus\nvalue\n"))
	req.Header.Set("Content-Type", "text/csv")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "no recognized columns") {
		t.Fatalf("body = %q, want no recognized columns error", w.Body.String())
	}
}
