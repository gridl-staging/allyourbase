package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSitesCommandRegistered(t *testing.T) {
	var sitesCommand *cobra.Command
	for _, command := range rootCmd.Commands() {
		if command.Name() == "sites" {
			sitesCommand = command
			break
		}
	}
	if sitesCommand == nil {
		t.Fatal("expected 'sites' subcommand to be registered")
	}

	foundDeployCommand := false
	for _, command := range sitesCommand.Commands() {
		if command.Name() == "deploy" {
			foundDeployCommand = true
			break
		}
	}
	if !foundDeployCommand {
		t.Fatal("expected 'sites deploy' subcommand to be registered")
	}
}

func TestSitesDeployRequiresSiteReferenceArgument(t *testing.T) {
	rootCmd.SetArgs([]string{"sites", "deploy"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected missing site reference error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Fatalf("expected exact-args error, got %q", err.Error())
	}
}

func TestSitesDeployRequiresDeployDirectory(t *testing.T) {
	rootCmd.SetArgs([]string{"sites", "deploy", "my-site", "--dir", filepath.Join(t.TempDir(), "missing"), "--url", "http://127.0.0.1:1", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected missing directory error")
	}
	if !strings.Contains(err.Error(), "deploy directory") {
		t.Fatalf("expected deploy directory error, got %q", err.Error())
	}
}

func TestCollectDeployFilesRejectsMissingIndexHTML(t *testing.T) {
	distDir := t.TempDir()
	mustWriteDeployFile(t, distDir, "assets/app.js", []byte("console.log('app')"))

	_, err := collectDeployFiles(distDir)
	if err == nil {
		t.Fatal("expected missing index.html error")
	}
	if !strings.Contains(err.Error(), "must include index.html") {
		t.Fatalf("expected index.html contract error, got %q", err.Error())
	}
}

func TestSitesDeployRejectsSymlinkedFiles(t *testing.T) {
	distDir := t.TempDir()
	mustWriteDeployFile(t, distDir, "index.html", []byte("<h1>Hello</h1>"))

	targetPath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(targetPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	symlinkPath := filepath.Join(distDir, "assets", "leak.txt")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	rootCmd.SetArgs([]string{"sites", "deploy", "my-site", "--dir", distDir, "--url", "http://127.0.0.1:1", "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected symlink rejection error")
	}
	if !strings.Contains(err.Error(), "symlinks are not allowed") {
		t.Fatalf("expected symlink rejection message, got %q", err.Error())
	}
}

func TestSitesDeployUploadsDeterministicallyAndPromotes(t *testing.T) {
	distDir := t.TempDir()
	mustWriteDeployFile(t, distDir, "index.html", []byte("<h1>Hello</h1>"))
	mustWriteDeployFile(t, distDir, "assets/app.js", []byte("console.log('app')"))
	mustWriteDeployFile(t, distDir, "assets/styles.css", []byte("body { color: black; }"))

	requestPaths := []string{}
	uploadedNames := []string{}
	promoteCalled := false
	failCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/sites":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sites": []map[string]any{
					{"id": "site-1", "slug": "my-site"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "siteId": "site-1", "status": "uploading"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/files":
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Fatalf("parse multipart form: %v", err)
			}
			uploadedNames = append(uploadedNames, r.FormValue("name"))
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("read multipart file: %v", err)
			}
			_, _ = io.Copy(io.Discard, file)
			_ = file.Close()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/promote":
			promoteCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "status": "live"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/fail":
			failCalled = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "status": "failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"sites", "deploy", "my-site", "--dir", distDir, "--url", server.URL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected deploy error: %v", err)
		}
	})

	if !strings.Contains(output, "Deployed site") {
		t.Fatalf("expected deploy success output, got %q", output)
	}
	if len(uploadedNames) != 3 {
		t.Fatalf("expected 3 uploaded files, got %d", len(uploadedNames))
	}
	expectedOrder := []string{"assets/app.js", "assets/styles.css", "index.html"}
	for index, expected := range expectedOrder {
		if uploadedNames[index] != expected {
			t.Fatalf("expected deterministic upload order %v, got %v", expectedOrder, uploadedNames)
		}
	}
	if !promoteCalled {
		t.Fatal("expected deploy promote call after successful uploads")
	}
	if failCalled {
		t.Fatal("did not expect fail call on successful deploy")
	}
	if got := requestPaths[len(requestPaths)-1]; got != "POST /api/admin/sites/site-1/deploys/deploy-1/promote" {
		t.Fatalf("expected promote request to be last, got %q", got)
	}
}

