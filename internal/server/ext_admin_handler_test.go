package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/extensions"
	"github.com/go-chi/chi/v5"
)

// --- Fake extension service ---

type fakeExtService struct {
	exts       []extensions.ExtensionInfo
	enableErr  error
	disableErr error
}

func (f *fakeExtService) List(_ context.Context) ([]extensions.ExtensionInfo, error) {
	return f.exts, nil
}

func (f *fakeExtService) Enable(_ context.Context, name string) error {
	return f.enableErr
}

func (f *fakeExtService) Disable(_ context.Context, name string) error {
	return f.disableErr
}

func extTestServer(svc extensionAdmin) *Server {
	s := &Server{extService: svc}
	r := chi.NewRouter()
	r.Route("/admin/extensions", func(r chi.Router) {
		r.Get("/", s.handleAdminExtensionList)
		r.Post("/", s.handleAdminExtensionEnable)
		r.Delete("/{name}", s.handleAdminExtensionDisable)
	})
	s.router = r
	return s
}

// --- List tests ---

func TestAdminExtensionListEmpty(t *testing.T) {
	s := extTestServer(&fakeExtService{})
	req := httptest.NewRequest("GET", "/admin/extensions", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(0) {
		t.Errorf("total = %v; want 0", resp["total"])
	}
}

func TestAdminExtensionListWithItems(t *testing.T) {
	svc := &fakeExtService{exts: []extensions.ExtensionInfo{
		{Name: "pgvector", Installed: true, Available: true, InstalledVersion: "0.7.0"},
		{Name: "pg_trgm", Available: true},
	}}
	s := extTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/extensions", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(2) {
		t.Errorf("total = %v; want 2", resp["total"])
	}
}

func TestAdminExtensionListNilService(t *testing.T) {
	s := extTestServer(nil)
	req := httptest.NewRequest("GET", "/admin/extensions", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
}

// --- Enable tests ---

func TestAdminExtensionEnableSuccess(t *testing.T) {
	s := extTestServer(&fakeExtService{})
	body := `{"name":"pgvector"}`
	req := httptest.NewRequest("POST", "/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "pgvector" || resp["enabled"] != true {
		t.Errorf("resp = %v", resp)
	}
}

func TestAdminExtensionEnableMissingName(t *testing.T) {
	s := extTestServer(&fakeExtService{})
	body := `{"name":""}`
	req := httptest.NewRequest("POST", "/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

func TestAdminExtensionEnableNotAvailable(t *testing.T) {
	svc := &fakeExtService{enableErr: fmt.Errorf(`extension "bad" is not available on this server`)}
	s := extTestServer(svc)
	body := `{"name":"bad"}`
	req := httptest.NewRequest("POST", "/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestAdminExtensionEnableInvalidName(t *testing.T) {
	svc := &fakeExtService{enableErr: fmt.Errorf(`extension name "bad name" contains invalid characters: must start with a letter`)}
	s := extTestServer(svc)
	body := `{"name":"bad name"}`
	req := httptest.NewRequest("POST", "/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

func TestAdminExtensionEnableNilService(t *testing.T) {
	s := extTestServer(nil)
	body := `{"name":"pgvector"}`
	req := httptest.NewRequest("POST", "/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

// --- Disable tests ---

func TestAdminExtensionDisableSuccess(t *testing.T) {
	s := extTestServer(&fakeExtService{})
	req := httptest.NewRequest("DELETE", "/admin/extensions/pgvector", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", w.Code)
	}
}

func TestAdminExtensionDisableDependency(t *testing.T) {
	svc := &fakeExtService{disableErr: fmt.Errorf(`extension "pgvector" has dependent objects; use cascade`)}
	s := extTestServer(svc)
	req := httptest.NewRequest("DELETE", "/admin/extensions/pgvector", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d; want 409; body = %s", w.Code, w.Body.String())
	}
}

func TestAdminExtensionDisableNilService(t *testing.T) {
	s := extTestServer(nil)
	req := httptest.NewRequest("DELETE", "/admin/extensions/pgvector", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}
