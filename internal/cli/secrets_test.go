package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// resetSecretsFlags resets flags that persist between tests.
func resetSecretsFlags() {
	resetJSONFlag()
	rootCmd.PersistentFlags().Set("output", "table")
	secretsDeleteCmd.Flags().Set("yes", "false")
	secretsGetCmd.Flags().Set("reveal", "false")
}

func TestResetSecretsFlagsResetsJSON(t *testing.T) {
	if err := rootCmd.PersistentFlags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	resetSecretsFlags()

	jsonFlag, err := rootCmd.PersistentFlags().GetBool("json")
	if err != nil {
		t.Fatalf("get json flag: %v", err)
	}
	if jsonFlag {
		t.Fatal("expected resetSecretsFlags to clear --json")
	}
}

// --- secrets set ---

func TestSecretsSetCreatesNew(t *testing.T) {
	resetSecretsFlags()
	var method, path string
	var reqBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		json.NewDecoder(r.Body).Decode(&reqBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"name": "DB_PASSWORD"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "set", "DB_PASSWORD", "s3cret",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if method != "POST" {
		t.Fatalf("expected POST, got %s", method)
	}
	if path != "/api/admin/secrets" {
		t.Fatalf("expected /api/admin/secrets, got %s", path)
	}
	if reqBody["name"] != "DB_PASSWORD" {
		t.Fatalf("expected name=DB_PASSWORD, got %v", reqBody["name"])
	}
	if reqBody["value"] != "s3cret" {
		t.Fatalf("expected value=s3cret, got %v", reqBody["value"])
	}
	if !strings.Contains(output, "Secret \"DB_PASSWORD\" created") {
		t.Fatalf("expected creation message, got %q", output)
	}
}

func TestSecretsSetUpdatesOnConflict(t *testing.T) {
	resetSecretsFlags()
	callCount := 0
	var lastMethod, lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		lastMethod = r.Method
		lastPath = r.URL.Path
		if r.Method == "POST" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"message": "secret already exists"})
			return
		}
		// PUT fallback
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"name": "DB_PASSWORD"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "set", "DB_PASSWORD", "new-value",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if callCount != 2 {
		t.Fatalf("expected 2 calls (POST then PUT), got %d", callCount)
	}
	if lastMethod != "PUT" {
		t.Fatalf("expected final method PUT, got %s", lastMethod)
	}
	if lastPath != "/api/admin/secrets/DB_PASSWORD" {
		t.Fatalf("expected PUT path /api/admin/secrets/DB_PASSWORD, got %s", lastPath)
	}
	if !strings.Contains(output, "Secret \"DB_PASSWORD\" updated") {
		t.Fatalf("expected update message, got %q", output)
	}
}

func TestSecretsSetFromStdin(t *testing.T) {
	resetSecretsFlags()
	var reqBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&reqBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"name": "API_KEY"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "set", "API_KEY", "-",
			"--url", srv.URL, "--admin-token", "tok"})
		rootCmd.SetIn(strings.NewReader("stdin-value\n"))
		defer rootCmd.SetIn(nil)
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if reqBody["value"] != "stdin-value" {
		t.Fatalf("expected value from stdin 'stdin-value', got %q", reqBody["value"])
	}
	if !strings.Contains(output, "Secret \"API_KEY\" created") {
		t.Fatalf("expected creation message, got %q", output)
	}
}

func TestSecretsSetRequiresArgs(t *testing.T) {
	resetSecretsFlags()
	rootCmd.SetArgs([]string{"secrets", "set",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

// --- secrets get ---

func TestSecretsGetMasked(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/admin/secrets/DB_PASSWORD" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"name": "DB_PASSWORD", "value": "s3cret"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "get", "DB_PASSWORD",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "DB_PASSWORD=****") {
		t.Fatalf("expected masked output DB_PASSWORD=****, got %q", output)
	}
	if strings.Contains(output, "s3cret") {
		t.Fatalf("value should be masked, got %q", output)
	}
}

func TestSecretsGetRevealed(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"name": "DB_PASSWORD", "value": "s3cret"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "get", "DB_PASSWORD", "--reveal",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "DB_PASSWORD=s3cret") {
		t.Fatalf("expected revealed output DB_PASSWORD=s3cret, got %q", output)
	}
}

func TestSecretsGetNotFound(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "secret not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"secrets", "get", "NOPE",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "secret not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestSecretsGetRequiresArg(t *testing.T) {
	resetSecretsFlags()
	rootCmd.SetArgs([]string{"secrets", "get",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

// --- secrets list ---

func TestSecretsListTable(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/admin/secrets" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "DB_PASSWORD", "created_at": "2026-02-22T00:00:00Z", "updated_at": "2026-02-22T01:00:00Z"},
			{"name": "API_KEY", "created_at": "2026-02-22T02:00:00Z", "updated_at": "2026-02-22T03:00:00Z"},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "list",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "DB_PASSWORD") {
		t.Fatalf("expected DB_PASSWORD in output, got %q", output)
	}
	if !strings.Contains(output, "API_KEY") {
		t.Fatalf("expected API_KEY in output, got %q", output)
	}
	if !strings.Contains(output, "2 secret(s)") {
		t.Fatalf("expected count in output, got %q", output)
	}
}

func TestSecretsListEmpty(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "list",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No secrets") {
		t.Fatalf("expected empty message, got %q", output)
	}
}

func TestSecretsListJSON(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"DB_PASSWORD","created_at":"2026-02-22T00:00:00Z","updated_at":"2026-02-22T00:00:00Z"}]`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "list", "--json",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"name"`) {
		t.Fatalf("expected JSON output, got %q", output)
	}
}

