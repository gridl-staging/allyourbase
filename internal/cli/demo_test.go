package cli

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/allyourbase/ayb/examples"
)

func TestDemoCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "demo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'demo' subcommand to be registered")
	}
}

func TestDemoRegistryComplete(t *testing.T) {
	expected := map[string]int{
		"kanban":     5173,
		"live-polls": 5175,
	}
	for name, port := range expected {
		demo, ok := demoRegistry[name]
		if !ok {
			t.Errorf("demo %q not found in registry", name)
			continue
		}
		if demo.Port != port {
			t.Errorf("demo %q: expected port %d, got %d", name, port, demo.Port)
		}
		if demo.Title == "" {
			t.Errorf("demo %q: title is empty", name)
		}
		if len(demo.TrySteps) == 0 {
			t.Errorf("demo %q: no try steps", name)
		}
	}
	if len(demoRegistry) != len(expected) {
		t.Errorf("expected %d demos, got %d", len(expected), len(demoRegistry))
	}
}

func TestDemoUnknownName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"demo", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown demo name")
	}
	if !strings.Contains(err.Error(), "unknown demo") {
		t.Fatalf("expected 'unknown demo' error, got %q", err.Error())
	}
}

func TestDemoRequiresName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"demo"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing demo name")
	}
}

func TestEmbeddedDemoFSContainsSchemas(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
		if err != nil {
			t.Errorf("reading embedded %s/schema.sql: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded %s/schema.sql is empty", name)
		}
		if !strings.Contains(string(data), "CREATE TABLE") {
			t.Errorf("embedded %s/schema.sql doesn't contain CREATE TABLE", name)
		}
	}
}

func TestDemoRegistryNameConsistency(t *testing.T) {
	for key, demo := range demoRegistry {
		if key != demo.Name {
			t.Errorf("registry key %q != demo.Name %q", key, demo.Name)
		}
	}
}

func TestDemoRegistryDescriptionNonEmpty(t *testing.T) {
	for name, demo := range demoRegistry {
		if demo.Description == "" {
			t.Errorf("demo %q: Description is empty", name)
		}
	}
}

func TestDemoValidArgsMatchRegistry(t *testing.T) {
	validArgs := demoCmd.ValidArgs
	if len(validArgs) != len(demoRegistry) {
		t.Fatalf("ValidArgs has %d entries but registry has %d", len(validArgs), len(demoRegistry))
	}
	for _, arg := range validArgs {
		if _, ok := demoRegistry[arg]; !ok {
			t.Errorf("ValidArg %q not found in demoRegistry", arg)
		}
	}
}

// TestEmbeddedSchemasUseCorrectRLSKey verifies all demo schemas reference
// the 'ayb.user_id' session variable that the AYB server actually sets
// (via SET LOCAL in internal/auth/rls.go). Any reference to 'request.jwt.sub'
// would silently break RLS policies and RPC functions at runtime.
func TestEmbeddedSchemasUseCorrectRLSKey(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
		if err != nil {
			t.Fatalf("reading embedded %s/schema.sql: %v", name, err)
		}
		content := string(data)

		// Must use the key the server sets
		if !strings.Contains(content, "ayb.user_id") {
			t.Errorf("%s/schema.sql: does not reference 'ayb.user_id' — RLS policies won't work", name)
		}

		// Must NOT use the wrong key
		if strings.Contains(content, "request.jwt.sub") {
			t.Errorf("%s/schema.sql: contains 'request.jwt.sub' instead of 'ayb.user_id' — server sets ayb.user_id", name)
		}
	}
}

// TestEmbeddedSchemasHaveRLS verifies every demo schema enables row-level security.
func TestEmbeddedSchemasHaveRLS(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
		if err != nil {
			t.Fatalf("reading embedded %s/schema.sql: %v", name, err)
		}
		content := string(data)
		if !strings.Contains(content, "ENABLE ROW LEVEL SECURITY") {
			t.Errorf("%s/schema.sql: does not enable RLS", name)
		}
		if !strings.Contains(content, "CREATE POLICY") {
			t.Errorf("%s/schema.sql: does not create any RLS policies", name)
		}
	}
}

