package codehealth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
)

const gridlNodePackageDir = "deploy/gridl-node"

func TestGridlNodePackageFilesExist(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	requireRepoFilesExist(t,
		"deploy/gridl-node/ayb.toml",
		"deploy/gridl-node/docker-compose.yml",
		"deploy/gridl-node/Dockerfile",
		"deploy/gridl-node/Caddyfile",
	)
}

func TestGridlNodeAybTomlLoadsSharedNodeDefaults(t *testing.T) {
	skipIfGridlNodePackageAbsent(t)

	repoRoot := findRepoRoot(t)
	configPath := filepath.Join(repoRoot, "deploy", "gridl-node", "ayb.toml")
	t.Setenv("AYB_AUTH_JWT_SECRET", "this-secret-is-at-least-thirty-two-characters-long")

	cfg, err := config.Load(configPath, nil)
	if err != nil {
		t.Fatalf("config.Load(%q) returned error: %v", configPath, err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("expected server.host 0.0.0.0, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 8090 {
		t.Fatalf("expected server.port 8090, got %d", cfg.Server.Port)
	}
	if cfg.Database.MaxConns != 100 {
		t.Fatalf("expected database.max_conns 100, got %d", cfg.Database.MaxConns)
	}
	if cfg.Database.MinConns != 5 {
		t.Fatalf("expected database.min_conns 5, got %d", cfg.Database.MinConns)
	}
	if !cfg.Storage.Enabled {
		t.Fatal("expected storage.enabled to be true")
	}
	if cfg.Storage.LocalPath != "/data/storage" {
		t.Fatalf("expected storage.local_path /data/storage, got %q", cfg.Storage.LocalPath)
	}
	if !cfg.Admin.Enabled {
		t.Fatal("expected admin.enabled to be true")
	}
	if !cfg.Auth.Enabled {
		t.Fatal("expected auth.enabled to be true")
	}
}

func TestGridlNodeComposeUsesSupportedRuntimeContract(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	composeContent := readGridlNodeFile(t, "docker-compose.yml")

	requireContainsAll(t, composeContent, []string{
		"dockerfile: deploy/gridl-node/Dockerfile",
		"AYB_DATABASE_URL",
		"AYB_ADMIN_PASSWORD: ${AYB_ADMIN_PASSWORD}",
		"AYB_AUTH_JWT_SECRET: ${AYB_AUTH_JWT_SECRET}",
		"AYB_CORS_ORIGINS",
		"AYB_SERVER_SITE_URL",
		"AYB_LOG_LEVEL",
		"./ayb.toml:/app/deploy/gridl-node/ayb.toml:ro",
		"127.0.0.1:8090:8090",
	})
	requireDoesNotContainAny(t, composeContent, []string{
		"AYB_ADMIN_ENABLED",
		"${ADMIN_PASSWORD}",
		"${JWT_SECRET}",
		"AYB_SERVER_CORS_ALLOWED_ORIGINS",
		"AYB_LOGGING_LEVEL",
	})

	flattened := strings.ReplaceAll(composeContent, "\n", " ")
	requireContainsAll(t, flattened, []string{"start", "--config", "/app/deploy/gridl-node/ayb.toml", "--foreground"})
}

func TestGridlNodeDockerfileUsesStartConfigEntrypoint(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	dockerfileContent := readGridlNodeFile(t, "Dockerfile")

	requireContainsAll(t, dockerfileContent, []string{
		"FROM golang:1.25-alpine AS builder",
		"FROM alpine:3.20",
		"addgroup -S ayb && adduser -S -G ayb -h /app ayb",
		"USER ayb",
		"ENTRYPOINT [\"ayb\"]",
		"CMD [\"start\", \"--config\", \"/app/deploy/gridl-node/ayb.toml\", \"--foreground\"]",
	})
}

func TestGridlNodeCaddyfileReverseProxiesToHostAybListener(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	caddyfileContent := readGridlNodeFile(t, "Caddyfile")
	requireContainsAll(t, caddyfileContent, []string{"reverse_proxy localhost:8090"})
	requireDoesNotContainAny(t, caddyfileContent, []string{"reverse_proxy ayb:8090"})
}

func TestGridlNodePackageExcludesShareboroughPatterns(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	repoRoot := findRepoRoot(t)
	for _, relativePath := range []string{
		"deploy/gridl-node/deploy.sh",
		"deploy/gridl-node/user-data.sh",
		"deploy/gridl-node/run-e2e-on-ec2.sh",
		"deploy/gridl-node/restart-on-reboot.sh",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, relativePath)); err == nil {
			t.Fatalf("did not expect shareborough-pattern asset %s", relativePath)
		}
	}
}

func skipIfGridlNodePackageAbsent(t *testing.T) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	requiredPaths := []string{
		filepath.Join(repoRoot, "deploy", "gridl-node", "ayb.toml"),
		filepath.Join(repoRoot, "docs", "gridl-integration-contract.md"),
	}
	for _, path := range requiredPaths {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				t.Skip("gridl-node runtime package is intentionally excluded from public mirrors")
			}
			t.Fatalf("stat %s failed: %v", path, err)
		}
	}
}

func readGridlNodeFile(t *testing.T, fileName string) string {
	t.Helper()

	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, gridlNodePackageDir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(data)
}

func requireRepoFilesExist(t *testing.T, relativePaths ...string) {
	t.Helper()

	repoRoot := findRepoRoot(t)
	for _, relativePath := range relativePaths {
		if _, err := os.Stat(filepath.Join(repoRoot, relativePath)); err != nil {
			t.Fatalf("expected %s to exist: %v", relativePath, err)
		}
		requirePathTrackedByGit(t, repoRoot, relativePath)
	}
}

func requireContainsAll(t *testing.T, content string, expectedSubstrings []string) {
	t.Helper()

	for _, expectedSubstring := range expectedSubstrings {
		if !strings.Contains(content, expectedSubstring) {
			t.Fatalf("expected content to include %q", expectedSubstring)
		}
	}
}

func requireDoesNotContainAny(t *testing.T, content string, bannedSubstrings []string) {
	t.Helper()

	for _, bannedSubstring := range bannedSubstrings {
		if strings.Contains(content, bannedSubstring) {
			t.Fatalf("expected content not to include %q", bannedSubstring)
		}
	}
}

func requirePathTrackedByGit(t *testing.T, repoRoot string, relativePath string) {
	t.Helper()

	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "--error-unmatch", "--", relativePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected %s to be tracked by git: %v output=%s", relativePath, err, strings.TrimSpace(string(output)))
	}
}
