package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func resetEmailTemplateFlags() {
	resetJSONFlag()
	_ = emailTemplatesSetCmd.Flags().Set("subject", "")
	_ = emailTemplatesSetCmd.Flags().Set("html-file", "")
	_ = emailTemplatesPreviewCmd.Flags().Set("vars", "{}")
	_ = emailTemplatesPreviewCmd.Flags().Set("subject", "")
	_ = emailTemplatesPreviewCmd.Flags().Set("html-file", "")
	_ = emailTemplatesSendCmd.Flags().Set("to", "")
	_ = emailTemplatesSendCmd.Flags().Set("vars", "{}")
}

func TestEmailTemplatesCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "email-templates" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'email-templates' subcommand to be registered")
	}
}

func TestEmailTemplatesListTable(t *testing.T) {
	resetEmailTemplateFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/email/templates", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"templateKey":     "auth.password_reset",
					"source":          "builtin",
					"subjectTemplate": "Reset your password",
					"enabled":         true,
				},
				{
					"templateKey":     "app.club_invite",
					"source":          "custom",
					"subjectTemplate": "Join {{.ClubName}}",
					"enabled":         false,
				},
			},
			"count": 2,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "list", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "auth.password_reset")
	testutil.Contains(t, output, "app.club_invite")
	testutil.Contains(t, output, "builtin")
	testutil.Contains(t, output, "custom")
}

func TestEmailTemplatesListJSON(t *testing.T) {
	resetEmailTemplateFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/email/templates", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"templateKey": "auth.password_reset",
					"source":      "builtin",
					"enabled":     true,
				},
			},
			"count": 1,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "list", "--json", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "auth.password_reset", items[0]["templateKey"])
}

func TestEmailTemplatesGet(t *testing.T) {
	resetEmailTemplateFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/email/templates/auth.password_reset", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"source":          "builtin",
			"templateKey":     "auth.password_reset",
			"subjectTemplate": "Reset your password",
			"htmlTemplate":    "<p>Reset link: {{.ActionURL}}</p>",
			"enabled":         true,
			"variables":       []string{"AppName", "ActionURL"},
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "get", "auth.password_reset", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "auth.password_reset")
	testutil.Contains(t, output, "Reset your password")
	testutil.Contains(t, output, "ActionURL")
}

func TestEmailTemplatesGetJSON(t *testing.T) {
	resetEmailTemplateFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/email/templates/auth.password_reset", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"source":          "builtin",
			"templateKey":     "auth.password_reset",
			"subjectTemplate": "Reset your password",
			"htmlTemplate":    "<p>Reset link: {{.ActionURL}}</p>",
			"enabled":         true,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "get", "auth.password_reset", "--json", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &resp))
	testutil.Equal(t, "auth.password_reset", resp["templateKey"])
	testutil.Equal(t, "builtin", resp["source"])
}

func TestEmailTemplatesSetReadsHTMLFileAndSendsPayload(t *testing.T) {
	resetEmailTemplateFlags()
	htmlFile := t.TempDir() + "/invite.html"
	testutil.NoError(t, os.WriteFile(htmlFile, []byte("<p>Invite {{.Name}}</p>"), 0o600))

	var payload map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "PUT", r.Method)
		testutil.Equal(t, "/api/admin/email/templates/app.club_invite", r.URL.Path)
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"templateKey":     "app.club_invite",
			"subjectTemplate": "You're invited",
			"htmlTemplate":    "<p>Invite {{.Name}}</p>",
			"enabled":         true,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"email-templates", "set", "app.club_invite",
			"--subject", "You're invited",
			"--html-file", htmlFile,
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "You're invited", payload["subjectTemplate"])
	testutil.Equal(t, "<p>Invite {{.Name}}</p>", payload["htmlTemplate"])
	testutil.Contains(t, output, "updated")
}

func TestEmailTemplatesSetRequiresHTMLFile(t *testing.T) {
	resetEmailTemplateFlags()
	rootCmd.SetArgs([]string{
		"email-templates", "set", "app.club_invite",
		"--subject", "You're invited",
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "--html-file is required")
}

func TestEmailTemplatesDelete(t *testing.T) {
	resetEmailTemplateFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "DELETE", r.Method)
		testutil.Equal(t, "/api/admin/email/templates/app.club_invite", r.URL.Path)
		w.WriteHeader(204)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "delete", "app.club_invite", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	testutil.Contains(t, output, "deleted")
}