// TestDemoDistContainsIndexHTML verifies each demo's dist/ has an index.html.
func TestDemoDistContainsIndexHTML(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		distFS, err := examples.DemoDist(name)
		if err != nil {
			t.Fatalf("DemoDist(%q): %v", name, err)
		}
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			t.Errorf("demo %q: dist/index.html not found: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("demo %q: dist/index.html is empty", name)
		}
	}
}

// TestDemoDistContainsAssets verifies each demo's dist/ has at least one JS and CSS file.
func TestDemoDistContainsAssets(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		distFS, err := examples.DemoDist(name)
		if err != nil {
			t.Fatalf("DemoDist(%q): %v", name, err)
		}

		var hasJS, hasCSS bool
		fs.WalkDir(distFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, ".js") {
				hasJS = true
			}
			if strings.HasSuffix(path, ".css") {
				hasCSS = true
			}
			return nil
		})

		if !hasJS {
			t.Errorf("demo %q: dist/ has no .js files", name)
		}
		if !hasCSS {
			t.Errorf("demo %q: dist/ has no .css files", name)
		}
	}
}

// ---------------------------------------------------------------------------
// demoFileHandler / serveDemoFile unit tests
//
// These test the Go HTTP server that replaced Vite dev. The handler serves
// pre-built static files from an fs.FS and falls back to index.html for
// client-side routing (SPA behavior).
// ---------------------------------------------------------------------------

// testDistFS creates an in-memory filesystem mimicking a Vite build output.
func testDistFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<html><body>SPA</body></html>")},
		"assets/index-abc123.js":  {Data: []byte("console.log('app')")},
		"assets/index-abc123.css": {Data: []byte("body{margin:0}")},
		"assets/logo.svg":         {Data: []byte("<svg></svg>")},
		"favicon.ico":             {Data: []byte("icon")},
	}
}

func TestDemoFileHandler_RootServesIndexHTML(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SPA") {
		t.Error("root path did not serve index.html content")
	}
}

func TestDemoFileHandler_ExactFileServed(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "console.log") {
		t.Error("JS file content not served")
	}
}

func TestDemoFileHandler_SPAFallbackForUnknownPath(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	// Client-side route like /polls/123 should fall back to index.html.
	req := httptest.NewRequest("GET", "/polls/123", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SPA") {
		t.Error("SPA fallback did not serve index.html for unknown path")
	}
}

func TestDemoFileHandler_CSSServedWithCorrectType(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.css", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected Content-Type to contain text/css, got %q", ct)
	}
}

func TestDemoFileHandler_JSServedWithCorrectType(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header for .js file, got empty")
	}
}

func TestDemoFileHandler_StaticAssetsCached(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "max-age=") {
		t.Errorf("expected Cache-Control with max-age for static asset, got %q", cc)
	}
}

func TestDemoFileHandler_IndexHTMLNotCached(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "" {
		t.Errorf("index.html should not have Cache-Control, got %q", cc)
	}
}

func TestServeDemoFile_ReturnsFalseForMissingFile(t *testing.T) {
	w := httptest.NewRecorder()
	ok := serveDemoFile(w, testDistFS(), "nonexistent.txt")
	if ok {
		t.Error("expected false for missing file")
	}
}

func TestServeDemoFile_ReturnsFalseForDirectory(t *testing.T) {
	w := httptest.NewRecorder()
	ok := serveDemoFile(w, testDistFS(), "assets")
	if ok {
		t.Error("expected false for directory path")
	}
}

func TestDemoFileHandler_FaviconServed(t *testing.T) {
	handler := demoFileHandler(testDistFS())
	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for favicon, got %d", w.Code)
	}
	if w.Body.String() != "icon" {
		t.Error("favicon content not served correctly")
	}
}

