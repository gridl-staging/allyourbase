package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type auditRoundTripFunc func(*http.Request) (*http.Response, error)

func (f auditRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestAuditCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "audit" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'audit' subcommand to be registered")
	}
}

func TestAuditExportJSON(t *testing.T) {
	resetJSONFlag()

	var receivedTable, receivedFrom, receivedTo string
	oldClient := cliHTTPClient
	cliHTTPClient = &http.Client{Transport: auditRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		receivedTable = r.URL.Query().Get("table")
		receivedFrom = r.URL.Query().Get("from")
		receivedTo = r.URL.Query().Get("to")
		body, _ := json.Marshal(map[string]any{
			"items": []map[string]any{
				{
					"id":         "10000000-0000-0000-0000-000000000001",
					"timestamp":  "2026-02-01T10:00:00Z",
					"table_name": "orders",
					"operation":  "DELETE",
				},
			},
			"count": 1,
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}
	defer func() { cliHTTPClient = oldClient }()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"audit", "export",
			"--url", "http://example.test", "--admin-token", "tok",
			"--from", "2026-02-01", "--to", "2026-02-28",
			"--table", "orders", "--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "orders", receivedTable)
	testutil.Equal(t, "2026-02-01", receivedFrom)
	testutil.Equal(t, "2026-02-28", receivedTo)

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "orders", items[0]["table_name"])
}

func TestAuditExportCSV(t *testing.T) {
	resetJSONFlag()

	oldClient := cliHTTPClient
	cliHTTPClient = &http.Client{Transport: auditRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{
			"items": []map[string]any{
				{
					"id":         "10000000-0000-0000-0000-000000000001",
					"timestamp":  "2026-02-01T10:00:00Z",
					"table_name": "orders",
					"operation":  "DELETE",
					"user_id":    "11111111-1111-1111-1111-111111111111",
					"ip_address": "198.51.100.10",
				},
			},
			"count": 1,
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}
	defer func() { cliHTTPClient = oldClient }()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"audit", "export",
			"--url", "http://example.test", "--admin-token", "tok",
			"--from", "2026-02-01", "--to", "2026-02-28",
			"--format", "csv",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "id,timestamp,user_id,api_key_id,table_name,operation,record_id,old_values,new_values,ip_address")
	testutil.Contains(t, output, "10000000-0000-0000-0000-000000000001")
	testutil.Contains(t, output, "orders")
}
