package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func resetFDWOutputFlags() {
	resetJSONFlag()
	_ = rootCmd.PersistentFlags().Set("output", "table")
}

func TestDBFDWCommandsRegistered(t *testing.T) {
	resetFDWOutputFlags()
	t.Cleanup(resetFDWOutputFlags)

	var fdwCmd *cobra.Command
	for _, cmd := range dbCmd.Commands() {
		if cmd.Name() == "fdw" {
			fdwCmd = cmd
			break
		}
	}
	testutil.NotNil(t, fdwCmd)

	want := map[string]bool{
		"list-servers":  false,
		"create-server": false,
		"import-tables": false,
		"drop-server":   false,
	}
	for _, sub := range fdwCmd.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, ok := range want {
		testutil.True(t, ok, "missing subcommand %s", name)
	}
}

func TestDBFDWListServersTableAndJSONOutput(t *testing.T) {
	resetFDWOutputFlags()
	t.Cleanup(resetFDWOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodGet, r.Method)
		testutil.Equal(t, "/api/admin/fdw/servers", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"servers": []map[string]any{
				{
					"name":       "analytics_fdw",
					"fdw_type":   "postgres_fdw",
					"created_at": "2026-03-05T10:00:00Z",
				},
			},
		})
	})

	tableOut := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "fdw", "list-servers", "--url", testAdminURL, "--admin-token", "tok"})
		testutil.NoError(t, rootCmd.Execute())
	})
	testutil.Contains(t, tableOut, "analytics_fdw")
	testutil.Contains(t, tableOut, "postgres_fdw")
	testutil.Contains(t, tableOut, "2026-03-05T10:00:00Z")

	jsonOut := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "fdw", "list-servers", "--url", testAdminURL, "--admin-token", "tok", "--output", "json"})
		testutil.NoError(t, rootCmd.Execute())
	})
	var payload map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(jsonOut), &payload))
	testutil.NotNil(t, payload["servers"])
}

func TestDBFDWCreateServerPostgresRequest(t *testing.T) {
	resetFDWOutputFlags()
	t.Cleanup(resetFDWOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/fdw/servers", r.URL.Path)

		var body map[string]any
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		testutil.Equal(t, "analytics_fdw", body["name"])
		testutil.Equal(t, "postgres_fdw", body["fdw_type"])

		options, _ := body["options"].(map[string]any)
		testutil.Equal(t, "localhost", options["host"])
		testutil.Equal(t, "5432", options["port"])
		testutil.Equal(t, "app", options["dbname"])

		userMapping, _ := body["user_mapping"].(map[string]any)
		testutil.Equal(t, "report_user", userMapping["user"])
		testutil.Equal(t, "secret", userMapping["password"])

		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "analytics_fdw",
			"type": "postgres_fdw",
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "fdw", "create-server", "analytics_fdw",
			"--type", "postgres_fdw",
			"--host", "localhost",
			"--port", "5432",
			"--dbname", "app",
			"--user", "report_user",
			"--password", "secret",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})
	testutil.Contains(t, output, "analytics_fdw")
}

func TestDBFDWImportTablesRequest(t *testing.T) {
	resetFDWOutputFlags()
	t.Cleanup(resetFDWOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/fdw/servers/analytics_fdw/import", r.URL.Path)

		var body map[string]any
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		testutil.Equal(t, "public", body["remote_schema"])
		testutil.Equal(t, "local", body["local_schema"])
		tables, _ := body["table_names"].([]any)
		testutil.Equal(t, 2, len(tables))

		_ = json.NewEncoder(w).Encode(map[string]any{
			"tables": []map[string]any{{"schema": "local", "name": "events", "server_name": "analytics_fdw"}},
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"db", "fdw", "import-tables", "analytics_fdw",
			"--schema", "public",
			"--local-schema", "local",
			"--tables", "events,users",
			"--url", testAdminURL,
			"--admin-token", "tok",
		})
		testutil.NoError(t, rootCmd.Execute())
	})
	testutil.Contains(t, output, "events")
}

func TestDBFDWDropServerCascadeRequest(t *testing.T) {
	resetFDWOutputFlags()
	t.Cleanup(resetFDWOutputFlags)

	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodDelete, r.Method)
		testutil.Equal(t, "/api/admin/fdw/servers/analytics_fdw", r.URL.Path)
		testutil.Equal(t, "true", r.URL.Query().Get("cascade"))
		w.WriteHeader(http.StatusNoContent)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"db", "fdw", "drop-server", "analytics_fdw", "--cascade", "--url", testAdminURL, "--admin-token", "tok"})
		testutil.NoError(t, rootCmd.Execute())
	})
	testutil.True(t, strings.Contains(output, "analytics_fdw") || strings.TrimSpace(output) == "")
}
