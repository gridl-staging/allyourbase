package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/pflag"
)

func TestFunctionsCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "functions" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'functions' subcommand to be registered")
	}
}

func TestFunctionsCommandPersistentFlags(t *testing.T) {
	if f := functionsCmd.PersistentFlags().Lookup("admin-token"); f == nil {
		t.Fatal("expected --admin-token flag on functions command")
	}
	if f := functionsCmd.PersistentFlags().Lookup("url"); f == nil {
		t.Fatal("expected --url flag on functions command")
	}
}

func TestFunctionsListTable(t *testing.T) {
	resetJSONFlag()

	var gotPage string
	var gotPerPage string
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/functions", r.URL.Path)
		gotPage = r.URL.Query().Get("page")
		gotPerPage = r.URL.Query().Get("perPage")

		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":            "11111111-1111-1111-1111-111111111111",
				"name":          "hello-world",
				"public":        true,
				"timeout":       int64(5000000000),
				"lastInvokedAt": "2026-02-24T12:00:00Z",
				"createdAt":     "2026-02-24T08:00:00Z",
			},
			{
				"id":        "22222222-2222-2222-2222-222222222222",
				"name":      "private-task",
				"public":    false,
				"timeout":   int64(2000000000),
				"createdAt": "2026-02-24T09:00:00Z",
			},
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "list",
			"--page", "2",
			"--per-page", "10",
			"--url", testAdminURL,
			"--admin-token", "test-token",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotPage != "2" {
		t.Fatalf("expected page=2, got %q", gotPage)
	}
	if gotPerPage != "10" {
		t.Fatalf("expected perPage=10, got %q", gotPerPage)
	}
	if !strings.Contains(output, "hello-world") {
		t.Fatalf("expected function name in output, got %q", output)
	}
	if !strings.Contains(output, "public") {
		t.Fatalf("expected visibility in output, got %q", output)
	}
	if !strings.Contains(output, "private") {
		t.Fatalf("expected visibility in output, got %q", output)
	}
	if !strings.Contains(output, "5s") {
		t.Fatalf("expected timeout in output, got %q", output)
	}
	if !strings.Contains(output, "2026-02-24T12:00:00Z") {
		t.Fatalf("expected last invoked value in output, got %q", output)
	}
	if !strings.Contains(output, "2026-02-24T08:00:00Z") {
		t.Fatalf("expected created value in output, got %q", output)
	}
}

func TestFunctionsListJSON(t *testing.T) {
	resetJSONFlag()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"11111111-1111-1111-1111-111111111111","name":"hello-world"}]`))
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "list",
			"--url", testAdminURL,
			"--admin-token", "tok",
			"--json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"name":"hello-world"`) {
		t.Fatalf("expected JSON output, got %q", output)
	}
}

func TestFunctionsListEmpty(t *testing.T) {
	resetJSONFlag()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "list", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No edge functions deployed.") {
		t.Fatalf("expected empty-state message, got %q", output)
	}
}

func TestFunctionsListServerError(t *testing.T) {
	resetJSONFlag()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "db down"})
	})

	rootCmd.SetArgs([]string{"functions", "list", "--url", testAdminURL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for server failure")
	}
	if !strings.Contains(err.Error(), "db down") {
		t.Fatalf("expected server error message, got %q", err.Error())
	}
}

func TestFunctionsGetByIDTable(t *testing.T) {
	resetJSONFlag()
	const fnID = "11111111-1111-1111-1111-111111111111"

	var gotPaths []string
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)

		switch r.URL.Path {
		case "/api/admin/functions/" + fnID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            fnID,
				"name":          "hello-world",
				"entryPoint":    "handler",
				"timeout":       int64(5000000000),
				"lastInvokedAt": "2026-02-24T12:00:00Z",
				"envVars": map[string]string{
					"API_KEY": "super-secret",
					"MODE":    "dev",
				},
				"public":    true,
				"source":    "export function handler() { return { statusCode: 200 }; }",
				"createdAt": "2026-02-24T08:00:00Z",
				"updatedAt": "2026-02-24T08:30:00Z",
			})
		case "/api/admin/functions/" + fnID + "/triggers/db":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "db-1"}, {"id": "db-2"}})
		case "/api/admin/functions/" + fnID + "/triggers/cron":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "cron-1"}})
		case "/api/admin/functions/" + fnID + "/triggers/storage":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "get", fnID, "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	wantPaths := []string{
		"/api/admin/functions/" + fnID,
		"/api/admin/functions/" + fnID + "/triggers/db",
		"/api/admin/functions/" + fnID + "/triggers/cron",
		"/api/admin/functions/" + fnID + "/triggers/storage",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("unexpected request paths: got %v want %v", gotPaths, wantPaths)
	}
	if !strings.Contains(output, "Name: hello-world") {
		t.Fatalf("expected function name in output, got %q", output)
	}
	if !strings.Contains(output, "Trigger Count: 3") {
		t.Fatalf("expected trigger count in output, got %q", output)
	}
	if !strings.Contains(output, "API_KEY=********") {
		t.Fatalf("expected masked env var output, got %q", output)
	}
	if strings.Contains(output, "super-secret") {
		t.Fatalf("expected env var value to be masked, got %q", output)
	}
	if !strings.Contains(output, "Source:") {
		t.Fatalf("expected source heading in output, got %q", output)
	}
}