func TestSitesDeployFailsDeployOnUploadError(t *testing.T) {
	distDir := t.TempDir()
	mustWriteDeployFile(t, distDir, "index.html", []byte("<h1>Hello</h1>"))

	failCalled := false
	promoteCalled := false
	var failErrorMessage string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/sites":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sites": []map[string]any{
					{"id": "site-1", "slug": "my-site"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "siteId": "site-1", "status": "uploading"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/files":
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "upload exploded"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/fail":
			failCalled = true
			var requestBody struct {
				ErrorMessage string `json:"errorMessage"`
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatalf("decode fail deploy body: %v", err)
			}
			failErrorMessage = requestBody.ErrorMessage
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "status": "failed"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/promote":
			promoteCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "status": "live"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"sites", "deploy", "my-site", "--dir", distDir, "--url", server.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected deploy upload error")
	}
	if !strings.Contains(err.Error(), "upload exploded") {
		t.Fatalf("expected upload server error, got %q", err.Error())
	}
	if !failCalled {
		t.Fatal("expected fail endpoint call after upload error")
	}
	if failErrorMessage == "" {
		t.Fatal("expected non-empty fail errorMessage payload")
	}
	if promoteCalled {
		t.Fatal("did not expect promote call after upload error")
	}
}

func TestSitesDeployResolvesSiteBeyondFirstPage(t *testing.T) {
	distDir := t.TempDir()
	mustWriteDeployFile(t, distDir, "index.html", []byte("<h1>Hello</h1>"))

	pageRequests := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/sites" && r.URL.Query().Get("page") == "1":
			pageRequests = append(pageRequests, "1")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sites": []map[string]any{
					{"id": "site-1", "slug": "first-site"},
				},
				"totalCount": 101,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/sites" && r.URL.Query().Get("page") == "2":
			pageRequests = append(pageRequests, "2")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sites": []map[string]any{
					{"id": "site-101", "slug": "my-site"},
				},
				"totalCount": 101,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-101/deploys":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-101/deploys/deploy-1/files":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-101/deploys/deploy-1/promote":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "status": "live"})
		default:
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"sites", "deploy", "MY-SITE", "--dir", distDir, "--url", server.URL, "--admin-token", "tok"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected deploy error: %v", err)
	}

	if got, want := strings.Join(pageRequests, ","), "1,2"; got != want {
		t.Fatalf("expected paginated site lookup %q, got %q", want, got)
	}
}

func TestSitesDeployFailsDeployOnPromoteError(t *testing.T) {
	distDir := t.TempDir()
	mustWriteDeployFile(t, distDir, "index.html", []byte("<h1>Hello</h1>"))

	failCalled := false
	var failErrorMessage string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/sites":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sites": []map[string]any{
					{"id": "site-1", "slug": "my-site"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "siteId": "site-1", "status": "uploading"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/files":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/promote":
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "promote exploded"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/sites/site-1/deploys/deploy-1/fail":
			failCalled = true
			var requestBody struct {
				ErrorMessage string `json:"errorMessage"`
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatalf("decode fail deploy body: %v", err)
			}
			failErrorMessage = requestBody.ErrorMessage
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "deploy-1", "status": "failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{"sites", "deploy", "my-site", "--dir", distDir, "--url", server.URL, "--admin-token", "tok"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected deploy promote error")
	}
	if !strings.Contains(err.Error(), "promote exploded") {
		t.Fatalf("expected promote server error, got %q", err.Error())
	}
	if !failCalled {
		t.Fatal("expected fail endpoint call after promote error")
	}
	if failErrorMessage == "" {
		t.Fatal("expected non-empty fail errorMessage payload")
	}
}

func mustWriteDeployFile(t *testing.T, distDir, relativePath string, body []byte) {
	t.Helper()
	fullPath := filepath.Join(distDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", fullPath, err)
	}
}
