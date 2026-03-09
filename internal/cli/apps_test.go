package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppsCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "apps" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'apps' subcommand to be registered")
	}
}

func TestAppsListTable(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/apps" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":                     "11111111-1111-1111-1111-111111111111",
					"name":                   "Test App",
					"description":            "A test app",
					"ownerUserId":            "22222222-2222-2222-2222-222222222222",
					"rateLimitRps":           100,
					"rateLimitWindowSeconds": 60,
					"createdAt":              "2026-02-21T00:00:00Z",
				},
			},
			"totalItems": 1,
			"page":       1,
			"perPage":    20,
			"totalPages": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "list", "--url", srv.URL, "--admin-token", "test-token"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Test App") {
		t.Fatalf("expected app name in output, got %q", output)
	}
	if !strings.Contains(output, "11111111-1111-1111-1111-111111111111") {
		t.Fatalf("expected app ID in output, got %q", output)
	}
	if !strings.Contains(output, "100 req/60s") {
		t.Fatalf("expected rate limit in output, got %q", output)
	}
	if !strings.Contains(output, "1 app(s)") {
		t.Fatalf("expected total count in output, got %q", output)
	}
}

func TestAppsListJSON(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"totalItems":0}`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "list", "--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"totalItems":0`) {
		t.Fatalf("expected JSON output, got %q", output)
	}
}

func TestAppsListEmpty(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items":      []any{},
			"totalItems": 0,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No apps registered") {
		t.Fatalf("expected empty message, got %q", output)
	}
}

func TestAppsListServerError(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"message": "db down"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"apps", "list", "--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for server failure")
	}
	if !strings.Contains(err.Error(), "db down") {
		t.Fatalf("expected error message, got %q", err.Error())
	}
}

func TestAppsCreateSuccess(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":   "33333333-3333-3333-3333-333333333333",
			"name": "My App",
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "create", "My App",
			"--owner-id", "22222222-2222-2222-2222-222222222222",
			"--description", "cool app",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "App created") {
		t.Fatalf("expected success message, got %q", output)
	}
	if !strings.Contains(output, "33333333-3333-3333-3333-333333333333") {
		t.Fatalf("expected app ID in output, got %q", output)
	}
	if receivedBody["name"] != "My App" {
		t.Fatalf("expected name in request body, got %v", receivedBody["name"])
	}
	if receivedBody["ownerUserId"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("expected ownerUserId in request body, got %v", receivedBody["ownerUserId"])
	}
	if receivedBody["description"] != "cool app" {
		t.Fatalf("expected description in request body, got %v", receivedBody["description"])
	}
}

func TestAppsCreateMissingOwnerID(t *testing.T) {
	resetJSONFlag()
	appsCreateCmd.Flags().Set("owner-id", "")
	appsCreateCmd.Flags().Set("description", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached when owner-id is missing")
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"apps", "create", "My App", "--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing owner-id")
	}
	if !strings.Contains(err.Error(), "--owner-id is required") {
		t.Fatalf("expected owner-id error, got %q", err.Error())
	}
}

func TestAppsCreateRequiresName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"apps", "create", "--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing name argument")
	}
}

func TestAppsCreateJSON(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"44444444-4444-4444-4444-444444444444","name":"JSON App"}`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "create", "JSON App",
			"--owner-id", "55555555-5555-5555-5555-555555555555",
			"--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"id":"44444444-4444-4444-4444-444444444444"`) {
		t.Fatalf("expected JSON output with id, got %q", output)
	}
}

func TestAppsDeleteSuccess(t *testing.T) {
	resetJSONFlag()
	var deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		deletedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "delete", "66666666-6666-6666-6666-666666666666",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if deletedPath != "/api/admin/apps/66666666-6666-6666-6666-666666666666" {
		t.Fatalf("expected delete path, got %q", deletedPath)
	}
	if !strings.Contains(output, "deleted") {
		t.Fatalf("expected delete confirmation, got %q", output)
	}
}