func TestFunctionsGetByNameUsesByNameRoute(t *testing.T) {
	resetJSONFlag()
	const fnID = "22222222-2222-2222-2222-222222222222"
	const name = "hello-world"

	var gotGetPath string
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/") {
			gotGetPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         fnID,
				"name":       name,
				"entryPoint": "handler",
				"timeout":    int64(5000000000),
				"public":     false,
				"source":     "function handler() {}",
				"createdAt":  "2026-02-24T10:00:00Z",
				"updatedAt":  "2026-02-24T10:10:00Z",
			})
			return
		}

		switch r.URL.Path {
		case "/api/admin/functions/" + fnID + "/triggers/db":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/api/admin/functions/" + fnID + "/triggers/cron":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/api/admin/functions/" + fnID + "/triggers/storage":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "get", name, "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	expectedPath := "/api/admin/functions/by-name/" + name
	if gotGetPath != expectedPath {
		t.Fatalf("expected by-name path %q, got %q", expectedPath, gotGetPath)
	}
	if !strings.Contains(output, "Name: "+name) {
		t.Fatalf("expected function name in output, got %q", output)
	}
}

func TestFunctionsGetJSONMasksEnvVarsAndAddsTriggerCount(t *testing.T) {
	resetJSONFlag()
	const fnID = "33333333-3333-3333-3333-333333333333"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/functions/" + fnID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         fnID,
				"name":       "json-fn",
				"entryPoint": "handler",
				"timeout":    int64(2000000000),
				"envVars": map[string]string{
					"TOKEN": "raw-secret",
				},
				"public":    true,
				"source":    "function handler() {}",
				"createdAt": "2026-02-24T10:00:00Z",
				"updatedAt": "2026-02-24T10:05:00Z",
			})
		case "/api/admin/functions/" + fnID + "/triggers/db":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "db-1"}})
		case "/api/admin/functions/" + fnID + "/triggers/cron":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "cron-1"}})
		case "/api/admin/functions/" + fnID + "/triggers/storage":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "st-1"}})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "get", fnID, "--url", testAdminURL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("expected valid JSON output: %v", err)
	}

	envVars, ok := got["envVars"].(map[string]any)
	if !ok {
		t.Fatalf("expected envVars object in JSON output, got %T", got["envVars"])
	}
	if envVars["TOKEN"] != "********" {
		t.Fatalf("expected masked env var value, got %v", envVars["TOKEN"])
	}
	if got["triggerCount"] != float64(3) {
		t.Fatalf("expected triggerCount=3, got %v", got["triggerCount"])
	}
}

func TestFunctionsGetServerError(t *testing.T) {
	resetJSONFlag()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
	})

	rootCmd.SetArgs([]string{
		"functions", "get", "does-not-exist",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing function")
	}
	if !strings.Contains(err.Error(), "function not found") {
		t.Fatalf("expected server error message, got %q", err.Error())
	}
}

