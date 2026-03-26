//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Stage 5 sites-deploy helpers
// ---------------------------------------------------------------------------

// uniqueSiteSlug returns a slug safe for use in a shared database, incorporating
// a stage prefix, sanitized test name, and nanosecond timestamp.
func uniqueSiteSlug(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("s5-%s-%d", sanitizeTestName(t, "-"), time.Now().UnixNano())
}

// createSiteViaAPI creates a site through the admin API and returns its ID.
func createSiteViaAPI(t *testing.T, slug, name string) string {
	t.Helper()

	body := fmt.Sprintf(`{"name":%q,"slug":%q}`, name, slug)
	req, err := http.NewRequest(http.MethodPost, cliE2EHarnessBaseURL+"/api/admin/sites", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building create-site request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cliE2EHarnessBearerToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("creating site via API: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create site: expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("parsing create-site response: %v", err)
	}
	if result.ID == "" {
		t.Fatal("create site: response missing id")
	}
	return result.ID
}

// deleteSiteCleanup registers a t.Cleanup that deletes the site via admin API.
func deleteSiteCleanup(t *testing.T, siteID string) {
	t.Helper()
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, cliE2EHarnessBaseURL+"/api/admin/sites/"+siteID, nil)
		if err != nil {
			t.Logf("deleteSiteCleanup: building request: %v", err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+cliE2EHarnessBearerToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Logf("deleteSiteCleanup: request failed: %v", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Logf("deleteSiteCleanup: unexpected status %d for site %s", resp.StatusCode, siteID)
		}
	})
}

// deployResponse is the subset of deploy fields we assert against.
type deployResponse struct {
	ID        string `json:"id"`
	SiteID    string `json:"siteId"`
	Status    string `json:"status"`
	FileCount int    `json:"fileCount"`
}

