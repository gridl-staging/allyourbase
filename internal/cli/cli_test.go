package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

func TestSetVersion(t *testing.T) {
	SetVersion("1.2.3", "abc123", "2026-01-01")
	if buildVersion != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %q", buildVersion)
	}
	if buildCommit != "abc123" {
		t.Fatalf("expected abc123, got %q", buildCommit)
	}
	if buildDate != "2026-01-01" {
		t.Fatalf("expected 2026-01-01, got %q", buildDate)
	}
	SetVersion("dev", "none", "unknown")
}

// resetJSONFlag ensures the persistent --json flag is reset between tests.
func resetJSONFlag() {
	rootCmd.PersistentFlags().Set("json", "false")
}

// freePort allocates and returns a free TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// captureStdout captures stdout output from the given function.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("captureStdout: read pipe: %v", err)
	}
	return string(out)
}

func TestVersionCommand(t *testing.T) {
	SetVersion("0.1.0", "deadbeef", "2026-02-07")
	defer SetVersion("dev", "none", "unknown")

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"version"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "0.1.0") {
		t.Fatalf("expected version in output, got %q", output)
	}
	if !strings.Contains(output, "deadbeef") {
		t.Fatalf("expected commit in output, got %q", output)
	}
	if !strings.Contains(output, "2026-02-07") {
		t.Fatalf("expected date in output, got %q", output)
	}
}

func TestConfigCommandProducesValidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify it's valid TOML.
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("config output is not valid TOML: %v\noutput:\n%s", err, output)
	}
	if _, ok := parsed["server"]; !ok {
		t.Fatal("expected 'server' section in config output")
	}
	if _, ok := parsed["database"]; !ok {
		t.Fatal("expected 'database' section in config output")
	}
}

func TestRootCommandRegistersSubcommands(t *testing.T) {
	expected := []string{"start", "stop", "status", "config", "version", "migrate", "admin", "types", "sql", "query", "webhooks", "users", "storage", "schema", "rpc", "mcp", "init", "apikeys", "db", "logs", "stats", "secrets", "uninstall"}

	commands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		commands[cmd.Use] = true
	}

	for _, name := range expected {
		found := false
		for use := range commands {
			// Extract command name (Use field may contain "name [args]")
			cmdName := strings.Fields(use)[0]
			if cmdName == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

func TestMigrateCreateGeneratesFile(t *testing.T) {
	tmpDir := t.TempDir()
	migrDir := filepath.Join(tmpDir, "migrations")

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"migrate", "create", "add_posts", "--migrations-dir", migrDir})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Created migration") {
		t.Fatalf("expected 'Created migration' in output, got %q", output)
	}

	entries, err := os.ReadDir(migrDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 migration file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), "_add_posts.sql") {
		t.Fatalf("expected filename ending in _add_posts.sql, got %q", entries[0].Name())
	}
}

// captureStderr captures stderr output from the given function.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old

	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("captureStderr: read pipe: %v", err)
	}
	return string(out)
}

func TestPrintBanner(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8090
	cfg.Admin.Enabled = true
	cfg.Admin.Path = "/admin"

	SetVersion("0.2.0", "abc123", "2026-02-09")
	defer SetVersion("dev", "none", "unknown")

	output := captureStderr(t, func() {
		printBanner(cfg, true, "", "")
	})

	if !strings.Contains(output, "Allyourbase v0.2.0") {
		t.Errorf("expected version in banner, got %q", output)
	}
	if !strings.Contains(output, "http://localhost:8090/api") {
		t.Errorf("expected API URL in banner, got %q", output)
	}
	if !strings.Contains(output, "http://localhost:8090/admin") {
		t.Errorf("expected Admin URL in banner, got %q", output)
	}
	if !strings.Contains(output, "managed") {
		t.Errorf("expected 'managed' in banner, got %q", output)
	}
	if !strings.Contains(output, "allyourbase.io") {
		t.Errorf("expected docs link in banner, got %q", output)
	}
}

func TestStopCommandNoServer(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"stop"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Acceptable: clean state ("No AYB server is running") or orphan state
	// (another process holds the port but no PID file exists).
	if !strings.Contains(output, "No AYB server is running") && !strings.Contains(output, "No PID file") && !strings.Contains(output, "orphan") {
		t.Fatalf("expected 'no server running' or 'no PID file' message, got %q", output)
	}
}

func TestStopCommandJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"stop", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	// Acceptable statuses: not_running (clean) or orphan (port in use without PID file).
	status, _ := result["status"].(string)
	if status != "not_running" && status != "orphan" {
		t.Fatalf("expected status 'not_running' or 'orphan', got %v", result["status"])
	}
}

func TestStatusCommandNoServer(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"status"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "AYB server is not running") {
		t.Fatalf("expected 'AYB server is not running' message, got %q", output)
	}
}

func TestStatusCommandJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"status", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	if result["status"] != "stopped" {
		t.Fatalf("expected status 'stopped', got %v", result["status"])
	}
}

func TestStopCommandStalePID(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	aybDir := filepath.Join(tmpDir, ".ayb")
	os.MkdirAll(aybDir, 0o755)
	// Write a PID that doesn't exist (use a very high PID).
	if err := os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("9999999\n8090"), 0o644); err != nil {
		t.Fatalf("writing PID file: %v", err)
	}

	// Use cobra's SetOut to capture output without redirecting os.Stdout,
	// which avoids contamination from concurrent test goroutines in CI.
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	defer rootCmd.SetOut(nil) // restore default (os.Stdout)

	rootCmd.SetArgs([]string{"stop"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No AYB server is running") {
		t.Fatalf("expected 'No AYB server is running' message, got %q", output)
	}
}

func TestConfigCommandWithCustomFile(t *testing.T) {
	tmpDir := t.TempDir()

	customConfig := filepath.Join(tmpDir, "custom.toml")
	if err := os.WriteFile(customConfig, []byte("[server]\nport = 9999\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "--config", customConfig})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "9999") {
		t.Fatalf("expected custom port 9999 in output, got %q", output)
	}
}

func TestMigrateCreateRequiresName(t *testing.T) {
	rootCmd.SetArgs([]string{"migrate", "create"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing migration name")
	}
}

func TestMigrateSubcommands(t *testing.T) {
	found := make(map[string]bool)
	for _, cmd := range migrateCmd.Commands() {
		found[cmd.Name()] = true
	}
	for _, name := range []string{"up", "create", "status", "pocketbase", "supabase", "firebase"} {
		if !found[name] {
			t.Errorf("expected migrate subcommand %q", name)
		}
	}
}

func TestMigrateSupabaseRequiresFlags(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"migrate", "supabase"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required flags")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required flag error, got %q", err.Error())
	}
}

func TestMigrateSupabaseFlagDefinitions(t *testing.T) {
	flags := migrateSupabaseCmd.Flags()
	for _, name := range []string{"source-url", "database-url", "storage-export", "storage-path", "dry-run", "force", "verbose", "skip-rls", "skip-oauth", "skip-data", "skip-storage", "include-anonymous", "yes", "json"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on migrate supabase command", name)
			continue
		}
		// Verify boolean flags have correct type.
		switch name {
		case "source-url", "database-url", "storage-export", "storage-path":
			if f.Value.Type() != "string" {
				t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
			}
		default:
			if f.Value.Type() != "bool" {
				t.Errorf("flag %q should be bool, got %s", name, f.Value.Type())
			}
		}
	}
}