func TestSecretsListCSV(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "DB_PASSWORD", "created_at": "2026-02-22T00:00:00Z", "updated_at": "2026-02-22T01:00:00Z"},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "list", "--output", "csv",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header+data), got %d: %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "Name") {
		t.Fatalf("expected header with Name, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "DB_PASSWORD") {
		t.Fatalf("expected DB_PASSWORD in data, got %q", lines[1])
	}
}

// --- secrets delete ---

func TestSecretsDeleteWithConfirmation(t *testing.T) {
	resetSecretsFlags()
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
		rootCmd.SetArgs([]string{"secrets", "delete", "DB_PASSWORD",
			"--url", srv.URL, "--admin-token", "tok"})
		rootCmd.SetIn(strings.NewReader("y\n"))
		defer rootCmd.SetIn(nil)
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if deletedPath != "/api/admin/secrets/DB_PASSWORD" {
		t.Fatalf("expected delete path /api/admin/secrets/DB_PASSWORD, got %s", deletedPath)
	}
	if !strings.Contains(output, "Secret \"DB_PASSWORD\" deleted") {
		t.Fatalf("expected deletion message, got %q", output)
	}
}

func TestSecretsDeleteWithYesFlag(t *testing.T) {
	resetSecretsFlags()
	var deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deletedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "delete", "DB_PASSWORD", "--yes",
			"--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if deletedPath != "/api/admin/secrets/DB_PASSWORD" {
		t.Fatalf("expected delete path, got %s", deletedPath)
	}
	if !strings.Contains(output, "Secret \"DB_PASSWORD\" deleted") {
		t.Fatalf("expected deletion message, got %q", output)
	}
}

func TestSecretsDeleteCancelled(t *testing.T) {
	resetSecretsFlags()
	serverCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "delete", "DB_PASSWORD",
			"--url", srv.URL, "--admin-token", "tok"})
		rootCmd.SetIn(strings.NewReader("n\n"))
		defer rootCmd.SetIn(nil)
		// cancelled delete is not an error, just no-op
		rootCmd.Execute()
	})

	if serverCalled {
		t.Fatal("server should not be called when deletion is cancelled")
	}
}

func TestSecretsDeleteNotFound(t *testing.T) {
	resetSecretsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "secret not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"secrets", "delete", "NOPE", "--yes",
		"--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "secret not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestSecretsDeleteRequiresArg(t *testing.T) {
	resetSecretsFlags()
	rootCmd.SetArgs([]string{"secrets", "delete",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

// --- secrets command registration ---

func TestSecretsSubcommandsRegistered(t *testing.T) {
	resetSecretsFlags()
	want := map[string]bool{
		"set":    false,
		"get":    false,
		"list":   false,
		"delete": false,
	}
	for _, cmd := range secretsCmd.Commands() {
		if _, ok := want[cmd.Name()]; ok {
			want[cmd.Name()] = true
		}
	}
	for subcommand, found := range want {
		if !found {
			t.Fatalf("expected '%s' subcommand under 'secrets'", subcommand)
		}
	}
}

// --- auth propagation ---

func TestSecretsAuthSendsToken(t *testing.T) {
	resetSecretsFlags()
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"secrets", "list", "--url", srv.URL, "--admin-token", "my-secret-token"})
		rootCmd.Execute()
	})

	if receivedAuth != "Bearer my-secret-token" {
		t.Fatalf("expected Bearer token, got %q", receivedAuth)
	}
}