func TestFunctionsListInvalidPageZero(t *testing.T) {
	resetJSONFlag()

	rootCmd.SetArgs([]string{
		"functions", "list",
		"--page", "0",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for page=0")
	}
	if !strings.Contains(err.Error(), "--page must be greater than 0") {
		t.Fatalf("expected page validation error, got %q", err.Error())
	}
}

func TestFunctionsListInvalidPerPageZero(t *testing.T) {
	resetJSONFlag()

	rootCmd.SetArgs([]string{
		"functions", "list",
		"--page", "1",
		"--per-page", "0",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for per-page=0")
	}
	if !strings.Contains(err.Error(), "--per-page must be greater than 0") {
		t.Fatalf("expected per-page validation error, got %q", err.Error())
	}
}

func TestResolveFunctionsGetPath_URLEncodesSpecialChars(t *testing.T) {
	// UUID input should use direct ID path
	uuidPath := resolveFunctionsGetPath("11111111-1111-1111-1111-111111111111")
	if uuidPath != "/api/admin/functions/11111111-1111-1111-1111-111111111111" {
		t.Fatalf("expected UUID path, got %q", uuidPath)
	}

	// Name with special chars should be URL-escaped in by-name path
	specialPath := resolveFunctionsGetPath("my func/special")
	if !strings.Contains(specialPath, "/by-name/") {
		t.Fatalf("expected by-name path, got %q", specialPath)
	}
	if strings.Contains(specialPath, " ") || strings.Contains(specialPath, "my func/special") {
		t.Fatalf("expected URL-encoded special chars, got %q", specialPath)
	}
	// url.PathEscape encodes space as %20 and slash as %2F
	if !strings.Contains(specialPath, "%20") {
		t.Fatalf("expected space encoded as %%20, got %q", specialPath)
	}
	if !strings.Contains(specialPath, "%2F") {
		t.Fatalf("expected slash encoded as %%2F, got %q", specialPath)
	}

	// Simple name should use by-name path unescaped
	simplePath := resolveFunctionsGetPath("hello-world")
	if simplePath != "/api/admin/functions/by-name/hello-world" {
		t.Fatalf("expected simple by-name path, got %q", simplePath)
	}
}

func TestFunctionsGetTableNoEnvVarsAndNoLastInvoked(t *testing.T) {
	resetJSONFlag()
	const fnID = "55555555-5555-5555-5555-555555555555"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/admin/functions/"+fnID:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         fnID,
				"name":       "bare-fn",
				"entryPoint": "handler",
				"timeout":    int64(3000000000),
				"public":     false,
				"source":     "function handler() {}",
				"createdAt":  "2026-02-24T08:00:00Z",
				"updatedAt":  "2026-02-24T08:05:00Z",
			})
		case strings.HasSuffix(r.URL.Path, "/triggers/db"):
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case strings.HasSuffix(r.URL.Path, "/triggers/cron"):
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case strings.HasSuffix(r.URL.Path, "/triggers/storage"):
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "get", fnID, "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Env Vars: (none)") {
		t.Fatalf("expected (none) for empty env vars, got %q", output)
	}
	if !strings.Contains(output, "Last Invoked: -") {
		t.Fatalf("expected '-' for nil LastInvokedAt, got %q", output)
	}
	if !strings.Contains(output, "Trigger Count: 0") {
		t.Fatalf("expected trigger count 0, got %q", output)
	}
}

func resetFunctionsNewFlags() {
	_ = functionsNewCmd.Flags().Set("typescript", "false")
}

func TestFunctionsNewCreatesJavaScriptTemplateByDefault(t *testing.T) {
	resetJSONFlag()
	resetFunctionsNewFlags()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "new", "hello-world"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	jsPath := filepath.Join(tmpDir, "hello-world.js")
	content, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("expected %s to be created: %v", jsPath, err)
	}
	if strings.Contains(string(content), ": string") {
		t.Fatalf("expected JS template without TypeScript annotations, got %q", string(content))
	}
	if !strings.Contains(string(content), "export default function handler(req)") {
		t.Fatalf("expected handler boilerplate in JS template, got %q", string(content))
	}
	if !strings.Contains(output, "Created edge function scaffold") {
		t.Fatalf("expected success output, got %q", output)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "hello-world.ts")); err == nil {
		t.Fatal("did not expect TypeScript file for default scaffold")
	}
}

func TestFunctionsNewCreatesTypeScriptTemplateWithFlag(t *testing.T) {
	resetJSONFlag()
	resetFunctionsNewFlags()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"functions", "new", "typed-fn", "--typescript"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	tsPath := filepath.Join(tmpDir, "typed-fn.ts")
	content, err := os.ReadFile(tsPath)
	if err != nil {
		t.Fatalf("expected %s to be created: %v", tsPath, err)
	}
	if !strings.Contains(string(content), "type EdgeRequest = {") {
		t.Fatalf("expected TypeScript type definition in template, got %q", string(content))
	}
	if !strings.Contains(string(content), "export default function handler(req: EdgeRequest)") {
		t.Fatalf("expected typed handler boilerplate, got %q", string(content))
	}
	if !strings.Contains(output, "Created edge function scaffold") {
		t.Fatalf("expected success output, got %q", output)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "typed-fn.js")); err == nil {
		t.Fatal("did not expect JavaScript file when --typescript is used")
	}
}

