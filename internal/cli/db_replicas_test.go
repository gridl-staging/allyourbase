package cli

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func resetReplicaOutputFlags() {
	resetJSONFlag()
	_ = rootCmd.PersistentFlags().Set("output", "table")
	_ = dbReplicasAddCmd.Flags().Set("name", "")
	_ = dbReplicasAddCmd.Flags().Set("host", "")
	_ = dbReplicasAddCmd.Flags().Set("port", "5432")
	_ = dbReplicasAddCmd.Flags().Set("database", "")
	_ = dbReplicasAddCmd.Flags().Set("ssl-mode", "")
	_ = dbReplicasAddCmd.Flags().Set("weight", "1")
	_ = dbReplicasAddCmd.Flags().Set("max-lag-bytes", "10485760")
	_ = dbReplicasRemoveCmd.Flags().Set("force", "false")
	_ = dbReplicasFailoverCmd.Flags().Set("target", "")
	_ = dbReplicasFailoverCmd.Flags().Set("force", "false")
}

func TestDBReplicasCommandsRegistered(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	var replicas *cobra.Command
	for _, cmd := range dbCmd.Commands() {
		if cmd.Name() == "replicas" {
			replicas = cmd
			break
		}
	}
	testutil.NotNil(t, replicas)

	var hasList bool
	var hasCheck bool
	var hasAdd bool
	var hasRemove bool
	var hasPromote bool
	var hasFailover bool
	for _, cmd := range replicas.Commands() {
		switch cmd.Name() {
		case "list":
			hasList = true
		case "check":
			hasCheck = true
		case "add":
			hasAdd = true
		case "remove":
			hasRemove = true
		case "promote":
			hasPromote = true
		case "failover":
			hasFailover = true
		}
	}
	testutil.True(t, hasList)
	testutil.True(t, hasCheck)
	testutil.True(t, hasAdd)
	testutil.True(t, hasRemove)
	testutil.True(t, hasPromote)
	testutil.True(t, hasFailover)
}

func TestDBReplicasListTableOutput(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodGet, r.Method)
		testutil.Equal(t, "/api/admin/replicas", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"replicas": []map[string]any{
				{
					"url":             "postgres://replica-1.local:5432/appdb",
					"state":           "healthy",
					"lag_bytes":       128,
					"last_checked_at": "2026-03-04T18:00:00Z",
				},
			},
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "replicas", "list", "--url", testAdminURL, "--admin-token", "tok"})
		testutil.NoError(t, rootCmd.Execute())
	})

	testutil.Contains(t, out, "replica-1.local")
	testutil.Contains(t, out, "healthy")
	testutil.Contains(t, out, "128")
}

func TestDBReplicasListJSONOutput(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"replicas": []map[string]any{
				{
					"url":             "postgres://replica-1.local:5432/appdb",
					"state":           "healthy",
					"lag_bytes":       0,
					"last_checked_at": "2026-03-04T18:00:00Z",
				},
			},
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "replicas", "list", "--url", testAdminURL, "--admin-token", "tok", "--output", "json"})
		testutil.NoError(t, rootCmd.Execute())
	})

	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(out), &payload))
	testutil.NotNil(t, payload["replicas"])
}

func TestDBReplicasCheckTableAndJSONOutput(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/replicas/check", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"replicas": []map[string]any{
				{
					"url":             "postgres://replica-2.local:5432/appdb",
					"state":           "suspect",
					"lag_bytes":       4096,
					"last_checked_at": "2026-03-04T18:01:00Z",
				},
			},
		})
	})

	tableOut := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "replicas", "check", "--url", testAdminURL, "--admin-token", "tok"})
		testutil.NoError(t, rootCmd.Execute())
	})
	testutil.Contains(t, tableOut, "replica-2.local")
	testutil.Contains(t, tableOut, "suspect")
	testutil.Contains(t, tableOut, "4096")

	jsonOut := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "replicas", "check", "--url", testAdminURL, "--admin-token", "tok", "--json"})
		testutil.NoError(t, rootCmd.Execute())
	})
	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(jsonOut), &payload))
	testutil.NotNil(t, payload["replicas"])
}