// TestDemoFileHandler_WithRealDemoDist verifies the handler works with the
// actual embedded demo dist/ filesystem, not just the test fixture.
func TestDemoFileHandler_WithRealDemoDist(t *testing.T) {
	for _, name := range []string{"kanban", "live-polls"} {
		t.Run(name, func(t *testing.T) {
			distFS, err := examples.DemoDist(name)
			if err != nil {
				t.Fatalf("DemoDist(%q): %v", name, err)
			}
			handler := demoFileHandler(distFS)

			// Root should serve index.html.
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			handler(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("root: expected 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), "<html") && !strings.Contains(w.Body.String(), "<!doctype") && !strings.Contains(w.Body.String(), "<!DOCTYPE") {
				t.Errorf("root: response doesn't look like HTML: %s", w.Body.String()[:min(100, w.Body.Len())])
			}

			// SPA fallback for unknown route.
			req2 := httptest.NewRequest("GET", "/some/deep/route", nil)
			w2 := httptest.NewRecorder()
			handler(w2, req2)
			if w2.Code != http.StatusOK {
				t.Errorf("SPA fallback: expected 200, got %d", w2.Code)
			}
		})
	}
}

// TestEmbeddedSchemasHaveCHECKConstraints verifies every demo schema has CHECK
// constraints on critical columns to prevent invalid data at the database level.
// Since there is no manual QA, CHECK constraints are the last line of defense.
func TestEmbeddedSchemasHaveCHECKConstraints(t *testing.T) {
	checks := map[string][]string{
		"live-polls": {"CHECK (length(question) > 0)", "CHECK (length(label) > 0)", "CHECK (position >= 0)"},
		"kanban":     {"CHECK (length(title) > 0)", "CHECK (position >= 0)"},
	}
	for name, expected := range checks {
		data, err := fs.ReadFile(examples.FS, name+"/schema.sql")
		if err != nil {
			t.Fatalf("reading embedded %s/schema.sql: %v", name, err)
		}
		content := string(data)
		for _, check := range expected {
			if !strings.Contains(content, check) {
				t.Errorf("%s/schema.sql: missing CHECK constraint %q", name, check)
			}
		}
	}
}

// TestDemoTryStepsContainCorrectPort verifies that TrySteps URLs match the registered port.
func TestDemoTryStepsContainCorrectPort(t *testing.T) {
	for name, demo := range demoRegistry {
		portStr := fmt.Sprintf("localhost:%d", demo.Port)
		found := false
		for _, step := range demo.TrySteps {
			if strings.Contains(step, portStr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("demo %q: TrySteps don't reference port %d", name, demo.Port)
		}
	}
}

// TestResolveDemoAdminTokenMissingFile verifies that when the admin-token file
// doesn't exist, the error message gives actionable instructions (not a cryptic
// "admin-token not found"). This was a bug fix in session 186.
func TestResolveDemoAdminTokenMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("AYB_ADMIN_TOKEN", "") // ensure env var doesn't short-circuit

	_, err := resolveDemoAdminToken("http://127.0.0.1:8090")
	if err == nil {
		t.Fatal("expected error when admin-token file missing")
	}
	msg := err.Error()
	// Should contain actionable instructions, not just "file not found".
	if !strings.Contains(msg, "ayb stop") {
		t.Errorf("error message should suggest 'ayb stop', got: %s", msg)
	}
	if !strings.Contains(msg, "ayb demo") {
		t.Errorf("error message should suggest 'ayb demo', got: %s", msg)
	}
}

// TestResolveDemoAdminTokenFromEnv verifies the AYB_ADMIN_TOKEN env var
// short-circuits file lookup.
func TestResolveDemoAdminTokenFromEnv(t *testing.T) {
	t.Setenv("AYB_ADMIN_TOKEN", "test-token-from-env")

	token, err := resolveDemoAdminToken("http://127.0.0.1:8090")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-token-from-env" {
		t.Errorf("expected token from env, got %q", token)
	}
}
