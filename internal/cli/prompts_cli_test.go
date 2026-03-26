package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func resetPromptsFlags() {
	resetJSONFlag()
	rootCmd.PersistentFlags().Set("output", "table")
}

// --- prompts list ---

func TestPromptsListTable(t *testing.T) {
	resetPromptsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/admin/ai/prompts" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"prompts": []map[string]any{
				{"id": "abc-123", "name": "greeting", "version": 1, "model": "gpt-4o", "provider": "openai"},
				{"id": "def-456", "name": "summary", "version": 2, "model": "", "provider": ""},
			},
			"total": 2,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompts", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "greeting") {
		t.Errorf("expected 'greeting' in output, got %q", output)
	}
	if !strings.Contains(output, "summary") {
		t.Errorf("expected 'summary' in output, got %q", output)
	}
	if !strings.Contains(output, "2 prompt(s)") {
		t.Errorf("expected count in output, got %q", output)
	}
}

func TestPromptsListEmpty(t *testing.T) {
	resetPromptsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"prompts": []any{}, "total": 0})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompts", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No prompts") {
		t.Errorf("expected empty message, got %q", output)
	}
}

func TestPromptsListJSON(t *testing.T) {
	resetPromptsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"prompts":[{"id":"abc","name":"greeting","version":1}],"total":1}`))
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompts", "list", "--json", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, `"name"`) {
		t.Errorf("expected JSON output, got %q", output)
	}
}

// --- prompts get ---

func TestPromptsGetSuccess(t *testing.T) {
	resetPromptsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/admin/ai/prompts/abc-123" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "abc-123", "name": "greeting", "version": 1,
			"template": "Hello {{name}}", "model": "gpt-4o", "provider": "openai",
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"prompts", "get", "abc-123", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "greeting") {
		t.Errorf("expected name in output, got %q", output)
	}
	if !strings.Contains(output, "Hello {{name}}") {
		t.Errorf("expected template in output, got %q", output)
	}
}

func TestPromptsGetNotFound(t *testing.T) {
	resetPromptsFlags()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "prompt not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"prompts", "get", "nope", "--url", srv.URL, "--admin-token", "tok"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestPromptsGetRequiresArg(t *testing.T) {
	resetPromptsFlags()
	rootCmd.SetArgs([]string{"prompts", "get", "--url", "http://localhost:0", "--admin-token", "tok"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for missing arg")
	}
}

// --- prompts create ---

func TestPromptsCreateSuccess(t *testing.T) {
	resetPromptsFlags()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/admin/ai/prompts" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "new-id", "name": "greeting"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"prompts", "create",
			"--name", "greeting",
			"--template", "Hello {{name}}",
			"--model", "gpt-4o",
			"--url", srv.URL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if gotBody["name"] != "greeting" {
		t.Errorf("name = %v", gotBody["name"])
	}
	if gotBody["template"] != "Hello {{name}}" {
		t.Errorf("template = %v", gotBody["template"])
	}
	if gotBody["model"] != "gpt-4o" {
		t.Errorf("model = %v", gotBody["model"])
	}
	if !strings.Contains(output, "created") {
		t.Errorf("expected creation message, got %q", output)
	}
}

func TestPromptsCreateRequiresNameAndTemplate(t *testing.T) {
	resetPromptsFlags()
	rootCmd.SetArgs([]string{"prompts", "create", "--name", "foo",
		"--url", "http://localhost:0", "--admin-token", "tok"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for missing template")
	}
}

// --- prompts delete ---

func TestPromptsDeleteSuccess(t *testing.T) {
	resetPromptsFlags()
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
		rootCmd.SetArgs([]string{"prompts", "delete", "abc-123", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if deletedPath != "/api/admin/ai/prompts/abc-123" {
		t.Errorf("unexpected path: %s", deletedPath)
	}
	if !strings.Contains(output, "deleted") {
		t.Errorf("expected deleted message, got %q", output)
	}
}

func TestPromptsDeleteRequiresArg(t *testing.T) {
	resetPromptsFlags()
	rootCmd.SetArgs([]string{"prompts", "delete", "--url", "http://localhost:0", "--admin-token", "tok"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for missing arg")
	}
}

// --- prompts render ---

func TestPromptsRenderSuccess(t *testing.T) {
	resetPromptsFlags()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/ai/prompts/abc-123/render" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]string{"rendered": "Hello World"})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"prompts", "render", "abc-123",
			"--var", "name=World",
			"--url", srv.URL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	vars, _ := gotBody["variables"].(map[string]any)
	if vars["name"] != "World" {
		t.Errorf("variables[name] = %v", vars["name"])
	}
	if !strings.Contains(output, "Hello World") {
		t.Errorf("expected rendered output, got %q", output)
	}
}

func TestPromptsRenderInvalidVar(t *testing.T) {
	resetPromptsFlags()
	rootCmd.SetArgs([]string{
		"prompts", "render", "abc-123",
		"--var", "notakeyvalue",
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for bad var format")
	}
}

// --- prompts command registration ---

func TestPromptsSubcommandsRegistered(t *testing.T) {
	resetPromptsFlags()
	want := map[string]bool{
		"list":   false,
		"get":    false,
		"create": false,
		"delete": false,
		"render": false,
	}
	for _, cmd := range promptsCmd.Commands() {
		if _, ok := want[cmd.Name()]; ok {
			want[cmd.Name()] = true
		}
	}
	for sub, found := range want {
		if !found {
			t.Fatalf("expected '%s' subcommand under 'prompts'", sub)
		}
	}
}

func TestPromptsRegisteredInRoot(t *testing.T) {
	resetPromptsFlags()
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "prompts" {
			return
		}
	}
	t.Fatal("expected 'prompts' command registered in root")
}
