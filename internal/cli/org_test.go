package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOrgCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "org" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'org' subcommand to be registered")
	}
}

func TestOrgListTable(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/admin/orgs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        "org-1",
					"name":      "Acme",
					"slug":      "acme",
					"planTier":  "free",
					"createdAt": "2026-03-13T00:00:00Z",
				},
			},
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"org", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Acme") {
		t.Fatalf("expected org name in output, got %q", output)
	}
	if !strings.Contains(output, "org-1") {
		t.Fatalf("expected org ID in output, got %q", output)
	}
	if !strings.Contains(output, "acme") {
		t.Fatalf("expected org slug in output, got %q", output)
	}
}

func TestOrgMembersAddResolvesSlug(t *testing.T) {
	resetJSONFlag()

	requests := []string{}
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/orgs":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":        "org-1",
						"name":      "Acme",
						"slug":      "acme",
						"planTier":  "free",
						"createdAt": "2026-03-13T00:00:00Z",
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/orgs/org-1/members":
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"userId": "user-1", "role": "admin"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"org", "members", "add", "acme", "user-1",
			"--role", "admin",
			"--url", srv.URL,
			"--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if got := strings.Join(requests, " -> "); !strings.Contains(got, "GET /api/admin/orgs") || !strings.Contains(got, "POST /api/admin/orgs/org-1/members") {
		t.Fatalf("expected slug resolution request flow, got %q", got)
	}
	if receivedBody["userId"] != "user-1" {
		t.Fatalf("expected userId in request body, got %q", receivedBody["userId"])
	}
	if receivedBody["role"] != "admin" {
		t.Fatalf("expected role in request body, got %q", receivedBody["role"])
	}
	if !strings.Contains(output, "Added user user-1 to org acme as admin.") {
		t.Fatalf("expected success output, got %q", output)
	}
}