func TestAppsDeleteNotFound(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"message": "app not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"apps", "delete", "nonexistent",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "app not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestAppsDeleteRequiresArg(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"apps", "delete", "--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	// cobra returns error for ExactArgs(1) violation
	if err == nil {
		t.Fatal("expected error for missing ID arg")
	}
}

func TestAppsListCSV(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":                     "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					"name":                   "CSV App",
					"description":            "test",
					"ownerUserId":            "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
					"rateLimitRps":           0,
					"rateLimitWindowSeconds": 0,
					"createdAt":              "2026-02-21T00:00:00Z",
				},
			},
			"totalItems": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "list", "--url", srv.URL, "--admin-token", "tok", "--output", "csv"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "CSV App") {
		t.Fatalf("expected CSV output with app name, got %q", output)
	}
	if !strings.Contains(output, "none") {
		t.Fatalf("expected 'none' rate limit for zero values, got %q", output)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header+data), got %d", len(lines))
	}
}

func TestAppsListAuthSendsToken(t *testing.T) {
	resetJSONFlag()
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "totalItems": 0})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apps", "list", "--url", srv.URL, "--admin-token", "my-secret-token"})
		rootCmd.Execute()
	})

	if receivedAuth != "Bearer my-secret-token" {
		t.Fatalf("expected Bearer token, got %q", receivedAuth)
	}
}

// Test for apikeys list showing app association
func TestAPIKeysListShowsAppColumn(t *testing.T) {
	resetJSONFlag()
	appID := "99999999-9999-9999-9999-999999999999"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":            "11111111-1111-1111-1111-111111111111",
					"userId":        "22222222-2222-2222-2222-222222222222",
					"name":          "App Key",
					"keyPrefix":     "ayb_abc12345",
					"scope":         "readwrite",
					"allowedTables": []string{"posts"},
					"appId":         appID,
					"createdAt":     "2026-02-21T00:00:00Z",
				},
				{
					"id":            "33333333-3333-3333-3333-333333333333",
					"userId":        "22222222-2222-2222-2222-222222222222",
					"name":          "Legacy Key",
					"keyPrefix":     "ayb_def67890",
					"scope":         "*",
					"allowedTables": []string{},
					"appId":         nil,
					"createdAt":     "2026-02-21T00:00:00Z",
				},
			},
			"totalItems": 2,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apikeys", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// App-scoped key should show the app ID.
	if !strings.Contains(output, appID) {
		t.Fatalf("expected app ID %s in output, got %q", appID, output)
	}
	// Legacy key should show "-" for app column.
	if !strings.Contains(output, "-") {
		t.Fatalf("expected '-' for legacy key app column, got %q", output)
	}
	// Header should include "App" column.
	if !strings.Contains(output, "App") {
		t.Fatalf("expected 'App' column header, got %q", output)
	}
}

// Test for apikeys create --app flag
func TestAPIKeysCreateWithAppFlag(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"key": "ayb_test_plaintext_key_00000000000000000000000000000",
			"apiKey": map[string]any{
				"id":    "77777777-7777-7777-7777-777777777777",
				"name":  "app-scoped-key",
				"scope": "*",
			},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apikeys", "create",
			"--user-id", "88888888-8888-8888-8888-888888888888",
			"--name", "app-scoped-key",
			"--app", "99999999-9999-9999-9999-999999999999",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if receivedBody["appId"] != "99999999-9999-9999-9999-999999999999" {
		t.Fatalf("expected appId in request body, got %v", receivedBody["appId"])
	}
	if !strings.Contains(output, "App: 99999999-9999-9999-9999-999999999999") {
		t.Fatalf("expected app ID in output, got %q", output)
	}
}

func TestAPIKeysCreateWithoutAppFlag(t *testing.T) {
	resetJSONFlag()
	// Reset --app flag from any previous test run
	apikeysCreateCmd.Flags().Set("app", "")

	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"key": "ayb_test_plaintext_key_00000000000000000000000000000",
			"apiKey": map[string]any{
				"id":    "77777777-7777-7777-7777-777777777777",
				"name":  "user-key",
				"scope": "*",
			},
		})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"apikeys", "create",
			"--user-id", "88888888-8888-8888-8888-888888888888",
			"--name", "user-key",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if _, hasAppID := receivedBody["appId"]; hasAppID {
		t.Fatalf("expected no appId in request when --app not provided, got %v", receivedBody["appId"])
	}
}