func TestFunctionsNewRejectsExistingTargetFile(t *testing.T) {
	resetJSONFlag()
	resetFunctionsNewFlags()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	existingPath := filepath.Join(tmpDir, "already-there.js")
	original := []byte("keep me")
	if err := os.WriteFile(existingPath, original, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	rootCmd.SetArgs([]string{"functions", "new", "already-there"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when target scaffold file already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing-file error, got %q", err.Error())
	}

	content, readErr := os.ReadFile(existingPath)
	if readErr != nil {
		t.Fatalf("reading existing file after failed scaffold: %v", readErr)
	}
	if string(content) != string(original) {
		t.Fatalf("expected existing file content to remain unchanged, got %q", string(content))
	}
}

func TestFunctionsNewRejectsPathLikeName(t *testing.T) {
	resetJSONFlag()
	resetFunctionsNewFlags()
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.MkdirAll(filepath.Join(tmpDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	rootCmd.SetArgs([]string{"functions", "new", "nested/name"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for path-like function name")
	}
	if !strings.Contains(err.Error(), "must not contain path separators") {
		t.Fatalf("expected path-separator validation error, got %q", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "nested", "name.js")); statErr == nil {
		t.Fatal("did not expect scaffold file to be created in nested directory")
	}
}

// --- Deploy tests ---

func resetFunctionsDeployFlags() {
	_ = functionsDeployCmd.Flags().Set("source", "")
	_ = functionsDeployCmd.Flags().Set("entry-point", "")
	_ = functionsDeployCmd.Flags().Set("timeout", "0")
	_ = functionsDeployCmd.Flags().Set("public", "false")
	_ = functionsDeployCmd.Flags().Set("private", "false")
	for _, name := range []string{"source", "entry-point", "timeout", "public", "private"} {
		if f := functionsDeployCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

func TestFunctionsDeployCreatesNewFunction(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	srcContent := `export default function handler(req) { return { statusCode: 200 }; }`
	if err := os.WriteFile(srcFile, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	const createdID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var gotMethod string
	var gotPath string
	var gotBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			// Function does not exist yet → 404
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions":
			gotMethod = r.Method
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   createdID,
				"name": "myfunc",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "deploy", "myfunc",
			"--source", srcFile,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "POST", gotMethod)
	testutil.Equal(t, "/api/admin/functions", gotPath)
	if gotBody["name"] != "myfunc" {
		t.Fatalf("expected name=myfunc in body, got %v", gotBody["name"])
	}
	if gotBody["source"] != srcContent {
		t.Fatalf("expected source in body, got %v", gotBody["source"])
	}
	if !strings.Contains(output, createdID) {
		t.Fatalf("expected function ID in output, got %q", output)
	}
	if !strings.Contains(output, "Created") {
		t.Fatalf("expected 'Created' status in output, got %q", output)
	}
}

func TestFunctionsDeployUpdatesExistingFunction(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	srcContent := `export default function handler(req) { return { statusCode: 201 }; }`
	if err := os.WriteFile(srcFile, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	const existingID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	var gotMethod string
	var gotPath string

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			// Function exists
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   existingID,
				"name": "myfunc",
			})
		case r.Method == "PUT" && r.URL.Path == "/api/admin/functions/"+existingID:
			gotMethod = r.Method
			gotPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   existingID,
				"name": "myfunc",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "deploy", "myfunc",
			"--source", srcFile,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "PUT", gotMethod)
	testutil.Equal(t, "/api/admin/functions/"+existingID, gotPath)
	if !strings.Contains(output, existingID) {
		t.Fatalf("expected function ID in output, got %q", output)
	}
	if !strings.Contains(output, "Updated") {
		t.Fatalf("expected 'Updated' status in output, got %q", output)
	}
}

func TestFunctionsDeployPassesFlags(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.ts")
	if err := os.WriteFile(srcFile, []byte("export default function main() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	var gotBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "cccccccc-cccc-cccc-cccc-cccccccccccc",
				"name": "myfunc",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "deploy", "myfunc",
			"--source", srcFile,
			"--entry-point", "main",
			"--timeout", "10000",
			"--public",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotBody["entry_point"] != "main" {
		t.Fatalf("expected entry_point=main, got %v", gotBody["entry_point"])
	}
	if gotBody["timeout_ms"] != float64(10000) {
		t.Fatalf("expected timeout_ms=10000, got %v", gotBody["timeout_ms"])
	}
	if gotBody["public"] != true {
		t.Fatalf("expected public=true, got %v", gotBody["public"])
	}
}

func TestFunctionsDeployPrivateFlag(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	if err := os.WriteFile(srcFile, []byte("export default function handler() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	var gotBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "dddddddd-dddd-dddd-dddd-dddddddddddd",
				"name": "myfunc",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "deploy", "myfunc",
			"--source", srcFile,
			"--private",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotBody["public"] != false {
		t.Fatalf("expected public=false with --private flag, got %v", gotBody["public"])
	}
}

func TestFunctionsDeployRejectsConflictingVisibilityFlags(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	if err := os.WriteFile(srcFile, []byte("export default function handler() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("did not expect API call for conflicting flags, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "deploy", "myfunc",
		"--source", srcFile,
		"--public",
		"--private",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --public and --private are set")
	}
	if !strings.Contains(err.Error(), "cannot use --public and --private together") {
		t.Fatalf("expected conflicting-flags error, got %q", err.Error())
	}
}

func TestFunctionsDeployUpdatePreservesVisibilityWhenFlagsUnset(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	if err := os.WriteFile(srcFile, []byte("export default function handler() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	const existingID = "99999999-9999-9999-9999-999999999999"
	var gotBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     existingID,
				"name":   "myfunc",
				"public": true,
			})
		case r.Method == "PUT" && r.URL.Path == "/api/admin/functions/"+existingID:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   existingID,
				"name": "myfunc",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "deploy", "myfunc",
			"--source", srcFile,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotBody["public"] != true {
		t.Fatalf("expected public=true to be preserved on update, got %v", gotBody["public"])
	}
}

func TestFunctionsDeployLookupNonNotFoundReturnsError(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	if err := os.WriteFile(srcFile, []byte("export default function handler() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/") {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "lookup failed"})
			return
		}
		t.Fatalf("did not expect fallback create request, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "deploy", "myfunc",
		"--source", srcFile,
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when lookup fails")
	}
	if !strings.Contains(err.Error(), "lookup failed") {
		t.Fatalf("expected lookup failure in error, got %q", err.Error())
	}
}

func TestFunctionsDeployMissingSourceFlag(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	rootCmd.SetArgs([]string{
		"functions", "deploy", "myfunc",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --source is missing")
	}
	if !strings.Contains(err.Error(), "--source") {
		t.Fatalf("expected --source error message, got %q", err.Error())
	}
}

func TestFunctionsDeploySourceFileNotFound(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	rootCmd.SetArgs([]string{
		"functions", "deploy", "myfunc",
		"--source", "/nonexistent/file.js",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when source file does not exist")
	}
	if !strings.Contains(err.Error(), "reading source file") {
		t.Fatalf("expected source file error, got %q", err.Error())
	}
}

func TestFunctionsDeployServerError(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "bad.js")
	if err := os.WriteFile(srcFile, []byte("syntax error {{"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
		case r.Method == "POST":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "transpilation failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	rootCmd.SetArgs([]string{
		"functions", "deploy", "bad",
		"--source", srcFile,
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for server deploy failure")
	}
	if !strings.Contains(err.Error(), "transpilation failed") {
		t.Fatalf("expected transpilation error, got %q", err.Error())
	}
}

func TestFunctionsDeployJSONOutput(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	if err := os.WriteFile(srcFile, []byte("export default function handler() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	const createdID = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
		case r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   createdID,
				"name": "myfunc",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "deploy", "myfunc",
			"--source", srcFile,
			"--json",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("expected valid JSON output: %v", err)
	}
	if got["id"] != createdID {
		t.Fatalf("expected id=%s in JSON, got %v", createdID, got["id"])
	}
}

func TestFunctionsDeployConflictError(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeployFlags()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "myfunc.js")
	if err := os.WriteFile(srcFile, []byte("export default function handler() {}"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
		case r.Method == "POST":
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function name already exists"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	rootCmd.SetArgs([]string{
		"functions", "deploy", "myfunc",
		"--source", srcFile,
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for conflict")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected conflict error, got %q", err.Error())
	}
}

// --- Delete tests ---

func resetFunctionsDeleteFlags() {
	_ = functionsDeleteCmd.Flags().Set("force", "false")
}

func setCommandInput(input string) {
	rootCmd.SetIn(strings.NewReader(input))
}

func resetCommandInput() {
	rootCmd.SetIn(nil)
}

func TestFunctionsDeleteByIDWithForce(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()
	resetCommandInput()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var gotMethod string
	var gotPath string

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/api/admin/functions/"+fnID {
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "delete", fnID,
			"--force",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "DELETE", gotMethod)
	testutil.Equal(t, "/api/admin/functions/"+fnID, gotPath)
	if !strings.Contains(output, "Deleted") {
		t.Fatalf("expected 'Deleted' in output, got %q", output)
	}
}

func TestFunctionsDeleteByNameWithForce(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()
	resetCommandInput()

	const fnID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	var deleteCalled bool

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/my-fn":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   fnID,
				"name": "my-fn",
			})
		case r.Method == "DELETE" && r.URL.Path == "/api/admin/functions/"+fnID:
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "delete", "my-fn",
			"--force",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("expected DELETE request to be made")
	}
	if !strings.Contains(output, "Deleted") {
		t.Fatalf("expected 'Deleted' in output, got %q", output)
	}
}

func TestFunctionsDeleteWithoutForceRequiresConfirmation(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()
	resetCommandInput()

	// Without --force, the command should fail with a prompt message
	// (since there's no interactive stdin in tests)
	rootCmd.SetArgs([]string{
		"functions", "delete", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error without --force flag")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force hint in error, got %q", err.Error())
	}
}

func TestFunctionsDeleteWithoutForcePromptsAndDeletesOnYes(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()
	setCommandInput("yes\n")
	defer resetCommandInput()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var deleteCalled bool

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/api/admin/functions/"+fnID {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			rootCmd.SetArgs([]string{
				"functions", "delete", fnID,
				"--url", testAdminURL,
				"--admin-token", "tok",
			})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})

	if !deleteCalled {
		t.Fatal("expected delete request after confirmation")
	}
	if !strings.Contains(stderr, "Delete edge function") {
		t.Fatalf("expected confirmation prompt, got %q", stderr)
	}
	if !strings.Contains(stdout, "Deleted function") {
		t.Fatalf("expected deleted message, got %q", stdout)
	}
}

func TestFunctionsDeleteWithoutForceCancelsOnNo(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()
	setCommandInput("n\n")
	defer resetCommandInput()

	var called bool
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("did not expect API request on cancellation: %s %s", r.Method, r.URL.Path)
	})

	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "delete", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if called {
		t.Fatal("expected no API calls when deletion is cancelled")
	}
	if !strings.Contains(stderr, "Deletion cancelled.") {
		t.Fatalf("expected cancellation message, got %q", stderr)
	}
}

func TestFunctionsDeleteNotFound(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
	})

	rootCmd.SetArgs([]string{
		"functions", "delete", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"--force",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "function not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestFunctionsDeleteNameLookupFails(t *testing.T) {
	resetJSONFlag()
	resetFunctionsDeleteFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "delete", "nonexistent",
		"--force",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for name lookup failure")
	}
	if !strings.Contains(err.Error(), "function not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

// --- Invoke tests ---

func resetFunctionsInvokeFlags() {
	_ = functionsInvokeCmd.Flags().Set("method", "GET")
	_ = functionsInvokeCmd.Flags().Set("path", "")
	_ = functionsInvokeCmd.Flags().Set("body", "")
	_ = functionsInvokeCmd.Flags().Set("body-file", "")
	for _, name := range []string{"method", "path", "body", "body-file"} {
		if f := functionsInvokeCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
	if f := functionsInvokeCmd.Flags().Lookup("header"); f != nil {
		f.Changed = false
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			sv.Replace([]string{})
		}
	}
}

func TestFunctionsInvokeByName(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var gotInvokeBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/hello":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "hello"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions/"+fnID+"/invoke":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotInvokeBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 200,
				"headers":    map[string]any{"Content-Type": []string{"application/json"}},
				"body":       `{"message":"ok"}`,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "invoke", "hello",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Default method is GET
	if gotInvokeBody["method"] != "GET" {
		t.Fatalf("expected method GET, got %v", gotInvokeBody["method"])
	}
	if !strings.Contains(output, "200") {
		t.Fatalf("expected status 200 in output, got %q", output)
	}
	if !strings.Contains(output, `{"message":"ok"}`) {
		t.Fatalf("expected response body in output, got %q", output)
	}
}

func TestFunctionsInvokeByID(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var invokeCalled bool

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/admin/functions/"+fnID+"/invoke" {
			invokeCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 200,
				"body":       "ok",
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "invoke", fnID,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !invokeCalled {
		t.Fatal("expected invoke API to be called")
	}
	if !strings.Contains(output, "200") {
		t.Fatalf("expected status 200 in output, got %q", output)
	}
}

func TestFunctionsInvokeWithFlags(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var gotInvokeBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/myfn":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "myfn"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions/"+fnID+"/invoke":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotInvokeBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 201,
				"body":       "created",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "invoke", "myfn",
			"--method", "POST",
			"--path", "/api/data",
			"--header", "X-Custom:value1",
			"--header", "Authorization:Bearer tok123",
			"--body", `{"key":"val"}`,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotInvokeBody["method"] != "POST" {
		t.Fatalf("expected method POST, got %v", gotInvokeBody["method"])
	}
	if gotInvokeBody["path"] != "/api/data" {
		t.Fatalf("expected path /api/data, got %v", gotInvokeBody["path"])
	}
	if gotInvokeBody["body"] != `{"key":"val"}` {
		t.Fatalf("expected body in payload, got %v", gotInvokeBody["body"])
	}
	// Check headers were sent
	headers, ok := gotInvokeBody["headers"].(map[string]any)
	if !ok {
		t.Fatalf("expected headers map, got %T", gotInvokeBody["headers"])
	}
	xCustom, ok := headers["X-Custom"].([]any)
	if !ok || len(xCustom) == 0 || xCustom[0] != "value1" {
		t.Fatalf("expected X-Custom header value1, got %v", headers["X-Custom"])
	}

	if !strings.Contains(output, "201") {
		t.Fatalf("expected status 201 in output, got %q", output)
	}
}

func TestFunctionsInvokeBodyFile(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	tmpDir := t.TempDir()
	bodyFile := filepath.Join(tmpDir, "body.json")
	if err := os.WriteFile(bodyFile, []byte(`{"from":"file"}`), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var gotInvokeBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/myfn":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "myfn"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions/"+fnID+"/invoke":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotInvokeBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 200,
				"body":       "ok",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "invoke", "myfn",
			"--method", "POST",
			"--body-file", bodyFile,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotInvokeBody["body"] != `{"from":"file"}` {
		t.Fatalf("expected body from file, got %v", gotInvokeBody["body"])
	}
}

func TestFunctionsInvokeBodyAndBodyFileMutuallyExclusive(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	rootCmd.SetArgs([]string{
		"functions", "invoke", "myfn",
		"--body", "inline",
		"--body-file", "/some/file",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for --body + --body-file")
	}
	if !strings.Contains(err.Error(), "cannot use --body and --body-file together") {
		t.Fatalf("expected mutual exclusivity error, got %q", err.Error())
	}
}

func TestFunctionsInvokeRejectsInvalidMethod(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("did not expect API call for invalid method, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "invoke", "hello",
		"--method", "TRACE",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --method value")
	}
	if !strings.Contains(err.Error(), "--method must be one of") {
		t.Fatalf("expected method validation error, got %q", err.Error())
	}
}

func TestFunctionsInvokeRejectsEmptyHeaderName(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("did not expect API call for invalid header, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "invoke", "hello",
		"--header", ":value",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for empty header name")
	}
	if !strings.Contains(err.Error(), "header name must not be empty") {
		t.Fatalf("expected header validation error, got %q", err.Error())
	}
}

func TestFunctionsInvokeJSONOutput(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/admin/functions/"+fnID+"/invoke" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 200,
				"headers":    map[string]any{"X-Resp": []string{"val"}},
				"body":       "hello",
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "invoke", fnID,
			"--json",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got %q", output)
	}
	if parsed["statusCode"].(float64) != 200 {
		t.Fatalf("expected statusCode 200, got %v", parsed["statusCode"])
	}
}

func TestFunctionsInvokeServerError(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/crash":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "crash"})
		case r.Method == "POST" && r.URL.Path == "/api/admin/functions/"+fnID+"/invoke":
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function execution failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	rootCmd.SetArgs([]string{
		"functions", "invoke", "crash",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for server error")
	}
	if !strings.Contains(err.Error(), "function execution failed") {
		t.Fatalf("expected execution error, got %q", err.Error())
	}
}

func TestFunctionsInvokeNotFound(t *testing.T) {
	resetJSONFlag()
	resetFunctionsInvokeFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "invoke", "nonexistent",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "function not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

// --- Logs tests ---

func resetFunctionsLogsFlags() {
	_ = functionsLogsCmd.Flags().Set("status", "")
	_ = functionsLogsCmd.Flags().Set("trigger-type", "")
	_ = functionsLogsCmd.Flags().Set("limit", "50")
	_ = functionsLogsCmd.Flags().Set("follow", "false")
	for _, name := range []string{"status", "trigger-type", "limit", "follow"} {
		if f := functionsLogsCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

func TestFunctionsLogsRejectsInvalidStatus(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("did not expect API call for invalid status, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "logs", "myfn",
		"--status", "maybe",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --status value")
	}
	if !strings.Contains(err.Error(), "--status must be one of") {
		t.Fatalf("expected status validation error, got %q", err.Error())
	}
}

func TestFunctionsLogsRejectsInvalidTriggerType(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("did not expect API call for invalid trigger type, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "logs", "myfn",
		"--trigger-type", "queue",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --trigger-type value")
	}
	if !strings.Contains(err.Error(), "--trigger-type must be one of") {
		t.Fatalf("expected trigger-type validation error, got %q", err.Error())
	}
}

func TestFunctionsLogsRejectsNonPositiveLimit(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("did not expect API call for invalid limit, got %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "logs", "myfn",
		"--limit", "-1",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-positive --limit")
	}
	if !strings.Contains(err.Error(), "--limit must be greater than 0") {
		t.Fatalf("expected limit validation error, got %q", err.Error())
	}
}

func TestFunctionsLogsFollowPollsWithSinceCursor(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()
	origPollInterval := functionsLogsFollowPollInterval
	origMaxPolls := functionsLogsFollowMaxPolls
	functionsLogsFollowPollInterval = 5 * time.Millisecond
	functionsLogsFollowMaxPolls = 3
	defer func() {
		functionsLogsFollowPollInterval = origPollInterval
		functionsLogsFollowMaxPolls = origMaxPolls
	}()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	const firstLogTime = "2026-02-24T12:00:00Z"
	const secondLogTime = "2026-02-24T12:00:02Z"

	var logCalls int
	var sinceParams []string

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/stream":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "stream"})
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/"+fnID+"/logs":
			logCalls++
			sinceParams = append(sinceParams, r.URL.Query().Get("since"))

			switch logCalls {
			case 1:
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":            "11111111-1111-1111-1111-111111111111",
						"status":        "success",
						"durationMs":    11,
						"triggerType":   "http",
						"requestMethod": "GET",
						"requestPath":   "/first",
						"createdAt":     firstLogTime,
					},
				})
			case 2:
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":            "22222222-2222-2222-2222-222222222222",
						"status":        "error",
						"durationMs":    17,
						"triggerType":   "cron",
						"requestMethod": "POST",
						"requestPath":   "/second",
						"error":         "boom",
						"createdAt":     secondLogTime,
					},
				})
			default:
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "logs", "stream",
			"--follow",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if logCalls < 2 {
		t.Fatalf("expected at least 2 polls in follow mode, got %d", logCalls)
	}
	if len(sinceParams) < 2 {
		t.Fatalf("expected at least 2 since values, got %v", sinceParams)
	}
	if sinceParams[0] != "" {
		t.Fatalf("expected empty since in first poll, got %q", sinceParams[0])
	}
	if sinceParams[1] != firstLogTime {
		t.Fatalf("expected since to advance to first log time %q, got %q", firstLogTime, sinceParams[1])
	}
	if !strings.Contains(output, "success") || !strings.Contains(output, "error") {
		t.Fatalf("expected follow output to include both streamed logs, got %q", output)
	}
}

func TestFunctionsLogsTable(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/hello":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "hello"})
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/"+fnID+"/logs":
			testutil.Equal(t, "50", r.URL.Query().Get("limit"))
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":            "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
					"functionId":    fnID,
					"invocationId":  "cccccccc-cccc-cccc-cccc-cccccccccccc",
					"status":        "success",
					"durationMs":    42,
					"triggerType":   "http",
					"requestMethod": "GET",
					"requestPath":   "/hello",
					"createdAt":     "2026-02-24T12:00:00Z",
				},
				{
					"id":            "dddddddd-dddd-dddd-dddd-dddddddddddd",
					"functionId":    fnID,
					"invocationId":  "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
					"status":        "error",
					"durationMs":    120,
					"error":         "ReferenceError: x is not defined",
					"triggerType":   "cron",
					"requestMethod": "",
					"requestPath":   "",
					"createdAt":     "2026-02-24T11:00:00Z",
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "logs", "hello",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Check table columns present
	if !strings.Contains(output, "success") {
		t.Fatalf("expected 'success' status in output, got %q", output)
	}
	if !strings.Contains(output, "error") {
		t.Fatalf("expected 'error' status in output, got %q", output)
	}
	if !strings.Contains(output, "http") {
		t.Fatalf("expected 'http' trigger type in output, got %q", output)
	}
	if !strings.Contains(output, "cron") {
		t.Fatalf("expected 'cron' trigger type in output, got %q", output)
	}
	if !strings.Contains(output, "42") {
		t.Fatalf("expected duration 42 in output, got %q", output)
	}
}

func TestFunctionsLogsWithFilters(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var gotQuery string

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/by-name/myfn":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": fnID, "name": "myfn"})
		case r.Method == "GET" && r.URL.Path == "/api/admin/functions/"+fnID+"/logs":
			gotQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "logs", "myfn",
			"--status", "error",
			"--trigger-type", "cron",
			"--limit", "10",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(gotQuery, "status=error") {
		t.Fatalf("expected status=error in query, got %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "trigger_type=cron") {
		t.Fatalf("expected trigger_type=cron in query, got %q", gotQuery)
	}
	if !strings.Contains(gotQuery, "limit=10") {
		t.Fatalf("expected limit=10 in query, got %q", gotQuery)
	}
	if !strings.Contains(output, "No logs") {
		t.Fatalf("expected empty state message, got %q", output)
	}
}

func TestFunctionsLogsJSONOutput(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/admin/functions/"+fnID+"/logs" {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "status": "success"},
			})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "logs", fnID,
			"--json",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var parsed []map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON array, got %q", output)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(parsed))
	}
}

func TestFunctionsLogsNotFound(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/admin/functions/by-name/") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "function not found"})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	rootCmd.SetArgs([]string{
		"functions", "logs", "ghost",
		"--url", testAdminURL,
		"--admin-token", "tok",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "function not found") {
		t.Fatalf("expected not found error, got %q", err.Error())
	}
}

func TestFunctionsLogsByID(t *testing.T) {
	resetJSONFlag()
	resetFunctionsLogsFlags()

	const fnID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var logsCalled bool

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/admin/functions/"+fnID+"/logs" {
			logsCalled = true
			_ = json.NewEncoder(w).Encode([]map[string]any{})
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"functions", "logs", fnID,
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !logsCalled {
		t.Fatal("expected logs API to be called directly with UUID")
	}
}
