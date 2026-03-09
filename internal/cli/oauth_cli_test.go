package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetOAuthCreateFlags resets all flags on the create command to defaults.
// StringSlice flags use pflag.SliceValue.Replace to bypass the internal
// changed-state append behavior that causes cross-test pollution.
func resetOAuthCreateFlags() {
	f := oauthClientsCreateCmd.Flags()
	f.Set("name", "")
	f.Set("type", "confidential")
	for _, name := range []string{"redirect-uris", "scopes"} {
		fl := f.Lookup(name)
		fl.Changed = false
		if sv, ok := fl.Value.(pflag.SliceValue); ok {
			sv.Replace([]string{})
		}
	}
}

// --- Command Registration ---

func TestOAuthCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "oauth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'oauth' subcommand to be registered")
	}
}

func TestOAuthClientsSubcommandRegistered(t *testing.T) {
	var oauthCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "oauth" {
			oauthCmd = cmd
			break
		}
	}
	if oauthCmd == nil {
		t.Fatal("expected 'oauth' subcommand")
	}

	found := false
	for _, cmd := range oauthCmd.Commands() {
		if cmd.Name() == "clients" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'clients' subcommand under 'oauth'")
	}
}

// --- oauth clients create ---

func TestOAuthClientsCreateSuccess(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/admin/oauth/clients" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"clientSecret": "ayb_cs_abcdef1234567890abcdef1234567890abcdef1234567890abcdef12345678",
			"client": map[string]any{
				"id":           "11111111-1111-1111-1111-111111111111",
				"appId":        "22222222-2222-2222-2222-222222222222",
				"clientId":     "ayb_cid_aabbccddee00112233445566778899aabbccddee0011",
				"name":         "My OAuth App",
				"redirectUris": []string{"https://example.com/callback"},
				"scopes":       []string{"readwrite"},
				"clientType":   "confidential",
				"createdAt":    "2026-02-22T00:00:00Z",
				"updatedAt":    "2026-02-22T00:00:00Z",
			},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "create",
			"22222222-2222-2222-2222-222222222222",
			"--name", "My OAuth App",
			"--redirect-uris", "https://example.com/callback",
			"--scopes", "readwrite",
			"--type", "confidential",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify request body
	if receivedBody["appId"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("expected appId in request body, got %v", receivedBody["appId"])
	}
	if receivedBody["name"] != "My OAuth App" {
		t.Fatalf("expected name in request body, got %v", receivedBody["name"])
	}
	if receivedBody["clientType"] != "confidential" {
		t.Fatalf("expected clientType in request body, got %v", receivedBody["clientType"])
	}

	// Verify output
	if !strings.Contains(output, "ayb_cid_") {
		t.Fatalf("expected client ID in output, got %q", output)
	}
	if !strings.Contains(output, "ayb_cs_") {
		t.Fatalf("expected client secret in output, got %q", output)
	}
	if !strings.Contains(output, "Save this secret") {
		t.Fatalf("expected save warning in output, got %q", output)
	}
}

func TestOAuthClientsCreatePublicNoSecret(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"clientSecret": "",
			"client": map[string]any{
				"id":           "11111111-1111-1111-1111-111111111111",
				"appId":        "22222222-2222-2222-2222-222222222222",
				"clientId":     "ayb_cid_aabbccddee00112233445566778899aabbccddee0011",
				"name":         "Public App",
				"redirectUris": []string{"http://localhost:3000/callback"},
				"scopes":       []string{"readonly"},
				"clientType":   "public",
				"createdAt":    "2026-02-22T00:00:00Z",
				"updatedAt":    "2026-02-22T00:00:00Z",
			},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "create",
			"22222222-2222-2222-2222-222222222222",
			"--name", "Public App",
			"--redirect-uris", "http://localhost:3000/callback",
			"--scopes", "readonly",
			"--type", "public",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Public client should not show secret
	if strings.Contains(output, "ayb_cs_") {
		t.Fatalf("did not expect client secret for public client, got %q", output)
	}
	if strings.Contains(output, "Save this secret") {
		t.Fatalf("did not expect save warning for public client, got %q", output)
	}
	if !strings.Contains(output, "ayb_cid_") {
		t.Fatalf("expected client ID in output, got %q", output)
	}
}

func TestOAuthClientsCreateMissingName(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached when name is missing")
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "create",
		"22222222-2222-2222-2222-222222222222",
		"--redirect-uris", "https://example.com/callback",
		"--scopes", "readwrite",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected name required error, got %q", err.Error())
	}
}

