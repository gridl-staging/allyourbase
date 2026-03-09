package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

const testAdminURL = "http://ayb.test"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func stubAdminHandler(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	prevClient := cliHTTPClient
	cliHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler(rec, req)
			return rec.Result(), nil
		}),
	}
	t.Cleanup(func() {
		cliHTTPClient = prevClient
	})
}

// --- Command Registration ---

func TestMatviewsCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "matviews" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'matviews' subcommand to be registered")
	}
}

// --- matviews list ---

func TestMatviewsListTable(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/matviews", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":          "aaaa0000-0000-0000-0000-000000000001",
					"schemaName":  "public",
					"viewName":    "leaderboard",
					"refreshMode": "standard",
					"createdAt":   "2026-02-22T10:00:00Z",
				},
				{
					"id":          "aaaa0000-0000-0000-0000-000000000002",
					"schemaName":  "public",
					"viewName":    "stats",
					"refreshMode": "concurrent",
					"createdAt":   "2026-02-22T11:00:00Z",
				},
			},
			"count": 2,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "list", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "leaderboard")
	testutil.Contains(t, output, "stats")
	testutil.Contains(t, output, "standard")
	testutil.Contains(t, output, "concurrent")
}

func TestMatviewsListJSON(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "aaaa0000-0000-0000-0000-000000000001", "schemaName": "public", "viewName": "leaderboard"},
			},
			"count": 1,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "list", "--url", testAdminURL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
}

func TestMatviewsListEmpty(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "count": 0})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "list", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "No materialized views")
}

// --- matviews register ---

func TestMatviewsRegisterSuccess(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "POST", r.Method)
		testutil.Equal(t, "/api/admin/matviews", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "bbbb0000-0000-0000-0000-000000000001",
			"schemaName":  "public",
			"viewName":    "leaderboard",
			"refreshMode": "standard",
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "register",
			"--url", testAdminURL, "--admin-token", "tok",
			"--view", "leaderboard",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "leaderboard")
	testutil.Contains(t, output, "registered")
	testutil.Equal(t, "leaderboard", receivedBody["viewName"])
	testutil.Equal(t, "public", receivedBody["schema"])
	testutil.Equal(t, "standard", receivedBody["refreshMode"])
}

func TestMatviewsRegisterCustomSchema(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "bbbb0000-0000-0000-0000-000000000002",
			"schemaName":  "analytics",
			"viewName":    "daily_stats",
			"refreshMode": "concurrent",
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "register",
			"--url", testAdminURL, "--admin-token", "tok",
			"--schema", "analytics",
			"--view", "daily_stats",
			"--mode", "concurrent",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "daily_stats")
	testutil.Equal(t, "analytics", receivedBody["schema"])
	testutil.Equal(t, "concurrent", receivedBody["refreshMode"])
}

func TestMatviewsRegisterMissingView(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"matviews", "register",
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "view")
}

func TestMatviewsRegisterInvalidMode(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"matviews", "register",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--view", "test",
		"--mode", "invalid",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "mode")
}

// --- matviews update ---

func TestMatviewsUpdateSuccess(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "PUT", r.Method)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "aaaa0000-0000-0000-0000-000000000001",
			"schemaName":  "public",
			"viewName":    "leaderboard",
			"refreshMode": "concurrent",
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "update",
			"aaaa0000-0000-0000-0000-000000000001",
			"--url", testAdminURL, "--admin-token", "tok",
			"--mode", "concurrent",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "leaderboard")
	testutil.Contains(t, output, "updated")
	testutil.Equal(t, "concurrent", receivedBody["refreshMode"])
}

func TestMatviewsUpdateNotFound(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "matview registration not found"})
	})

	rootCmd.SetArgs([]string{"matviews", "update",
		"99999999-9999-9999-9999-999999999999",
		"--url", testAdminURL, "--admin-token", "tok",
		"--mode", "standard",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
}

func TestMatviewsUpdateInvalidMode(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"matviews", "update",
		"aaaa0000-0000-0000-0000-000000000001",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--mode", "bogus",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "mode")
}

// --- matviews unregister ---

func TestMatviewsUnregisterSuccess(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "DELETE", r.Method)
		w.WriteHeader(http.StatusNoContent)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "unregister",
			"aaaa0000-0000-0000-0000-000000000001",
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "unregistered")
}

func TestMatviewsUnregisterNotFound(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "matview registration not found"})
	})

	rootCmd.SetArgs([]string{"matviews", "unregister",
		"99999999-9999-9999-9999-999999999999",
		"--url", testAdminURL, "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
}

// --- matviews refresh ---

func TestMatviewsRefreshSuccess(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "POST", r.Method)
		testutil.Contains(t, r.URL.Path, "/refresh")
		json.NewEncoder(w).Encode(map[string]any{
			"registration": map[string]any{
				"id":         "aaaa0000-0000-0000-0000-000000000001",
				"schemaName": "public",
				"viewName":   "leaderboard",
			},
			"durationMs": 150,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "refresh",
			"aaaa0000-0000-0000-0000-000000000001",
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "leaderboard")
	testutil.Contains(t, output, "150ms")
}

func TestMatviewsRefreshInProgress(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": "refresh already in progress"})
	})

	rootCmd.SetArgs([]string{"matviews", "refresh",
		"aaaa0000-0000-0000-0000-000000000001",
		"--url", testAdminURL, "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
}

func TestMatviewsRefreshNotFound(t *testing.T) {
	resetJSONFlag()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "matview registration not found"})
	})

	rootCmd.SetArgs([]string{"matviews", "refresh",
		"99999999-9999-9999-9999-999999999999",
		"--url", testAdminURL, "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
}

func TestMatviewsRefreshByQualifiedName(t *testing.T) {
	resetJSONFlag()

	requests := make([]string, 0, 2)
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/admin/matviews":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":         "aaaa0000-0000-0000-0000-000000000001",
						"schemaName": "public",
						"viewName":   "leaderboard",
					},
				},
				"count": 1,
			})
		case r.Method == "POST" && r.URL.Path == "/api/admin/matviews/aaaa0000-0000-0000-0000-000000000001/refresh":
			json.NewEncoder(w).Encode(map[string]any{
				"registration": map[string]any{
					"id":         "aaaa0000-0000-0000-0000-000000000001",
					"schemaName": "public",
					"viewName":   "leaderboard",
				},
				"durationMs": 12,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": "unexpected request"})
		}
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"matviews", "refresh",
			"public.leaderboard",
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, 2, len(requests))
	testutil.Equal(t, "GET /api/admin/matviews", requests[0])
	testutil.Equal(t, "POST /api/admin/matviews/aaaa0000-0000-0000-0000-000000000001/refresh", requests[1])
	testutil.Contains(t, output, "leaderboard")
	testutil.Contains(t, output, "12ms")
}

func TestMatviewsRefreshByQualifiedNameNotFound(t *testing.T) {
	resetJSONFlag()

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/matviews", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":         "aaaa0000-0000-0000-0000-000000000001",
					"schemaName": "public",
					"viewName":   "leaderboard",
				},
			},
			"count": 1,
		})
	})

	rootCmd.SetArgs([]string{"matviews", "refresh",
		"public.missing_mv",
		"--url", testAdminURL, "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "not registered")
}