func TestMigratePocketbaseRequiresSource(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"migrate", "pocketbase"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required flags")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required flag error, got %q", err.Error())
	}
}

func TestMigratePocketbaseFlagDefinitions(t *testing.T) {
	flags := migratePocketbaseCmd.Flags()
	for _, name := range []string{"source", "database-url", "dry-run", "skip-files", "force", "verbose", "yes", "json"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on migrate pocketbase command", name)
			continue
		}
		switch name {
		case "source", "database-url":
			if f.Value.Type() != "string" {
				t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
			}
		default:
			if f.Value.Type() != "bool" {
				t.Errorf("flag %q should be bool, got %s", name, f.Value.Type())
			}
		}
	}
}

func TestMigrateFirebaseRequiresDatabaseURL(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"migrate", "firebase", "--auth-export", "auth.json"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required flags")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required flag error, got %q", err.Error())
	}
}

func TestMigrateFirebaseRequiresExportPath(t *testing.T) {
	resetJSONFlag()
	migrateFirebaseCmd.Flags().Set("auth-export", "")
	rootCmd.SetArgs([]string{"migrate", "firebase", "--database-url", "postgres://localhost/test"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing export paths")
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("expected export path error, got %q", err.Error())
	}
}

func TestMigrateFirebaseFlagDefinitions(t *testing.T) {
	flags := migrateFirebaseCmd.Flags()
	for _, name := range []string{"auth-export", "firestore-export", "rtdb-export", "storage-export", "storage-path", "database-url", "dry-run", "verbose", "yes", "json"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on migrate firebase command", name)
			continue
		}
		switch name {
		case "auth-export", "firestore-export", "rtdb-export", "storage-export", "storage-path", "database-url":
			if f.Value.Type() != "string" {
				t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
			}
		default:
			if f.Value.Type() != "bool" {
				t.Errorf("flag %q should be bool, got %s", name, f.Value.Type())
			}
		}
	}
}

func TestStartFromFlagDefined(t *testing.T) {
	f := startCmd.Flags().Lookup("from")
	if f == nil {
		t.Fatal("expected --from flag on start command")
	}
	if f.Value.Type() != "string" {
		t.Errorf("--from flag should be string, got %s", f.Value.Type())
	}
}

func TestStartFromInvalidSource(t *testing.T) {
	resetJSONFlag()
	// Use a free port so the early port check doesn't short-circuit the test.
	port := freePort(t)
	rootCmd.SetArgs([]string{"start", "--foreground", "--from", "/nonexistent/pb_data", "--port", fmt.Sprintf("%d", port)})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid source path")
	}
	// Should fail on analysis (source doesn't exist), not on source detection
	if !strings.Contains(err.Error(), "migration failed") {
		t.Fatalf("expected migration error, got %q", err.Error())
	}
}

func TestPrintBannerExternalDB(t *testing.T) {
	cfg := config.Default()
	cfg.Admin.Enabled = false

	output := captureStderr(t, func() {
		printBanner(cfg, false, "", "")
	})

	if !strings.Contains(output, "external") {
		t.Errorf("expected 'external' in banner, got %q", output)
	}
	if strings.Contains(output, "Admin:") {
		t.Errorf("admin URL should not appear when disabled, got %q", output)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	resetJSONFlag()
	SetVersion("1.0.0", "abc123", "2026-02-09")
	defer SetVersion("dev", "none", "unknown")

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"version", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	if result["version"] != "1.0.0" {
		t.Fatalf("expected version 1.0.0, got %v", result["version"])
	}
	if result["commit"] != "abc123" {
		t.Fatalf("expected commit abc123, got %v", result["commit"])
	}
	if result["date"] != "2026-02-09" {
		t.Fatalf("expected date 2026-02-09, got %v", result["date"])
	}
}

func TestConfigCommandJSON(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	if _, ok := result["Server"]; !ok {
		t.Fatal("expected 'Server' key in JSON config output")
	}
}

func TestQueryCommandRequiresTable(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"query"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing table argument")
	}
}

// readAYBPID tests are in start_test.go (TestReadAYBPID_*).

func TestWebhooksSubcommands(t *testing.T) {
	found := make(map[string]bool)
	for _, cmd := range webhooksCmd.Commands() {
		found[cmd.Name()] = true
	}
	for _, name := range []string{"list", "create", "delete"} {
		if !found[name] {
			t.Errorf("expected webhooks subcommand %q", name)
		}
	}
}

func TestWebhooksDeleteRequiresID(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"webhooks", "delete"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing webhook ID")
	}
}

func TestWebhooksCreateRequiresURL(t *testing.T) {
	resetJSONFlag()
	// Create without --webhook-url should fail with our custom validation.
	// But it tries to connect to the server first, so we use a bogus URL.
	rootCmd.SetArgs([]string{"webhooks", "create", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --webhook-url")
	}
	if !strings.Contains(err.Error(), "--webhook-url is required") {
		t.Fatalf("expected --webhook-url error, got %q", err.Error())
	}
}

func TestUsersSubcommands(t *testing.T) {
	found := make(map[string]bool)
	for _, cmd := range usersCmd.Commands() {
		found[cmd.Name()] = true
	}
	for _, name := range []string{"list", "delete"} {
		if !found[name] {
			t.Errorf("expected users subcommand %q", name)
		}
	}
}

func TestUsersDeleteRequiresID(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"users", "delete"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing user ID")
	}
}

func TestStorageSubcommands(t *testing.T) {
	found := make(map[string]bool)
	for _, cmd := range storageCmd.Commands() {
		found[cmd.Name()] = true
	}
	for _, name := range []string{"ls", "upload", "download", "delete"} {
		if !found[name] {
			t.Errorf("expected storage subcommand %q", name)
		}
	}
}

func TestStorageLsRequiresBucket(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"storage", "ls"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing bucket argument")
	}
}

func TestStorageUploadRequiresBucketAndFile(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"storage", "upload", "mybucket"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing file argument")
	}
}

func TestStorageDeleteRequiresBucketAndName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"storage", "delete", "mybucket"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing name argument")
	}
}

func TestStorageDownloadRequiresBucketAndName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"storage", "download", "mybucket"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing name argument")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestServerURLDefault(t *testing.T) {
	// With no PID file, serverURL should return default.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	got := serverURL()
	if got != "http://127.0.0.1:8090" {
		t.Fatalf("expected default server URL, got %q", got)
	}
}

func TestServerURLFromPID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	aybDir := filepath.Join(tmpDir, ".ayb")
	if err := os.MkdirAll(aybDir, 0o755); err != nil {
		t.Fatalf("creating .ayb dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("12345\n3000"), 0o644); err != nil {
		t.Fatalf("writing PID file: %v", err)
	}

	got := serverURL()
	if got != "http://127.0.0.1:3000" {
		t.Fatalf("expected http://127.0.0.1:3000, got %q", got)
	}
}

// --- Schema command tests ---

func TestSchemaCommandAcceptsOptionalArg(t *testing.T) {
	// schema accepts 0 or 1 args — verify it doesn't reject 0 args
	// (it will fail connecting to server, which is expected)
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schema", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	// Expect connection error, not an args error.
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), "accepts") {
		t.Fatalf("expected connection error, not args error: %v", err)
	}
}

func TestSchemaCommandRejectsTwoArgs(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schema", "table1", "table2"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for too many arguments")
	}
}

// --- RPC command tests ---

func TestRPCCommandRequiresFunction(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"rpc"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing function name")
	}
}