func TestOAuthClientsCreateMissingRedirectURIs(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached when redirect-uris is missing")
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "create",
		"22222222-2222-2222-2222-222222222222",
		"--name", "Test",
		"--scopes", "readwrite",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing redirect URIs")
	}
	if !strings.Contains(err.Error(), "--redirect-uris is required") {
		t.Fatalf("expected redirect-uris required error, got %q", err.Error())
	}
}

func TestOAuthClientsCreateMissingScopes(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be reached when scopes is missing")
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "create",
		"22222222-2222-2222-2222-222222222222",
		"--name", "Test",
		"--redirect-uris", "https://example.com/callback",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing scopes")
	}
	if !strings.Contains(err.Error(), "--scopes is required") {
		t.Fatalf("expected scopes required error, got %q", err.Error())
	}
}

func TestOAuthClientsCreateRequiresAppIDArg(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"oauth", "clients", "create",
		"--name", "Test",
		"--redirect-uris", "https://example.com/callback",
		"--scopes", "readwrite",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing app-id argument")
	}
}

func TestOAuthClientsCreateJSON(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"clientSecret":"ayb_cs_secret","client":{"clientId":"ayb_cid_abc","name":"JSON App"}}`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "create",
			"22222222-2222-2222-2222-222222222222",
			"--name", "JSON App",
			"--redirect-uris", "https://example.com/callback",
			"--scopes", "readwrite",
			"--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"clientSecret"`) {
		t.Fatalf("expected JSON output with clientSecret, got %q", output)
	}
	if !strings.Contains(output, `"clientId"`) {
		t.Fatalf("expected JSON output with clientId, got %q", output)
	}
}

func TestOAuthClientsCreateDefaultTypeConfidential(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"clientSecret": "ayb_cs_secret",
			"client":       map[string]any{"clientId": "ayb_cid_abc", "name": "Test"},
		})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "create",
			"22222222-2222-2222-2222-222222222222",
			"--name", "Test",
			"--redirect-uris", "https://example.com/callback",
			"--scopes", "readwrite",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if receivedBody["clientType"] != "confidential" {
		t.Fatalf("expected default clientType=confidential, got %v", receivedBody["clientType"])
	}
}

func TestOAuthClientsCreateServerError(t *testing.T) {
	resetJSONFlag()
	resetOAuthCreateFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"message": "app not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "create",
		"22222222-2222-2222-2222-222222222222",
		"--name", "Test",
		"--redirect-uris", "https://example.com/callback",
		"--scopes", "readwrite",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for server failure")
	}
	if !strings.Contains(err.Error(), "app not found") {
		t.Fatalf("expected error message, got %q", err.Error())
	}
}

// --- oauth clients list ---

func TestOAuthClientsListTable(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/oauth/clients" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":           "11111111-1111-1111-1111-111111111111",
					"appId":        "22222222-2222-2222-2222-222222222222",
					"clientId":     "ayb_cid_aabbccddee00112233445566778899aabbccddee0011",
					"name":         "My OAuth Client",
					"redirectUris": []string{"https://example.com/callback"},
					"scopes":       []string{"readwrite"},
					"clientType":   "confidential",
					"createdAt":    "2026-02-22T00:00:00Z",
					"updatedAt":    "2026-02-22T00:00:00Z",
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
		rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "My OAuth Client") {
		t.Fatalf("expected client name in output, got %q", output)
	}
	if !strings.Contains(output, "ayb_cid_") {
		t.Fatalf("expected client ID prefix in output, got %q", output)
	}
	if !strings.Contains(output, "confidential") {
		t.Fatalf("expected client type in output, got %q", output)
	}
	if !strings.Contains(output, "1 oauth client(s)") {
		t.Fatalf("expected total count in output, got %q", output)
	}
}

func TestOAuthClientsListJSON(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"totalItems":0}`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"totalItems":0`) {
		t.Fatalf("expected JSON output, got %q", output)
	}
}

func TestOAuthClientsListEmpty(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items":      []any{},
			"totalItems": 0,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No OAuth clients registered") {
		t.Fatalf("expected empty message, got %q", output)
	}
}

func TestOAuthClientsListCSV(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":           "11111111-1111-1111-1111-111111111111",
					"appId":        "22222222-2222-2222-2222-222222222222",
					"clientId":     "ayb_cid_aabbcc",
					"name":         "CSV Client",
					"redirectUris": []string{"https://example.com/callback"},
					"scopes":       []string{"readonly"},
					"clientType":   "public",
					"createdAt":    "2026-02-22T00:00:00Z",
					"updatedAt":    "2026-02-22T00:00:00Z",
				},
			},
			"totalItems": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "tok", "--output", "csv"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "CSV Client") {
		t.Fatalf("expected CSV output with client name, got %q", output)
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header+data), got %d", len(lines))
	}
}

