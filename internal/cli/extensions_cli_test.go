package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetExtensionsFlags() {
	resetJSONFlag()
	rootCmd.PersistentFlags().Set("output", "table")
}

// --- extensions list ---

func TestExtensionsListTable(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/admin/extensions" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"extensions": []map[string]any{
				{"name": "pgvector", "installed": true, "available": true, "installed_version": "0.5.1", "default_version": "0.5.1"},
				{"name": "pg_trgm", "installed": false, "available": true, "default_version": "1.6"},
			},
			"total": 2,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "list",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "pgvector") {
		t.Fatalf("expected pgvector in output, got %q", output)
	}
	if !strings.Contains(output, "pg_trgm") {
		t.Fatalf("expected pg_trgm in output, got %q", output)
	}
}

func TestExtensionsListEmpty(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"extensions": []any{},
			"total":      0,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "list",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No extensions") {
		t.Fatalf("expected empty message, got %q", output)
	}
}

func TestExtensionsListJSON(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"extensions": []map[string]any{
				{"name": "pgvector", "installed": true},
			},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "list", "--json",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"name"`) {
		t.Fatalf("expected JSON output, got %q", output)
	}
}

// --- extensions enable ---

func TestExtensionsEnableSuccess(t *testing.T) {
	resetExtensionsFlags()
	var method, path string
	var reqBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&reqBody)
		json.NewEncoder(w).Encode(map[string]any{"name": "pgvector", "enabled": true})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "enable", "pgvector",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if method != "POST" {
		t.Fatalf("expected POST, got %s", method)
	}
	if path != "/api/admin/extensions" {
		t.Fatalf("expected /api/admin/extensions, got %s", path)
	}
	if reqBody["name"] != "pgvector" {
		t.Fatalf("expected name=pgvector, got %v", reqBody["name"])
	}
	if !strings.Contains(output, "pgvector") || !strings.Contains(output, "enabled") {
		t.Fatalf("expected enable message, got %q", output)
	}
}

func TestExtensionsEnableRequiresArg(t *testing.T) {
	resetExtensionsFlags()
	rootCmd.SetArgs([]string{"extensions", "enable",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

func TestExtensionsEnableServerError(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "extension not available"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"extensions", "enable", "nonexistent",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "extension not available") {
		t.Fatalf("expected not-available error, got %q", err.Error())
	}
}

// --- extensions disable ---

func TestExtensionsDisableSuccess(t *testing.T) {
	resetExtensionsFlags()
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "disable", "pgvector",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if method != "DELETE" {
		t.Fatalf("expected DELETE, got %s", method)
	}
	if path != "/api/admin/extensions/pgvector" {
		t.Fatalf("expected /api/admin/extensions/pgvector, got %s", path)
	}
	if !strings.Contains(output, "pgvector") || !strings.Contains(output, "disabled") {
		t.Fatalf("expected disable message, got %q", output)
	}
}

func TestExtensionsDisableRequiresArg(t *testing.T) {
	resetExtensionsFlags()
	rootCmd.SetArgs([]string{"extensions", "disable",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

func TestExtensionsDisableServerError(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"message": "extension has dependent objects"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"extensions", "disable", "pgvector",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "dependent objects") {
		t.Fatalf("expected dependency error, got %q", err.Error())
	}
}

// --- config persistence ---

func TestExtensionsEnableWithConfig(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"name": "pgvector", "enabled": true})
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ayb.toml")

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "enable", "pgvector",
			"--url", srv.URL, "--admin-token", "tok",
			"--config", cfgPath})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if !strings.Contains(string(data), "pgvector") {
		t.Errorf("expected pgvector in config, got %s", data)
	}
}

func TestExtensionsDisableWithConfig(t *testing.T) {
	resetExtensionsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ayb.toml")
	os.WriteFile(cfgPath, []byte("[managed_pg]\nextensions = [\"pgvector\", \"pg_trgm\"]\n"), 0o600)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"extensions", "disable", "pgvector",
			"--url", srv.URL, "--admin-token", "tok",
			"--config", cfgPath})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if strings.Contains(content, "pgvector") {
		t.Errorf("expected pgvector removed from config, got %s", content)
	}
	if !strings.Contains(content, "pg_trgm") {
		t.Errorf("expected pg_trgm preserved in config, got %s", content)
	}
}

// --- subcommand registration ---

func TestExtensionsSubcommandsRegistered(t *testing.T) {
	resetExtensionsFlags()
	want := map[string]bool{
		"list":    false,
		"enable":  false,
		"disable": false,
	}
	for _, cmd := range extensionsCmd.Commands() {
		if _, ok := want[cmd.Name()]; ok {
			want[cmd.Name()] = true
		}
	}
	for sub, found := range want {
		if !found {
			t.Fatalf("expected '%s' subcommand under 'extensions'", sub)
		}
	}
}