func TestParseRPCArgs(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    map[string]any
		wantErr bool
	}{
		{
			name:  "string value",
			input: []string{"name=alice"},
			want:  map[string]any{"name": "alice"},
		},
		{
			name:  "numeric value",
			input: []string{"count=42"},
			want:  map[string]any{"count": float64(42)},
		},
		{
			name:  "boolean value",
			input: []string{"active=true"},
			want:  map[string]any{"active": true},
		},
		{
			name:  "null value",
			input: []string{"data=null"},
			want:  map[string]any{"data": nil},
		},
		{
			name:  "multiple args",
			input: []string{"a=1", "b=hello"},
			want:  map[string]any{"a": float64(1), "b": "hello"},
		},
		{
			name:    "invalid format no equals",
			input:   []string{"badarg"},
			wantErr: true,
		},
		{
			name:  "value with equals sign",
			input: []string{"expr=a=b"},
			want:  map[string]any{"expr": "a=b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRPCArgs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestFormatScalar(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, "NULL"},
		{"hello", "hello"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
	}
	for _, tt := range tests {
		got := formatScalar(tt.input)
		if got != tt.want {
			t.Errorf("formatScalar(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Init command tests ---

func TestInitRequiresProjectName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"init"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing project name")
	}
}

func TestInitRejectsInvalidTemplate(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"init", "myapp", "--template", "nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
	if !strings.Contains(err.Error(), "unknown template") {
		t.Fatalf("expected 'unknown template' error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "react") {
		t.Fatalf("expected available templates listed in error, got %q", err.Error())
	}
}

func TestInitGeneratesProject(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"init", "test-project", "--template", "react"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Verify output mentions the project
	if !strings.Contains(output, "test-project") {
		t.Fatalf("expected project name in output, got %q", output)
	}
	if !strings.Contains(output, "Done!") {
		t.Fatalf("expected 'Done!' in output, got %q", output)
	}
	if !strings.Contains(output, "ayb start") {
		t.Fatalf("expected 'ayb start' in next steps, got %q", output)
	}

	// Verify project directory and key files exist
	projectDir := filepath.Join(tmpDir, "test-project")
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		t.Fatal("expected project directory to exist")
	}
	for _, file := range []string{"ayb.toml", "schema.sql", "package.json", "src/lib/ayb.ts", "CLAUDE.md"} {
		path := filepath.Join(projectDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatalf("expected file %q to exist", file)
		}
	}
}

func TestInitRejectsExistingDirectory(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	// Pre-create the directory
	os.MkdirAll(filepath.Join(tmpDir, "existing-app"), 0755)

	rootCmd.SetArgs([]string{"init", "existing-app", "--template", "react"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for existing directory")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got %q", err.Error())
	}
}

func TestInitDefaultsToReactTemplate(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"init", "react-default"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "react") {
		t.Fatalf("expected 'react' template in output, got %q", output)
	}

	// Verify React-specific file exists (vite.config.ts)
	viteConfig := filepath.Join(tmpDir, "react-default", "vite.config.ts")
	if _, err := os.Stat(viteConfig); os.IsNotExist(err) {
		t.Fatal("expected vite.config.ts for default React template")
	}
}

// --- MCP command tests ---

// --- DB command tests ---

func TestDBSubcommands(t *testing.T) {
	var dbCommand *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "db" {
			dbCommand = cmd
			break
		}
	}
	if dbCommand == nil {
		t.Fatal("db command not found")
	}

	expected := map[string]bool{"backup": true, "restore": true}
	for _, sub := range dbCommand.Commands() {
		delete(expected, sub.Name())
	}
	for name := range expected {
		t.Errorf("missing db subcommand: %s", name)
	}
}

func TestDBBackupRequiresDBURL(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir) // no ayb.toml
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"db", "backup"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without database URL")
	}
}

func TestDBBackupInvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"db", "backup", "--format", "invalid", "--database-url", "postgresql://localhost/db"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("expected 'invalid format' error, got: %v", err)
	}
}

func TestDBRestoreRequiresArg(t *testing.T) {
	rootCmd.SetArgs([]string{"db", "restore"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without backup file argument")
	}
}

func TestDBRestoreFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"db", "restore", "/nonexistent/backup.sql", "--database-url", "postgresql://localhost/db"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing backup file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

// --- Config get/set tests ---

func TestConfigGetSubcommands(t *testing.T) {
	var cfgCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "config" {
			cfgCmd = cmd
			break
		}
	}
	if cfgCmd == nil {
		t.Fatal("config command not found")
	}

	expected := map[string]bool{"get": true, "set": true}
	for _, sub := range cfgCmd.Commands() {
		delete(expected, sub.Name())
	}
	for name := range expected {
		t.Errorf("missing config subcommand: %s", name)
	}
}

func TestConfigGetRequiresArg(t *testing.T) {
	rootCmd.SetArgs([]string{"config", "get"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without key argument")
	}
}

func TestConfigGetReturnsValue(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir) // no ayb.toml, will use defaults
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "get", "server.port"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "8090") {
		t.Fatalf("expected default port 8090, got %q", output)
	}
}

func TestConfigSetRequiresArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"config", "set"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without arguments")
	}

	rootCmd.SetArgs([]string{"config", "set", "server.port"})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error with only one argument")
	}
}

func TestConfigSetUnknownKey(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"config", "set", "nonexistent.key", "value"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown configuration key") {
		t.Fatalf("expected 'unknown configuration key' error, got: %v", err)
	}
}

func TestConfigSetAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Set a value.
	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "server.port", "3000"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("set error: %v", err)
		}
	})
	if !strings.Contains(output, "server.port = 3000") {
		t.Fatalf("expected confirmation, got %q", output)
	}

	// Get it back.
	output = captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "get", "server.port"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("get error: %v", err)
		}
	})
	if !strings.Contains(output, "3000") {
		t.Fatalf("expected 3000, got %q", output)
	}
}

func TestConfigSetCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	configPath := filepath.Join(tmpDir, "ayb.toml")
	// File should not exist yet.
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("ayb.toml should not exist before set")
	}

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "server.port", "4000"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("set error: %v", err)
		}
	})

	// File should now exist.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("ayb.toml should be created by config set")
	}

	// Verify the file is valid TOML with the value.
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !strings.Contains(string(raw), "4000") {
		t.Fatalf("expected 4000 in config file, got %q", string(raw))
	}
}

func TestConfigSetBoolCoercion(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Use admin.enabled which doesn't trigger validation errors like auth.enabled does.
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "admin.enabled", "true"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("set error: %v", err)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "get", "admin.enabled"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("get error: %v", err)
		}
	})
	if !strings.Contains(output, "true") {
		t.Fatalf("expected true, got %q", output)
	}

	// Verify TOML file stores as bool (no quotes).
	raw, err := os.ReadFile(filepath.Join(tmpDir, "ayb.toml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if strings.Contains(string(raw), `"true"`) {
		t.Fatal("boolean should be stored as TOML bool, not quoted string")
	}
}