func TestOAuthClientsListShowsRevokedStatus(t *testing.T) {
	resetJSONFlag()
	revokedAt := "2026-02-22T12:00:00Z"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":           "11111111-1111-1111-1111-111111111111",
					"appId":        "22222222-2222-2222-2222-222222222222",
					"clientId":     "ayb_cid_aabbcc",
					"name":         "Revoked Client",
					"redirectUris": []string{"https://example.com/callback"},
					"scopes":       []string{"readwrite"},
					"clientType":   "confidential",
					"createdAt":    "2026-02-22T00:00:00Z",
					"updatedAt":    "2026-02-22T00:00:00Z",
					"revokedAt":    revokedAt,
				},
			},
			"totalItems": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "revoked") {
		t.Fatalf("expected 'revoked' status in output, got %q", output)
	}
}

func TestOAuthClientsListServerError(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"message": "db down"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for server failure")
	}
	if !strings.Contains(err.Error(), "db down") {
		t.Fatalf("expected error message, got %q", err.Error())
	}
}

// --- oauth clients delete ---

func TestOAuthClientsDeleteSuccess(t *testing.T) {
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
		rootCmd.SetArgs([]string{"oauth", "clients", "delete",
			"ayb_cid_aabbccddee00112233445566778899aabbccddee0011",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	expectedPath := "/api/admin/oauth/clients/ayb_cid_aabbccddee00112233445566778899aabbccddee0011"
	if deletedPath != expectedPath {
		t.Fatalf("expected delete path %q, got %q", expectedPath, deletedPath)
	}
	if !strings.Contains(output, "revoked") {
		t.Fatalf("expected revoke confirmation, got %q", output)
	}
}

func TestOAuthClientsDeleteNotFound(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"message": "oauth client not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "delete", "ayb_cid_nonexistent",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "oauth client not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestOAuthClientsDeleteRequiresArg(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"oauth", "clients", "delete", "--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing client-id arg")
	}
}

// --- oauth clients rotate-secret ---

func TestOAuthClientsRotateSecretSuccess(t *testing.T) {
	resetJSONFlag()
	var requestPath string
	var requestMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestMethod = r.Method
		json.NewEncoder(w).Encode(map[string]any{
			"clientSecret": "ayb_cs_new_secret_00000000000000000000000000000000000000000000000000",
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "rotate-secret",
			"ayb_cid_aabbccddee00112233445566778899aabbccddee0011",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	expectedPath := "/api/admin/oauth/clients/ayb_cid_aabbccddee00112233445566778899aabbccddee0011/rotate-secret"
	if requestPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, requestPath)
	}
	if requestMethod != "POST" {
		t.Fatalf("expected POST, got %s", requestMethod)
	}
	if !strings.Contains(output, "ayb_cs_") {
		t.Fatalf("expected new secret in output, got %q", output)
	}
	if !strings.Contains(output, "Save this secret") {
		t.Fatalf("expected save warning in output, got %q", output)
	}
}

func TestOAuthClientsRotateSecretJSON(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"clientSecret":"ayb_cs_new_secret"}`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "rotate-secret",
			"ayb_cid_aabbccddee00112233445566778899aabbccddee0011",
			"--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"clientSecret"`) {
		t.Fatalf("expected JSON output with clientSecret, got %q", output)
	}
}

func TestOAuthClientsRotateSecretNotFound(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"message": "oauth client not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "rotate-secret", "ayb_cid_nonexistent",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "oauth client not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestOAuthClientsRotateSecretPublicClientRejected(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"message": "cannot regenerate secret for public client"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"oauth", "clients", "rotate-secret", "ayb_cid_public",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for public client")
	}
	if !strings.Contains(err.Error(), "cannot regenerate secret") {
		t.Fatalf("expected public client error, got %q", err.Error())
	}
}

func TestOAuthClientsRotateSecretRequiresArg(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"oauth", "clients", "rotate-secret", "--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing client-id arg")
	}
}

// --- Auth header propagation ---

func TestOAuthClientsListAuthSendsToken(t *testing.T) {
	resetJSONFlag()
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "totalItems": 0})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"oauth", "clients", "list", "--url", srv.URL, "--admin-token", "my-secret-token"})
		rootCmd.Execute()
	})

	if receivedAuth != "Bearer my-secret-token" {
		t.Fatalf("expected Bearer token, got %q", receivedAuth)
	}
}