func TestDBReplicasAddRequest(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/replicas", r.URL.Path)
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]any
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		testutil.Equal(t, "replica-3", body["name"])
		testutil.Equal(t, "replica-3.local", body["host"])
		testutil.Equal(t, float64(6432), body["port"].(float64))
		testutil.Equal(t, "appdb", body["database"])
		testutil.Equal(t, "require", body["ssl_mode"])
		testutil.Equal(t, float64(2), body["weight"].(float64))
		testutil.Equal(t, float64(2048), body["max_lag_bytes"].(float64))

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "added",
			"record": map[string]any{
				"name":          "replica-3",
				"host":          "replica-3.local",
				"port":          6432,
				"database":      "appdb",
				"ssl_mode":      "require",
				"weight":        2,
				"max_lag_bytes": 2048,
				"role":          "replica",
				"state":         "active",
			},
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "add",
			"--name", "replica-3",
			"--host", "replica-3.local",
			"--port", "6432",
			"--database", "appdb",
			"--ssl-mode", "require",
			"--weight", "2",
			"--max-lag-bytes", "2048",
			"--url", testAdminURL,
			"--admin-token", "tok",
			"--json",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(out), &payload))
	record := payload["record"].(map[string]any)
	testutil.Equal(t, "replica-3", record["name"])
}

func TestDBReplicasAddHumanOutput(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "added",
			"record": map[string]any{
				"name":          "replica-3",
				"host":          "replica-3.local",
				"port":          6432,
				"database":      "appdb",
				"ssl_mode":      "require",
				"weight":        2,
				"max_lag_bytes": 2048,
				"role":          "replica",
				"state":         "active",
			},
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "add",
			"--name", "replica-3",
			"--host", "replica-3.local",
			"--database", "appdb",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	testutil.Contains(t, out, `Replica "replica-3" added`)
}

func TestDBReplicasRemoveRequest(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodDelete, r.Method)
		testutil.Equal(t, "/api/admin/replicas/replica%2Fa", r.URL.EscapedPath())
		testutil.Equal(t, "true", r.URL.Query().Get("force"))
		w.WriteHeader(http.StatusNoContent)
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "remove", "replica/a",
			"--force",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	testutil.Contains(t, out, `Replica "replica/a" removed`)
}

func TestDBReplicasRemoveRequestIncludesForceFalseByDefault(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodDelete, r.Method)
		testutil.Equal(t, "/api/admin/replicas/replica-b", r.URL.Path)
		testutil.Equal(t, "false", r.URL.Query().Get("force"))
		w.WriteHeader(http.StatusNoContent)
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "remove", "replica-b",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	testutil.Contains(t, out, `Replica "replica-b" removed`)
}

func TestDBReplicasPromoteRequest(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/replicas/replica%20primary/promote", r.URL.EscapedPath())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "promoted",
			"primary": map[string]any{
				"name":     "replica primary",
				"host":     "replica-primary.local",
				"port":     5432,
				"database": "appdb",
				"ssl_mode": "disable",
				"role":     "primary",
				"state":    "active",
			},
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "promote", "replica primary",
			"--url", testAdminURL,
			"--admin-token", "tok",
			"--json",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(out), &payload))
	testutil.Equal(t, "promoted", payload["status"])
	primary := payload["primary"].(map[string]any)
	testutil.Equal(t, "replica primary", primary["name"])
}

func TestDBReplicasPromoteHumanOutput(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "promoted",
			"primary": map[string]any{
				"name":     "replica primary",
				"host":     "replica-primary.local",
				"port":     5432,
				"database": "appdb",
				"ssl_mode": "disable",
				"role":     "primary",
				"state":    "active",
			},
			"replicas": []map[string]any{
				{
					"url":             "postgres://replica-2.local:5432/appdb",
					"state":           "healthy",
					"lag_bytes":       32,
					"last_checked_at": "2026-03-04T18:02:00Z",
				},
			},
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "promote", "replica primary",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	testutil.Contains(t, out, `Primary promoted to "replica primary"`)
	testutil.Contains(t, out, "replica-2.local")
}

func TestDBReplicasFailoverRequest(t *testing.T) {
	resetReplicaOutputFlags()
	t.Cleanup(resetReplicaOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/replicas/failover", r.URL.Path)
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]any
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		testutil.Equal(t, "replica-9", body["target"])
		testutil.Equal(t, true, body["force"])

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "failover_complete",
		})
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "replicas", "failover",
			"--target", "replica-9",
			"--force",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})

	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(out), &payload))
	testutil.Equal(t, "failover_complete", payload["status"])
}