func TestConfigSetIntCoercion(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "server.port", "9999"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("set error: %v", err)
		}
	})

	// The config file should contain the integer value, parseable as TOML int.
	raw, err := os.ReadFile(filepath.Join(tmpDir, "ayb.toml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	// TOML int format: port = 9999 (no quotes).
	if strings.Contains(string(raw), `"9999"`) {
		t.Fatal("port should be stored as TOML integer, not string")
	}
	if !strings.Contains(string(raw), "9999") {
		t.Fatalf("expected 9999 in config file, got %q", string(raw))
	}
}

func TestConfigGetMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Test reading default values for various config sections.
	tests := []struct {
		key      string
		contains string
	}{
		{"server.host", "0.0.0.0"},
		{"server.port", "8090"},
		{"admin.enabled", "true"},
		{"auth.enabled", "false"},
		{"storage.backend", "local"},
		{"logging.level", "info"},
	}
	for _, tt := range tests {
		output := captureStdout(t, func() {
			rootCmd.SetArgs([]string{"config", "get", tt.key})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("config get %s error: %v", tt.key, err)
			}
		})
		if !strings.Contains(output, tt.contains) {
			t.Errorf("config get %s: expected %q in output, got %q", tt.key, tt.contains, output)
		}
	}
}

func TestDBBackupValidFormats(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Valid format names should not produce "invalid format" error.
	// They will fail on the pg_dump step or DB URL step, but not on format validation.
	for _, format := range []string{"plain", "custom", "tar", "directory", "p", "c", "t", "d"} {
		rootCmd.SetArgs([]string{"db", "backup", "--format", format, "--database-url", "postgresql://localhost/db"})
		err := rootCmd.Execute()
		if err != nil && strings.Contains(err.Error(), "invalid format") {
			t.Errorf("format %q should be valid, got: %v", format, err)
		}
	}
}

func TestDBRestoreRequiresDBURL(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Reset any leaked flag state from prior tests.
	dbRestoreCmd.Flags().Set("database-url", "")

	// Create a dummy file to restore.
	dummyFile := filepath.Join(tmpDir, "test.sql")
	os.WriteFile(dummyFile, []byte("-- empty"), 0644)

	rootCmd.SetArgs([]string{"db", "restore", dummyFile})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without database URL")
	}
	if !strings.Contains(err.Error(), "no database URL") {
		t.Fatalf("expected 'no database URL' error, got: %v", err)
	}
}

func TestAPIKeysSubcommands(t *testing.T) {
	var apikeysCommand *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "apikeys" {
			apikeysCommand = cmd
			break
		}
	}
	if apikeysCommand == nil {
		t.Fatal("apikeys command not found")
	}

	expected := map[string]bool{"list": true, "create": true, "revoke": true}
	for _, sub := range apikeysCommand.Commands() {
		delete(expected, sub.Name())
	}
	for name := range expected {
		t.Errorf("missing apikeys subcommand: %s", name)
	}
}

func TestLogsCommandRegistered(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "logs" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("logs command not registered")
	}

	// Verify flags
	f := found.Flags()
	if f.Lookup("lines") == nil {
		t.Error("logs command missing --lines flag")
	}
	if f.Lookup("follow") == nil {
		t.Error("logs command missing --follow flag")
	}
	if f.Lookup("level") == nil {
		t.Error("logs command missing --level flag")
	}
}

func TestLogsDefaultLines(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "logs" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("logs command not registered")
	}

	lines, err := found.Flags().GetInt("lines")
	if err != nil {
		t.Fatalf("getting lines flag: %v", err)
	}
	if lines != 100 {
		t.Errorf("expected default lines=100, got %d", lines)
	}
}

func TestLogsFollowDefault(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "logs" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("logs command not registered")
	}

	follow, err := found.Flags().GetBool("follow")
	if err != nil {
		t.Fatalf("getting follow flag: %v", err)
	}
	if follow {
		t.Error("follow should default to false")
	}
}

func TestLogsLevelFlag(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "logs" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("logs command not registered")
	}

	level, err := found.Flags().GetString("level")
	if err != nil {
		t.Fatalf("getting level flag: %v", err)
	}
	if level != "" {
		t.Errorf("level should default to empty, got %q", level)
	}
}

func TestStatsCommandRegistered(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "stats" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("stats command not registered")
	}
}

func TestStatsHasShortDescription(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "stats" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("stats command not registered")
	}
	if found.Short == "" {
		t.Error("stats command should have a short description")
	}
}

func TestSecretsCommandRegistered(t *testing.T) {
	var found *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "secrets" {
			found = cmd
			break
		}
	}
	if found == nil {
		t.Fatal("secrets command not registered")
	}
}

func TestSecretsRotateSubcommand(t *testing.T) {
	var secretsCommand *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "secrets" {
			secretsCommand = cmd
			break
		}
	}
	if secretsCommand == nil {
		t.Fatal("secrets command not found")
	}

	var rotateFound bool
	for _, sub := range secretsCommand.Commands() {
		if sub.Name() == "rotate" {
			rotateFound = true
			break
		}
	}
	if !rotateFound {
		t.Error("secrets rotate subcommand not found")
	}
}

func TestSecretsNoRunWithoutSubcommand(t *testing.T) {
	var secretsCommand *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "secrets" {
			secretsCommand = cmd
			break
		}
	}
	if secretsCommand == nil {
		t.Fatal("secrets command not found")
	}
	// secrets with no subcommand should have no RunE (requires subcommand)
	if secretsCommand.RunE != nil {
		t.Error("secrets command should not have RunE (requires subcommand)")
	}
}

// --- Uninstall command tests ---

func TestUninstallCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "uninstall" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'uninstall' subcommand to be registered")
	}
}

func TestUninstallFlagDefinitions(t *testing.T) {
	flags := uninstallCmd.Flags()
	for _, name := range []string{"purge", "yes"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on uninstall command", name)
			continue
		}
		if f.Value.Type() != "bool" {
			t.Errorf("flag %q should be bool, got %s", name, f.Value.Type())
		}
	}
}

func TestUninstallNothingToUninstall(t *testing.T) {
	resetJSONFlag()
	// Temporarily override HOME so ~/.ayb doesn't exist.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Nothing to uninstall") {
		t.Fatalf("expected 'Nothing to uninstall' message, got %q", output)
	}
}

func TestUninstallNothingToUninstallJSON(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	if result["status"] != "not_installed" {
		t.Fatalf("expected status 'not_installed', got %v", result["status"])
	}
}

func TestUninstallRemovesBinary(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create fake ~/.ayb/bin/ayb
	binDir := filepath.Join(tmpHome, ".ayb", "bin")
	os.MkdirAll(binDir, 0755)
	binPath := filepath.Join(binDir, "ayb")
	os.WriteFile(binPath, []byte("fake-binary"), 0755)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "AYB uninstalled") {
		t.Fatalf("expected 'AYB uninstalled' message, got %q", output)
	}

	// Binary should be gone.
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Fatal("expected binary to be removed")
	}
}

func TestUninstallRemovesCachedDirs(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	aybDir := filepath.Join(tmpHome, ".ayb")
	// Create dirs that should be cleaned up.
	for _, sub := range []string{"bin", "pg", "run"} {
		os.MkdirAll(filepath.Join(aybDir, sub), 0755)
	}
	os.WriteFile(filepath.Join(aybDir, "bin", "ayb"), []byte("fake"), 0755)
	os.WriteFile(filepath.Join(aybDir, "pg", "cached"), []byte("pg-binary"), 0644)
	os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("99999\n8090"), 0644)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// pg and run dirs should be gone.
	for _, sub := range []string{"pg", "run"} {
		if _, err := os.Stat(filepath.Join(aybDir, sub)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", sub)
		}
	}
	// PID file should be gone.
	if _, err := os.Stat(filepath.Join(aybDir, "ayb.pid")); !os.IsNotExist(err) {
		t.Error("expected ayb.pid to be removed")
	}
}