func TestEmailTemplatesPreviewUsesEffectiveTemplateWhenNoOverridesProvided(t *testing.T) {
	resetEmailTemplateFlags()

	requests := make([]string, 0, 2)
	var previewBody map[string]any

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/email/templates/auth.password_reset":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"source":          "builtin",
				"templateKey":     "auth.password_reset",
				"subjectTemplate": "Reset {{.AppName}}",
				"htmlTemplate":    "<p>{{.ActionURL}}</p>",
				"enabled":         true,
			})
		case r.Method == "POST" && r.URL.Path == "/api/admin/email/templates/auth.password_reset/preview":
			testutil.NoError(t, json.NewDecoder(r.Body).Decode(&previewBody))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"subject": "Reset Sigil",
				"html":    "<p>https://example.test/reset</p>",
				"text":    "https://example.test/reset",
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"email-templates", "preview", "auth.password_reset",
			"--vars", `{"AppName":"Sigil","ActionURL":"https://example.test/reset"}`,
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, 2, len(requests))
	testutil.Equal(t, "GET /api/admin/email/templates/auth.password_reset", requests[0])
	testutil.Equal(t, "POST /api/admin/email/templates/auth.password_reset/preview", requests[1])
	testutil.Equal(t, "Reset {{.AppName}}", previewBody["subjectTemplate"])
	testutil.Equal(t, "<p>{{.ActionURL}}</p>", previewBody["htmlTemplate"])
	vars, ok := previewBody["variables"].(map[string]any)
	testutil.True(t, ok, "variables should be object")
	testutil.Equal(t, "Sigil", vars["AppName"])
	testutil.Contains(t, output, "Reset Sigil")
}

func TestEmailTemplatesPreviewInvalidVarsJSON(t *testing.T) {
	resetEmailTemplateFlags()
	rootCmd.SetArgs([]string{
		"email-templates", "preview", "auth.password_reset",
		"--vars", `{"AppName":123}`,
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.True(t, strings.Contains(err.Error(), "invalid --vars JSON"),
		"expected vars JSON validation error, got: %v", err)
}

func TestEmailTemplatesPreviewWithExplicitOverridesSkipsEffectiveTemplateGet(t *testing.T) {
	resetEmailTemplateFlags()
	htmlFile := t.TempDir() + "/preview.html"
	testutil.NoError(t, os.WriteFile(htmlFile, []byte("<p>Hello {{.Name}}</p>"), 0o600))

	requests := make([]string, 0, 1)
	var previewBody map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method != "POST" || r.URL.Path != "/api/admin/email/templates/app.club_invite/preview" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&previewBody))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subject": "Invite Alex",
			"html":    "<p>Hello Alex</p>",
			"text":    "Hello Alex",
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"email-templates", "preview", "app.club_invite",
			"--subject", "Invite {{.Name}}",
			"--html-file", htmlFile,
			"--vars", `{"Name":"Alex"}`,
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, 1, len(requests))
	testutil.Equal(t, "POST /api/admin/email/templates/app.club_invite/preview", requests[0])
	testutil.Equal(t, "Invite {{.Name}}", previewBody["subjectTemplate"])
	testutil.Equal(t, "<p>Hello {{.Name}}</p>", previewBody["htmlTemplate"])
	testutil.Contains(t, output, "Invite Alex")
}

func TestEmailTemplatesEnableDisable(t *testing.T) {
	resetEmailTemplateFlags()

	requests := make([]bool, 0, 2)
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "PATCH", r.Method)
		var body map[string]any
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		enabled, _ := body["enabled"].(bool)
		requests = append(requests, enabled)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"templateKey": "app.club_invite",
			"enabled":     enabled,
		})
	})

	outputEnable := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "enable", "app.club_invite", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	testutil.Contains(t, outputEnable, "enabled")

	outputDisable := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"email-templates", "disable", "app.club_invite", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	testutil.Contains(t, outputDisable, "disabled")

	testutil.Equal(t, 2, len(requests))
	testutil.True(t, requests[0], "first PATCH should set enabled=true")
	testutil.True(t, !requests[1], "second PATCH should set enabled=false")
}

func TestEmailTemplatesSendParsesVarsJSON(t *testing.T) {
	resetEmailTemplateFlags()

	var payload map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "POST", r.Method)
		testutil.Equal(t, "/api/admin/email/send", r.URL.Path)
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "sent"})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"email-templates", "send", "app.club_invite",
			"--to", "user@example.com",
			"--vars", `{"Name":"Alex"}`,
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "app.club_invite", payload["templateKey"])
	testutil.Equal(t, "user@example.com", payload["to"])
	vars, ok := payload["variables"].(map[string]any)
	testutil.True(t, ok, "variables should be object")
	testutil.Equal(t, "Alex", vars["Name"])
	testutil.Contains(t, output, "sent")
}

func TestEmailTemplatesSendInvalidVarsJSON(t *testing.T) {
	resetEmailTemplateFlags()
	rootCmd.SetArgs([]string{
		"email-templates", "send", "app.club_invite",
		"--to", "user@example.com",
		"--vars", `{"Name":123}`,
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.True(t, strings.Contains(err.Error(), "invalid --vars JSON"),
		"expected vars JSON validation error, got: %v", err)
}
