package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Command Registration ---

func TestJobsCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "jobs" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'jobs' subcommand to be registered")
	}
}

func TestSchedulesCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "schedules" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'schedules' subcommand to be registered")
	}
}

// --- jobs list ---

func TestJobsListTable(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/jobs", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":          "11111111-1111-1111-1111-111111111111",
					"type":        "stale_session_cleanup",
					"state":       "completed",
					"attempts":    1,
					"maxAttempts": 3,
					"createdAt":   "2026-02-22T10:00:00Z",
				},
				{
					"id":          "22222222-2222-2222-2222-222222222222",
					"type":        "webhook_delivery_prune",
					"state":       "queued",
					"attempts":    0,
					"maxAttempts": 3,
					"createdAt":   "2026-02-22T11:00:00Z",
				},
			},
			"count": 2,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"jobs", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "stale_session_cleanup")
	testutil.Contains(t, output, "webhook_delivery_prune")
	testutil.Contains(t, output, "completed")
	testutil.Contains(t, output, "queued")
}

func TestJobsListJSON(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "type": "test", "state": "queued"},
			},
			"count": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"jobs", "list", "--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
}

func TestJobsListFilterState(t *testing.T) {
	resetJSONFlag()
	var receivedState string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedState = r.URL.Query().Get("state")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "count": 0})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"jobs", "list", "--url", srv.URL, "--admin-token", "tok", "--state", "failed"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "failed", receivedState)
}

func TestJobsListFilterType(t *testing.T) {
	resetJSONFlag()
	var receivedType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedType = r.URL.Query().Get("type")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "count": 0})
	}))
	defer srv.Close()

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"jobs", "list", "--url", srv.URL, "--admin-token", "tok", "--type", "stale_session_cleanup"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "stale_session_cleanup", receivedType)
}

// --- jobs retry ---

func TestJobsRetrySuccess(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "33333333-3333-3333-3333-333333333333",
			"type":  "stale_session_cleanup",
			"state": "queued",
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"jobs", "retry", "33333333-3333-3333-3333-333333333333", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "33333333")
	testutil.Contains(t, output, "queued")
}

func TestJobsRetryNotFound(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": "job not found or not in failed state"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"jobs", "retry", "99999999-9999-9999-9999-999999999999", "--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
}

// --- jobs cancel ---

func TestJobsCancelSuccess(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "22222222-2222-2222-2222-222222222222",
			"type":  "webhook_delivery_prune",
			"state": "canceled",
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"jobs", "cancel", "22222222-2222-2222-2222-222222222222", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "22222222")
	testutil.Contains(t, output, "canceled")
}

// --- schedules list ---

func TestSchedulesListTable(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "GET", r.Method)
		testutil.Equal(t, "/api/admin/schedules", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":       "aaaa1111-1111-1111-1111-111111111111",
					"name":     "session_cleanup_hourly",
					"jobType":  "stale_session_cleanup",
					"cronExpr": "0 * * * *",
					"timezone": "UTC",
					"enabled":  true,
				},
			},
			"count": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "list", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "session_cleanup_hourly")
	testutil.Contains(t, output, "0 * * * *")
	testutil.Contains(t, output, "true")
}

func TestSchedulesListJSON(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "aaaa", "name": "test_sched", "cronExpr": "0 * * * *", "enabled": true},
			},
			"count": 1,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "list", "--url", srv.URL, "--admin-token", "tok", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
}

// --- schedules create ---

func TestSchedulesCreateSuccess(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "POST", r.Method)
		testutil.Equal(t, "/api/admin/schedules", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":       "bbbb2222-2222-2222-2222-222222222222",
			"name":     "my_schedule",
			"jobType":  "stale_session_cleanup",
			"cronExpr": "0 */2 * * *",
			"timezone": "UTC",
			"enabled":  true,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "create",
			"--url", srv.URL, "--admin-token", "tok",
			"--name", "my_schedule",
			"--job-type", "stale_session_cleanup",
			"--cron", "0 */2 * * *",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "my_schedule")
	testutil.Equal(t, "my_schedule", receivedBody["name"])
	testutil.Equal(t, "stale_session_cleanup", receivedBody["jobType"])
	testutil.Equal(t, "0 */2 * * *", receivedBody["cronExpr"])
}

func TestSchedulesCreateMissingName(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schedules", "create",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--name", "",
		"--job-type", "test",
		"--cron", "0 * * * *",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "name")
}

func TestSchedulesCreateMissingCron(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schedules", "create",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--name", "test",
		"--job-type", "test",
		"--cron", "",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "cron")
}

// --- schedules enable/disable ---

func TestSchedulesEnableSuccess(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "aaaa1111-1111-1111-1111-111111111111",
			"name":    "test_sched",
			"enabled": true,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "enable", "aaaa1111-1111-1111-1111-111111111111", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "enabled")
}

func TestSchedulesDisableSuccess(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "aaaa1111-1111-1111-1111-111111111111",
			"name":    "test_sched",
			"enabled": false,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "disable", "aaaa1111-1111-1111-1111-111111111111", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "disabled")
}

// --- schedules delete ---

func TestSchedulesDeleteSuccess(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "DELETE", r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "delete", "aaaa1111-1111-1111-1111-111111111111", "--url", srv.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "deleted")
}

func TestSchedulesDeleteNotFound(t *testing.T) {
	resetJSONFlag()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "schedule not found"})
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"schedules", "delete", "99999999-9999-9999-9999-999999999999", "--url", srv.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
}

// --- schedules update ---

func TestSchedulesUpdateSuccess(t *testing.T) {
	resetJSONFlag()
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "PUT", r.Method)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(map[string]any{
			"id":       "aaaa1111-1111-1111-1111-111111111111",
			"name":     "test_sched",
			"cronExpr": "*/5 * * * *",
			"timezone": "America/New_York",
			"enabled":  true,
		})
	}))
	defer srv.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"schedules", "update", "aaaa1111-1111-1111-1111-111111111111",
			"--url", srv.URL, "--admin-token", "tok",
			"--cron", "*/5 * * * *",
			"--timezone", "America/New_York",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "*/5 * * * *")
	testutil.Equal(t, "*/5 * * * *", receivedBody["cronExpr"])
}

func TestSchedulesCreateInvalidPayloadJSON(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schedules", "create",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--name", "test",
		"--job-type", "stale_session_cleanup",
		"--cron", "0 * * * *",
		"--payload", "{invalid",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "invalid --payload JSON")
}

func TestSchedulesUpdateInvalidPayloadJSON(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schedules", "update", "aaaa1111-1111-1111-1111-111111111111",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--payload", "{invalid",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "invalid --payload JSON")
}

func TestSchedulesUpdateInvalidEnabledValue(t *testing.T) {
	resetJSONFlag()
	rootCmd.SetArgs([]string{"schedules", "update", "aaaa1111-1111-1111-1111-111111111111",
		"--url", "http://localhost:0", "--admin-token", "tok",
		"--payload", "{}",
		"--enabled", "maybe",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "invalid --enabled value")
}