func TestUninstallPreservesDataByDefault(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	aybDir := filepath.Join(tmpHome, ".ayb")
	os.MkdirAll(filepath.Join(aybDir, "bin"), 0755)
	os.MkdirAll(filepath.Join(aybDir, "data"), 0755)
	os.WriteFile(filepath.Join(aybDir, "bin", "ayb"), []byte("fake"), 0755)
	os.WriteFile(filepath.Join(aybDir, "data", "PG_VERSION"), []byte("15"), 0644)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Data should still exist.
	if _, err := os.Stat(filepath.Join(aybDir, "data", "PG_VERSION")); os.IsNotExist(err) {
		t.Fatal("expected data directory to be preserved")
	}
	if !strings.Contains(output, "data directory was preserved") {
		t.Fatalf("expected data preservation notice, got %q", output)
	}
}

func TestUninstallPurgeRemovesEverything(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	aybDir := filepath.Join(tmpHome, ".ayb")
	os.MkdirAll(filepath.Join(aybDir, "bin"), 0755)
	os.MkdirAll(filepath.Join(aybDir, "data"), 0755)
	os.WriteFile(filepath.Join(aybDir, "bin", "ayb"), []byte("fake"), 0755)
	os.WriteFile(filepath.Join(aybDir, "data", "PG_VERSION"), []byte("15"), 0644)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall", "--purge", "--yes"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Entire ~/.ayb should be gone.
	if _, err := os.Stat(aybDir); !os.IsNotExist(err) {
		t.Fatal("expected ~/.ayb to be completely removed with --purge")
	}
}

func TestUninstallCleansShellProfile(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	aybDir := filepath.Join(tmpHome, ".ayb")
	binDir := filepath.Join(aybDir, "bin")
	os.MkdirAll(binDir, 0755)
	os.WriteFile(filepath.Join(binDir, "ayb"), []byte("fake"), 0755)

	// Create a .zshrc with AYB PATH entry (as install.sh would).
	zshrc := filepath.Join(tmpHome, ".zshrc")
	content := "# existing config\nexport PATH=\"/usr/bin:$PATH\"\n\n# Allyourbase\nexport PATH=\"" + binDir + ":$PATH\"\n"
	os.WriteFile(zshrc, []byte(content), 0644)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Read .zshrc and verify AYB lines are gone.
	data, err := os.ReadFile(zshrc)
	if err != nil {
		t.Fatalf("reading .zshrc: %v", err)
	}
	if strings.Contains(string(data), "Allyourbase") {
		t.Fatalf("expected Allyourbase lines to be removed from .zshrc, got:\n%s", string(data))
	}
	if strings.Contains(string(data), binDir) {
		t.Fatalf("expected bin dir to be removed from .zshrc, got:\n%s", string(data))
	}
	// Existing content should be preserved.
	if !strings.Contains(string(data), "existing config") {
		t.Fatalf("expected existing config to be preserved in .zshrc, got:\n%s", string(data))
	}
}

func TestUninstallJSONOutput(t *testing.T) {
	resetJSONFlag()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	aybDir := filepath.Join(tmpHome, ".ayb")
	os.MkdirAll(filepath.Join(aybDir, "bin"), 0755)
	os.WriteFile(filepath.Join(aybDir, "bin", "ayb"), []byte("fake"), 0755)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"uninstall", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", output, err)
	}
	if result["status"] != "uninstalled" {
		t.Fatalf("expected status 'uninstalled', got %v", result["status"])
	}
	removed, ok := result["removed"].([]any)
	if !ok {
		t.Fatal("expected 'removed' to be an array")
	}
	if len(removed) == 0 {
		t.Fatal("expected at least one removed item")
	}
}

func TestRemoveAYBLines(t *testing.T) {
	tmpDir := t.TempDir()
	profile := filepath.Join(tmpDir, ".bashrc")

	content := `# some config
export PATH="/usr/bin:$PATH"

# Allyourbase
export PATH="/home/user/.ayb/bin:$PATH"

# more config
alias ll='ls -la'
`
	os.WriteFile(profile, []byte(content), 0644)

	modified := removeAYBLines(profile, "/home/user/.ayb/bin")
	if !modified {
		t.Fatal("expected file to be modified")
	}

	data, _ := os.ReadFile(profile)
	result := string(data)
	if strings.Contains(result, "Allyourbase") {
		t.Fatalf("expected Allyourbase comment removed, got:\n%s", result)
	}
	if strings.Contains(result, ".ayb/bin") {
		t.Fatalf("expected PATH line removed, got:\n%s", result)
	}
	if !strings.Contains(result, "some config") || !strings.Contains(result, "alias ll") {
		t.Fatalf("expected other config preserved, got:\n%s", result)
	}
}

func TestRemoveAYBLinesNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	profile := filepath.Join(tmpDir, ".bashrc")

	content := "# just some config\nexport PATH=\"/usr/bin:$PATH\"\n"
	os.WriteFile(profile, []byte(content), 0644)

	modified := removeAYBLines(profile, "/home/user/.ayb/bin")
	if modified {
		t.Fatal("expected file to NOT be modified")
	}
}

func TestRemoveAYBLinesMissingFile(t *testing.T) {
	modified := removeAYBLines("/nonexistent/file", "/home/user/.ayb/bin")
	if modified {
		t.Fatal("expected false for missing file")
	}
}

func TestIsEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty dir
	emptyDir := filepath.Join(tmpDir, "empty")
	os.MkdirAll(emptyDir, 0755)
	if !isEmpty(emptyDir) {
		t.Error("expected empty dir to be empty")
	}

	// Non-empty dir
	os.WriteFile(filepath.Join(emptyDir, "file"), []byte("x"), 0644)
	if isEmpty(emptyDir) {
		t.Error("expected non-empty dir to not be empty")
	}

	// Non-existent dir
	if isEmpty(filepath.Join(tmpDir, "nope")) {
		t.Error("expected non-existent dir to not be empty")
	}
}

func TestSecretsRotateHasConfigFlag(t *testing.T) {
	var secretsCommand *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "secrets" {
			secretsCommand = cmd
			break
		}
	}
	if secretsCommand == nil {
		t.Fatal("secrets command not found")
	}

	var rotateCmd *cobra.Command
	for _, sub := range secretsCommand.Commands() {
		if sub.Name() == "rotate" {
			rotateCmd = sub
			break
		}
	}
	if rotateCmd == nil {
		t.Fatal("rotate subcommand not found")
	}

	if rotateCmd.Flags().Lookup("config") == nil {
		t.Error("secrets rotate missing --config flag")
	}
}

// --- Migration command error path tests ---

func TestMigrateFirebaseAuthExportOnly(t *testing.T) {
	// --auth-export alone (without --firestore-export) should pass flag validation
	// but fail on migrator creation (invalid DB URL).
	resetJSONFlag()
	rootCmd.SetArgs([]string{
		"migrate", "firebase",
		"--auth-export", "/nonexistent/auth.json",
		"--database-url", "postgres://invalid:5432/test",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error (migrator should fail), got nil")
	}
	// Should NOT get "at least one of" error — the validation passed.
	if strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("flag validation should pass with --auth-export only, got %q", err.Error())
	}
}

func TestMigrateFirebaseFirestoreExportOnly(t *testing.T) {
	// --firestore-export alone (without --auth-export) should pass flag validation.
	resetJSONFlag()
	rootCmd.SetArgs([]string{
		"migrate", "firebase",
		"--firestore-export", "/nonexistent/firestore/",
		"--database-url", "postgres://invalid:5432/test",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error (migrator should fail), got nil")
	}
	if strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("flag validation should pass with --firestore-export only, got %q", err.Error())
	}
}