// getDeployViaAPI fetches a deploy by site and deploy ID through the admin API.
func getDeployViaAPI(t *testing.T, siteID, deployID string) deployResponse {
	t.Helper()

	url := fmt.Sprintf("%s/api/admin/sites/%s/deploys/%s", cliE2EHarnessBaseURL, siteID, deployID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("building get-deploy request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cliE2EHarnessBearerToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getting deploy via API: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get deploy: expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var deploy deployResponse
	if err := json.Unmarshal(respBody, &deploy); err != nil {
		t.Fatalf("parsing get-deploy response: %v", err)
	}
	return deploy
}

// buildDeployDir creates a temp directory containing exactly the files in the
// map (relative path → content). Callers include "index.html" explicitly for
// happy-path tests; error-path tests omit it intentionally.
func buildDeployDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for relPath, content := range files {
		fullPath := filepath.Join(dir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("buildDeployDir: mkdir for %q: %v", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("buildDeployDir: writing %q: %v", relPath, err)
		}
	}
	return dir
}

// deployIDPattern matches the UUID in CLI output like:
//
//	Deployed site "myslug" with deploy 550e8400-e29b-41d4-a716-446655440000 (2 files)
var deployIDPattern = regexp.MustCompile(`with deploy ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// extractDeployIDFromOutput parses the deploy UUID from CLI stdout.
func extractDeployIDFromOutput(t *testing.T, stdout string) string {
	t.Helper()
	matches := deployIDPattern.FindStringSubmatch(stdout)
	if len(matches) < 2 {
		t.Fatalf("could not extract deploy ID from stdout: %s", stdout)
	}
	return matches[1]
}

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

// TestCLI_E2E_SitesDeploy_HappyPath creates a site via API, deploys a
// directory with index.html + style.css via the CLI binary, and verifies
// the deploy was promoted to "live" with the correct file count.
func TestCLI_E2E_SitesDeploy_HappyPath(t *testing.T) {
	slug := uniqueSiteSlug(t)
	siteID := createSiteViaAPI(t, slug, "Happy Path Site")
	deleteSiteCleanup(t, siteID)

	dir := buildDeployDir(t, map[string]string{
		"index.html": "<html><body>hello</body></html>",
		"style.css":  "body { color: red; }",
	})

	stdout, stderr, exitCode := runCLIE2E(t, "sites", "deploy", slug, "--dir", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Deployed site") {
		t.Fatalf("expected stdout to contain 'Deployed site', got: %s", stdout)
	}

	deployID := extractDeployIDFromOutput(t, stdout)
	deploy := getDeployViaAPI(t, siteID, deployID)
	if deploy.Status != "live" {
		t.Errorf("expected deploy status 'live', got %q", deploy.Status)
	}
	if deploy.FileCount != 2 {
		t.Errorf("expected fileCount 2, got %d", deploy.FileCount)
	}
}

// TestCLI_E2E_SitesDeploy_ByID proves lookupAdminSiteByReference resolves
// a site by UUID, not just by slug.
func TestCLI_E2E_SitesDeploy_ByID(t *testing.T) {
	slug := uniqueSiteSlug(t)
	siteID := createSiteViaAPI(t, slug, "By ID Site")
	deleteSiteCleanup(t, siteID)

	dir := buildDeployDir(t, map[string]string{
		"index.html": "<html><body>by-id</body></html>",
	})

	// Pass site UUID instead of slug.
	stdout, stderr, exitCode := runCLIE2E(t, "sites", "deploy", siteID, "--dir", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Deployed site") {
		t.Fatalf("expected stdout to contain 'Deployed site', got: %s", stdout)
	}

	deployID := extractDeployIDFromOutput(t, stdout)
	deploy := getDeployViaAPI(t, siteID, deployID)
	if deploy.Status != "live" {
		t.Errorf("expected deploy status 'live', got %q", deploy.Status)
	}
	if deploy.FileCount != 1 {
		t.Errorf("expected fileCount 1, got %d", deploy.FileCount)
	}
}

// ---------------------------------------------------------------------------
// Error-path tests
// ---------------------------------------------------------------------------

// TestCLI_E2E_SitesDeploy_MissingIndexHTML verifies the CLI rejects a deploy
// directory that does not contain index.html.
func TestCLI_E2E_SitesDeploy_MissingIndexHTML(t *testing.T) {
	slug := uniqueSiteSlug(t)
	siteID := createSiteViaAPI(t, slug, "Missing Index Site")
	deleteSiteCleanup(t, siteID)

	dir := buildDeployDir(t, map[string]string{
		"style.css": "body {}",
	})

	stdout, stderr, exitCode := runCLIE2E(t, "sites", "deploy", slug, "--dir", dir)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "must include index.html") {
		t.Fatalf("expected stderr to contain 'must include index.html', got: %s", stderr)
	}
}

// TestCLI_E2E_SitesDeploy_EmptyDir verifies the CLI rejects an empty deploy directory.
func TestCLI_E2E_SitesDeploy_EmptyDir(t *testing.T) {
	slug := uniqueSiteSlug(t)
	siteID := createSiteViaAPI(t, slug, "Empty Dir Site")
	deleteSiteCleanup(t, siteID)

	dir := buildDeployDir(t, map[string]string{})

	stdout, stderr, exitCode := runCLIE2E(t, "sites", "deploy", slug, "--dir", dir)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "contains no files") {
		t.Fatalf("expected stderr to contain 'contains no files', got: %s", stderr)
	}
}

// TestCLI_E2E_SitesDeploy_NonexistentSite verifies the CLI fails when
// deploying to a site slug that does not exist.
func TestCLI_E2E_SitesDeploy_NonexistentSite(t *testing.T) {
	dir := buildDeployDir(t, map[string]string{
		"index.html": "<html></html>",
	})

	stdout, stderr, exitCode := runCLIE2E(t, "sites", "deploy", "nonexistent-slug-s5", "--dir", dir)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit; stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "not found") {
		t.Fatalf("expected stderr to contain 'not found', got: %s", stderr)
	}
}
