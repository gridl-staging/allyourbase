package cli

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildQueryRequestBuildsURLAndAuthHeader(t *testing.T) {
	req, err := buildQueryRequest(queryRequestConfig{
		table:   "posts",
		token:   "tok",
		baseURL: "http://127.0.0.1:8090",
		filter:  "status='active'",
		sort:    "-created_at",
		fields:  "id,title",
		expand:  "author",
		page:    2,
		limit:   50,
	})
	if err != nil {
		t.Fatalf("buildQueryRequest returned error: %v", err)
	}
	if req.Method != "GET" {
		t.Fatalf("expected GET method, got %q", req.Method)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("expected bearer auth header, got %q", got)
	}

	values := req.URL.Query()
	expect := map[string]string{
		"filter":  "status='active'",
		"sort":    "-created_at",
		"fields":  "id,title",
		"expand":  "author",
		"page":    "2",
		"perPage": "50",
	}
	for key, want := range expect {
		if got := values.Get(key); got != want {
			t.Fatalf("expected %s=%q, got %q", key, want, got)
		}
	}
}

func TestRenderQueryResultsCSV(t *testing.T) {
	payload := []byte(`{"items":[{"id":1,"title":"one","note":null}],"page":1,"perPage":20,"totalItems":1,"totalPages":1}`)

	output := captureStdout(t, func() {
		if err := renderQueryResults(payload, "csv", "id,title,note"); err != nil {
			t.Fatalf("renderQueryResults returned error: %v", err)
		}
	})
	if !strings.Contains(output, "id,title,note") {
		t.Fatalf("expected CSV header, got %q", output)
	}
	if !strings.Contains(output, "1,one,") {
		t.Fatalf("expected CSV row, got %q", output)
	}
}

func TestParseInvokeHeadersValidation(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("header", nil, "")

	headers, err := parseInvokeHeaders(cmd, []string{"bad"})
	if err != nil {
		t.Fatalf("expected implicit invalid headers to be ignored, got error: %v", err)
	}
	if len(headers) != 0 {
		t.Fatalf("expected no headers for implicit invalid header, got %v", headers)
	}

	if err := cmd.Flags().Set("header", "bad"); err != nil {
		t.Fatalf("set header flag: %v", err)
	}
	if _, err := parseInvokeHeaders(cmd, []string{"bad"}); err == nil {
		t.Fatal("expected explicit invalid header to return an error")
	}
}

func TestRenderInvokeResultTextOutput(t *testing.T) {
	body := []byte(`{"statusCode":201,"headers":{"X-B":["2"],"X-A":["1"]},"body":"created"}`)

	output := captureStdout(t, func() {
		if err := renderInvokeResult(body, "text"); err != nil {
			t.Fatalf("renderInvokeResult returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Status: 201") {
		t.Fatalf("expected status line, got %q", output)
	}
	if !strings.Contains(output, "X-A: 1") || !strings.Contains(output, "X-B: 2") {
		t.Fatalf("expected headers, got %q", output)
	}
	if !strings.Contains(output, "Body:\ncreated") {
		t.Fatalf("expected response body, got %q", output)
	}
}

func TestUninstallPreflightNotInstalledJSON(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	aybDir := filepath.Join(homeDir, ".ayb")

	var proceed bool
	output := captureStdout(t, func() {
		var err error
		proceed, err = uninstallPreflight(aybDir, true, false, false)
		if err != nil {
			t.Fatalf("uninstallPreflight returned error: %v", err)
		}
	})
	if proceed {
		t.Fatal("expected preflight to stop when ~/.ayb is missing")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got %q: %v", output, err)
	}
	if parsed["status"] != "not_installed" {
		t.Fatalf("expected status not_installed, got %v", parsed["status"])
	}
}

func TestExecuteRemovalsAndRenderUninstallResult(t *testing.T) {
	homeDir := t.TempDir()
	aybDir := filepath.Join(homeDir, ".ayb")
	binPath := filepath.Join(aybDir, "bin", "ayb")
	if err := os.MkdirAll(filepath.Join(aybDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(aybDir, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	removed, dataPreserved := executeRemovals(aybDir, binPath, false)
	if !dataPreserved {
		t.Fatal("expected data to be preserved for non-purge uninstall")
	}
	if len(removed) == 0 {
		t.Fatal("expected at least one removed path")
	}

	output := captureStdout(t, func() {
		if err := renderUninstallResult(false, removed, nil, dataPreserved, aybDir); err != nil {
			t.Fatalf("renderUninstallResult returned error: %v", err)
		}
	})
	if !strings.Contains(output, "AYB uninstalled.") {
		t.Fatalf("expected uninstall status output, got %q", output)
	}
	if !strings.Contains(output, filepath.Join(aybDir, "data")) {
		t.Fatalf("expected preserved data notice, got %q", output)
	}
}

func TestBuildQueryRequestWithEmptyOptionalValues(t *testing.T) {
	req, err := buildQueryRequest(queryRequestConfig{
		table:   "things",
		baseURL: "http://127.0.0.1:8090",
		page:    1,
		limit:   20,
	})
	if err != nil {
		t.Fatalf("buildQueryRequest returned error: %v", err)
	}
	_, hasAuth := req.Header["Authorization"]
	if hasAuth {
		t.Fatalf("did not expect authorization header when token is empty: %v", req.Header)
	}
	query := req.URL.RawQuery
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if values.Get("page") != "1" || values.Get("perPage") != "20" {
		t.Fatalf("expected pagination query values, got %q", query)
	}
}