func TestMigrateFirebaseBothExportPaths(t *testing.T) {
	// Both --auth-export and --firestore-export should pass flag validation.
	resetJSONFlag()
	rootCmd.SetArgs([]string{
		"migrate", "firebase",
		"--auth-export", "/nonexistent/auth.json",
		"--firestore-export", "/nonexistent/firestore/",
		"--database-url", "postgres://invalid:5432/test",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error (migrator should fail), got nil")
	}
	if strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("flag validation should pass with both export paths, got %q", err.Error())
	}
}

func TestMigrateFirebaseDryRunStillRequiresExport(t *testing.T) {
	// --dry-run does not bypass the "at least one export" requirement.
	resetJSONFlag()
	migrateFirebaseCmd.Flags().Set("auth-export", "")
	migrateFirebaseCmd.Flags().Set("firestore-export", "")
	rootCmd.SetArgs([]string{
		"migrate", "firebase",
		"--database-url", "postgres://localhost/test",
		"--dry-run",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing export paths")
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("expected 'at least one of' error, got %q", err.Error())
	}
}

func TestMigrateSupabaseMissingSourceURLOnly(t *testing.T) {
	// Only --database-url provided, --source-url missing.
	// When Cobra required flag check is bypassed (flag previously set),
	// the migrator itself validates the empty source URL.
	resetJSONFlag()
	rootCmd.SetArgs([]string{
		"migrate", "supabase",
		"--database-url", "postgres://localhost/test",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --source-url")
	}
	// Cobra may return "required" or migrator may return "source database URL is required".
	errMsg := err.Error()
	if !strings.Contains(errMsg, "source") && !strings.Contains(errMsg, "required") {
		t.Fatalf("expected source-url or required error, got %q", errMsg)
	}
}

func TestMigrateSupabaseMissingDatabaseURLOnly(t *testing.T) {
	// Only --source-url provided, --database-url missing.
	resetJSONFlag()
	rootCmd.SetArgs([]string{
		"migrate", "supabase",
		"--source-url", "postgres://supabase:5432/postgres",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --database-url")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "database") && !strings.Contains(errMsg, "required") {
		t.Fatalf("expected database-url required error, got %q", err.Error())
	}
}

func TestMigratePocketbaseNonExistentSource(t *testing.T) {
	// Nonexistent source path should fail at analysis, not at flag parsing.
	resetJSONFlag()
	rootCmd.SetArgs([]string{
		"migrate", "pocketbase",
		"--source", "/nonexistent/pb_data",
		"--json",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
	if !strings.Contains(err.Error(), "analysis failed") {
		t.Fatalf("expected 'analysis failed' error, got %q", err.Error())
	}
}

func TestMigratePocketbaseYesFlagHasShorthand(t *testing.T) {
	f := migratePocketbaseCmd.Flags().ShorthandLookup("y")
	if f == nil {
		t.Fatal("expected -y shorthand for --yes flag")
	}
	if f.Name != "yes" {
		t.Fatalf("expected -y to map to 'yes', got %q", f.Name)
	}
}

func TestMigrateHelpDoesNotError(t *testing.T) {
	// All three migrate subcommands should show help without error.
	for _, sub := range []string{"firebase", "supabase", "pocketbase"} {
		t.Run(sub, func(t *testing.T) {
			resetJSONFlag()
			rootCmd.SetArgs([]string{"migrate", sub, "--help"})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("migrate %s --help should not error, got %v", sub, err)
			}
		})
	}
}

// --- Admin command tests ---

func TestAdminSubcommands(t *testing.T) {
	found := make(map[string]bool)
	for _, cmd := range adminCmd.Commands() {
		found[cmd.Name()] = true
	}
	for _, name := range []string{"create", "reset-password"} {
		if !found[name] {
			t.Errorf("expected admin subcommand %q", name)
		}
	}
}

func TestAdminNoRunWithoutSubcommand(t *testing.T) {
	// admin with no subcommand should have no RunE (requires subcommand).
	if adminCmd.RunE != nil {
		t.Error("admin command should not have RunE (requires subcommand)")
	}
}

func TestAdminCreateFlagsMarkedRequired(t *testing.T) {
	// Verify that email and password are marked as required by Cobra.
	for _, name := range []string{"email", "password"} {
		ann := adminCreateCmd.Flags().Lookup(name).Annotations
		if ann == nil {
			t.Errorf("expected flag %q to have annotations (required)", name)
			continue
		}
		if _, ok := ann[cobra.BashCompOneRequiredFlag]; !ok {
			t.Errorf("expected flag %q to be marked required", name)
		}
	}
}

func TestAdminCreateFailsWithoutDatabase(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir) // no ayb.toml
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{
		"admin", "create",
		"--email", "test@example.com",
		"--password", "test1234pass",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without database")
	}
	// Should fail on DB connection, not flag validation.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "database") && !strings.Contains(errMsg, "connecting") {
		t.Fatalf("expected database-related error, got %q", errMsg)
	}
}

func TestAdminCreateFlagDefinitions(t *testing.T) {
	flags := adminCreateCmd.Flags()
	for _, name := range []string{"email", "password", "database-url", "config"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on admin create command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
}

func TestAdminResetPasswordNoServer(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755)

	rootCmd.SetArgs([]string{"admin", "reset-password"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
	if !strings.Contains(err.Error(), "not running") && !strings.Contains(err.Error(), "PID") && !strings.Contains(err.Error(), "connecting") {
		t.Fatalf("expected 'not running', 'PID', or 'connecting' error, got %q", err.Error())
	}
}

// --- Types command tests ---

func TestTypesSubcommands(t *testing.T) {
	found := make(map[string]bool)
	for _, cmd := range typesCmd.Commands() {
		found[cmd.Name()] = true
	}
	if !found["typescript"] {
		t.Error("expected types subcommand 'typescript'")
	}
}

func TestTypesNoRunWithoutSubcommand(t *testing.T) {
	if typesCmd.RunE != nil {
		t.Error("types command should not have RunE (requires subcommand)")
	}
}

func TestTypesTypeScriptRequiresDatabaseURL(t *testing.T) {
	resetJSONFlag()
	// Unset DATABASE_URL and use a temp HOME so no PID file or config is found.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("HOME", t.TempDir())

	rootCmd.SetArgs([]string{"types", "typescript"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --database-url")
	}
	if !strings.Contains(err.Error(), "--database-url is required") {
		t.Fatalf("expected '--database-url is required' error, got %q", err.Error())
	}
}

func TestTypesTypeScriptFlagDefinitions(t *testing.T) {
	flags := typesTypeScriptCmd.Flags()
	for _, tc := range []struct {
		name string
		typ  string
	}{
		{"database-url", "string"},
		{"output", "string"},
	} {
		f := flags.Lookup(tc.name)
		if f == nil {
			t.Errorf("expected flag %q on types typescript command", tc.name)
			continue
		}
		if f.Value.Type() != tc.typ {
			t.Errorf("flag %q should be %s, got %s", tc.name, tc.typ, f.Value.Type())
		}
	}
}

func TestTypesTypeScriptOutputShorthand(t *testing.T) {
	f := typesTypeScriptCmd.Flags().ShorthandLookup("o")
	if f == nil {
		t.Fatal("expected -o shorthand for --output flag")
	}
	if f.Name != "output" {
		t.Fatalf("expected -o to map to 'output', got %q", f.Name)
	}
}

// --- SQL command tests (expanded) ---

func TestSQLCommandFlagDefinitions(t *testing.T) {
	flags := sqlCmd.Flags()
	for _, name := range []string{"admin-token", "url"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on sql command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
}

func TestSQLCommandEmptyQueryFails(t *testing.T) {
	resetJSONFlag()
	// An empty string arg is treated as a non-empty arg so the command
	// will attempt to connect. We verify it gets either a query validation
	// error or a connection error (both exercise the command logic).
	rootCmd.SetArgs([]string{"sql", "", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	msg := err.Error()
	if !strings.Contains(msg, "query") && !strings.Contains(msg, "connect") {
		t.Fatalf("expected query or connection error, got %q", msg)
	}
}

func TestSQLCommandConnectionError(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"sql", "SELECT 1", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

// --- Query command tests (expanded) ---

func TestQueryCommandFlagDefinitions(t *testing.T) {
	flags := queryCmd.Flags()
	stringFlags := []string{"filter", "sort", "fields", "expand", "admin-token", "url"}
	for _, name := range stringFlags {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on query command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
	intFlags := []string{"page", "limit"}
	for _, name := range intFlags {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on query command", name)
			continue
		}
		if f.Value.Type() != "int" {
			t.Errorf("flag %q should be int, got %s", name, f.Value.Type())
		}
	}
}

func TestQueryCommandDefaults(t *testing.T) {
	page, _ := queryCmd.Flags().GetInt("page")
	if page != 1 {
		t.Errorf("expected default page=1, got %d", page)
	}
	limit, _ := queryCmd.Flags().GetInt("limit")
	if limit != 20 {
		t.Errorf("expected default limit=20, got %d", limit)
	}
}

func TestQueryCommandConnectionError(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"query", "posts", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

func TestQueryCommandRejectsTwoArgs(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"query", "table1", "table2"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for too many arguments")
	}
}

// --- Schema command tests (expanded) ---

func TestSchemaCommandFlagDefinitions(t *testing.T) {
	flags := schemaCmd.Flags()
	for _, name := range []string{"admin-token", "url"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on schema command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
}

func TestSchemaCommandConnectionError(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schema", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

// --- Webhooks command tests (expanded) ---

func TestWebhooksCreateFlagDefinitions(t *testing.T) {
	flags := webhooksCreateCmd.Flags()
	stringFlags := []string{"webhook-url", "events", "tables", "secret"}
	for _, name := range stringFlags {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on webhooks create command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
	f := flags.Lookup("disabled")
	if f == nil {
		t.Error("expected flag 'disabled' on webhooks create command")
	} else if f.Value.Type() != "bool" {
		t.Errorf("flag 'disabled' should be bool, got %s", f.Value.Type())
	}
}

func TestWebhooksPersistentFlags(t *testing.T) {
	flags := webhooksCmd.PersistentFlags()
	for _, name := range []string{"admin-token", "url"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected persistent flag %q on webhooks command", name)
		}
	}
}

func TestWebhooksListConnectionError(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"webhooks", "list", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

func TestWebhooksNoRunWithoutSubcommand(t *testing.T) {
	if webhooksCmd.RunE != nil {
		t.Error("webhooks command should not have RunE (requires subcommand)")
	}
}

// --- Users command tests (expanded) ---

func TestUsersListFlagDefinitions(t *testing.T) {
	flags := usersListCmd.Flags()
	f := flags.Lookup("search")
	if f == nil {
		t.Error("expected flag 'search' on users list command")
	} else if f.Value.Type() != "string" {
		t.Errorf("flag 'search' should be string, got %s", f.Value.Type())
	}
	for _, name := range []string{"page", "per-page"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on users list command", name)
			continue
		}
		if f.Value.Type() != "int" {
			t.Errorf("flag %q should be int, got %s", name, f.Value.Type())
		}
	}
}

func TestUsersListDefaults(t *testing.T) {
	page, _ := usersListCmd.Flags().GetInt("page")
	if page != 1 {
		t.Errorf("expected default page=1, got %d", page)
	}
	perPage, _ := usersListCmd.Flags().GetInt("per-page")
	if perPage != 20 {
		t.Errorf("expected default per-page=20, got %d", perPage)
	}
}

func TestUsersPersistentFlags(t *testing.T) {
	flags := usersCmd.PersistentFlags()
	for _, name := range []string{"admin-token", "url"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected persistent flag %q on users command", name)
		}
	}
}

func TestUsersListConnectionError(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"users", "list", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

func TestUsersNoRunWithoutSubcommand(t *testing.T) {
	if usersCmd.RunE != nil {
		t.Error("users command should not have RunE (requires subcommand)")
	}
}

// --- API Keys command tests (expanded) ---

func TestAPIKeysCreateFlagDefinitions(t *testing.T) {
	flags := apikeysCreateCmd.Flags()
	for _, name := range []string{"user-id", "name", "scope"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on apikeys create command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
	f := flags.Lookup("tables")
	if f == nil {
		t.Error("expected flag 'tables' on apikeys create command")
	} else if f.Value.Type() != "stringSlice" {
		t.Errorf("flag 'tables' should be stringSlice, got %s", f.Value.Type())
	}
}

func TestAPIKeysCreateRequiresUserID(t *testing.T) {
	resetJSONFlag()
	// Reset flags that may be stale from other tests.
	apikeysCreateCmd.Flags().Set("user-id", "")
	apikeysCreateCmd.Flags().Set("name", "")
	rootCmd.SetArgs([]string{"apikeys", "create", "--name", "test-key", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --user-id")
	}
	if !strings.Contains(err.Error(), "--user-id is required") {
		t.Fatalf("expected '--user-id is required' error, got %q", err.Error())
	}
}

func TestAPIKeysCreateRequiresName(t *testing.T) {
	resetJSONFlag()
	// Reset flags that may be stale from other tests.
	apikeysCreateCmd.Flags().Set("user-id", "")
	apikeysCreateCmd.Flags().Set("name", "")
	rootCmd.SetArgs([]string{"apikeys", "create", "--user-id", "abc123", "--url", "http://127.0.0.1:1"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --name")
	}
	if !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected '--name is required' error, got %q", err.Error())
	}
}

func TestAPIKeysRevokeRequiresID(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"apikeys", "revoke"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing ID argument")
	}
}

func TestAPIKeysScopeDefault(t *testing.T) {
	scope, _ := apikeysCreateCmd.Flags().GetString("scope")
	if scope != "*" {
		t.Errorf("expected default scope='*', got %q", scope)
	}
}

func TestAPIKeysPersistentFlags(t *testing.T) {
	flags := apikeysCmd.PersistentFlags()
	for _, name := range []string{"admin-token", "url"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected persistent flag %q on apikeys command", name)
		}
	}
}

func TestAPIKeysNoRunWithoutSubcommand(t *testing.T) {
	if apikeysCmd.RunE != nil {
		t.Error("apikeys command should not have RunE (requires subcommand)")
	}
}

// --- MCP command tests (expanded) ---

func TestMCPFlagDefinitions(t *testing.T) {
	flags := mcpCmd.Flags()
	for _, name := range []string{"url", "admin-token", "token"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on mcp command", name)
			continue
		}
		if f.Value.Type() != "string" {
			t.Errorf("flag %q should be string, got %s", name, f.Value.Type())
		}
	}
}

func TestMCPHasRunE(t *testing.T) {
	if mcpCmd.RunE == nil {
		t.Error("mcp command should have RunE")
	}
}

// --- Stats command tests (expanded) ---

func TestStatsConnectionError(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	aybDir := filepath.Join(tmpDir, ".ayb")
	os.MkdirAll(aybDir, 0o755)

	// Use a bogus URL via PID file.
	os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("9999999\n1"), 0o644)

	rootCmd.SetArgs([]string{"stats"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

func TestStatsHasRunE(t *testing.T) {
	if statsCmd.RunE == nil {
		t.Error("stats command should have RunE")
	}
}

// --- Secrets command tests (expanded) ---

func TestSecretsRotateConnectionError(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	aybDir := filepath.Join(tmpDir, ".ayb")
	os.MkdirAll(aybDir, 0o755)

	// Write a PID file pointing to port 1 (unreachable).
	os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("9999999\n1"), 0o644)

	rootCmd.SetArgs([]string{"secrets", "rotate"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

func TestSecretsRotateNoServerNoConfig(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".ayb"), 0o755)

	rootCmd.SetArgs([]string{"secrets", "rotate"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without server or config")
	}
	// serverURL() returns default URL when no PID file, so it'll try to connect
	// and fail. Either a connection error, "not available", or an auth error
	// (when a server happens to be running) is acceptable — the important thing
	// is that the command errors rather than succeeding silently.
	msg := err.Error()
	if !strings.Contains(msg, "connect") && !strings.Contains(msg, "available") && !strings.Contains(msg, "401") && !strings.Contains(msg, "authentication") {
		t.Fatalf("expected connection, availability, or auth error, got %q", msg)
	}
}

// --- Output format tests ---

func TestOutputFormatJSON(t *testing.T) {
	resetJSONFlag()
	rootCmd.PersistentFlags().Set("json", "true")
	defer resetJSONFlag()

	result := outputFormat(rootCmd)
	if result != "json" {
		t.Errorf("expected 'json', got %q", result)
	}
}

func TestOutputFormatCSV(t *testing.T) {
	resetJSONFlag()
	rootCmd.PersistentFlags().Set("output", "csv")
	defer rootCmd.PersistentFlags().Set("output", "table")

	result := outputFormat(rootCmd)
	if result != "csv" {
		t.Errorf("expected 'csv', got %q", result)
	}
}

func TestOutputFormatDefault(t *testing.T) {
	resetJSONFlag()
	rootCmd.PersistentFlags().Set("output", "table")
	result := outputFormat(rootCmd)
	if result != "table" {
		t.Errorf("expected 'table', got %q", result)
	}
}

// --- WriteCSV tests ---

func TestWriteCSV(t *testing.T) {
	var buf strings.Builder
	cols := []string{"Name", "Age"}
	rows := [][]string{
		{"Alice", "30"},
		{"Bob", "25"},
	}
	if err := writeCSV(&buf, cols, rows); err != nil {
		t.Fatalf("writeCSV error: %v", err)
	}
	result := buf.String()
	if !strings.Contains(result, "Name,Age") {
		t.Errorf("expected header in CSV output, got %q", result)
	}
	if !strings.Contains(result, "Alice,30") {
		t.Errorf("expected Alice row in CSV output, got %q", result)
	}
	if !strings.Contains(result, "Bob,25") {
		t.Errorf("expected Bob row in CSV output, got %q", result)
	}
}

func TestWriteCSVEmpty(t *testing.T) {
	var buf strings.Builder
	cols := []string{"ID"}
	if err := writeCSV(&buf, cols, nil); err != nil {
		t.Fatalf("writeCSV error: %v", err)
	}
	result := buf.String()
	if !strings.Contains(result, "ID") {
		t.Errorf("expected header even with no rows, got %q", result)
	}
}

// --- Logs command tests (expanded) ---

func TestLogsShorthandFlags(t *testing.T) {
	// -n shorthand for --lines
	f := logsCmd.Flags().ShorthandLookup("n")
	if f == nil {
		t.Fatal("expected -n shorthand for --lines flag")
	}
	if f.Name != "lines" {
		t.Fatalf("expected -n to map to 'lines', got %q", f.Name)
	}

	// -f shorthand for --follow
	f = logsCmd.Flags().ShorthandLookup("f")
	if f == nil {
		t.Fatal("expected -f shorthand for --follow flag")
	}
	if f.Name != "follow" {
		t.Fatalf("expected -f to map to 'follow', got %q", f.Name)
	}
}

func TestLogsConnectionError(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	aybDir := filepath.Join(tmpDir, ".ayb")
	os.MkdirAll(aybDir, 0o755)

	os.WriteFile(filepath.Join(aybDir, "ayb.pid"), []byte("9999999\n1"), 0o644)

	rootCmd.SetArgs([]string{"logs"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "connecting to server") {
		t.Fatalf("expected connection error, got %q", err.Error())
	}
}

// --- Migrate up/status without database ---

func TestMigrateUpRequiresDatabase(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir) // no ayb.toml
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"migrate", "up"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without database URL")
	}
	msg := err.Error()
	if !strings.Contains(msg, "database") && !strings.Contains(msg, "database-url") {
		t.Fatalf("expected database-related error, got %q", msg)
	}
}

func TestMigrateStatusRequiresDatabase(t *testing.T) {
	resetJSONFlag()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir) // no ayb.toml
	defer os.Chdir(origDir)

	rootCmd.SetArgs([]string{"migrate", "status"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without database URL")
	}
	msg := err.Error()
	if !strings.Contains(msg, "database") && !strings.Contains(msg, "database-url") {
		t.Fatalf("expected database-related error, got %q", msg)
	}
}

// --- ServerError helper ---

func TestServerErrorWithJSON(t *testing.T) {
	body := []byte(`{"message": "not found"}`)
	err := serverError(404, body)
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain status code, got %q", err.Error())
	}
}

func TestServerErrorWithPlainText(t *testing.T) {
	body := []byte("plain text error")
	err := serverError(500, body)
	if !strings.Contains(err.Error(), "plain text error") {
		t.Errorf("expected error to contain body, got %q", err.Error())
	}
}

// --- Start command flag tests ---

func TestStartFlagDefinitions(t *testing.T) {
	flags := startCmd.Flags()
	for _, name := range []string{"database-url", "port", "host", "config", "from"} {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q on start command", name)
		}
	}
}

// --- Help for all commands does not error ---

func TestAllCommandsHelpDoesNotError(t *testing.T) {
	commands := [][]string{
		{"start", "--help"},
		{"stop", "--help"},
		{"status", "--help"},
		{"config", "--help"},
		{"version", "--help"},
		{"migrate", "--help"},
		{"admin", "--help"},
		{"types", "--help"},
		{"sql", "--help"},
		{"query", "--help"},
		{"webhooks", "--help"},
		{"users", "--help"},
		{"storage", "--help"},
		{"schema", "--help"},
		{"rpc", "--help"},
		{"apikeys", "--help"},
		{"mcp", "--help"},
		{"init", "--help"},
		{"db", "--help"},
		{"logs", "--help"},
		{"stats", "--help"},
		{"secrets", "--help"},
		{"uninstall", "--help"},
	}
	for _, args := range commands {
		t.Run(args[0], func(t *testing.T) {
			resetJSONFlag()
			rootCmd.SetArgs(args)
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("%s --help should not error, got %v", args[0], err)
			}
		})
	}
}
