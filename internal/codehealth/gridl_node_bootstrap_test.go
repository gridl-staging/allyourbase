package codehealth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGridlNodeBootstrapFilesExist(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	requireRepoFilesExist(t,
		"deploy/gridl-node/host-prepare.sh",
		"deploy/gridl-node/start-gridl-node.sh",
		"deploy/gridl-node/ayb.service",
		"deploy/gridl-node/README.md",
		"deploy/gridl-node/operator.env.example",
	)
}

func TestGridlNodeBootstrapScriptsDoNotRegenerateRuntimeConfig(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	for _, scriptName := range []string{"host-prepare.sh", "start-gridl-node.sh"} {
		content := readGridlNodeFile(t, scriptName)

		// Scripts must not inline docker-compose service definitions or ayb.toml content.
		requireDoesNotContainAny(t, content, []string{
			"cat > docker-compose.yml",
			"cat > ayb.toml",
			"cat > Caddyfile",
			"SHAREBOROUGH_",
			".secret/.env.secret",
		})
	}
}

func TestGridlNodeStartScriptUsesOperatorEnvNames(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	content := readGridlNodeFile(t, "start-gridl-node.sh")

	// The start script must validate operator-facing env vars directly.
	requireContainsAll(t, content, []string{
		"AYB_ADMIN_PASSWORD",
		"AYB_AUTH_JWT_SECRET",
		"POSTGRES_PASSWORD",
		"docker compose",
		"chmod 600 \"$ENV_FILE\"",
	})
}

func TestGridlNodeStartScriptFailsOnMissingSecrets(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	content := readGridlNodeFile(t, "start-gridl-node.sh")

	// The start script must validate required secrets before starting.
	requireContainsAll(t, content, []string{
		"set -euo pipefail",
	})

	requireDoesNotContainAny(t, content, []string{
		". \"$ENV_FILE\"",
		"source \"$ENV_FILE\"",
		".env.compose",
	})

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "ADMIN_PASSWORD") && !strings.Contains(trimmed, "AYB_ADMIN_PASSWORD") {
			t.Fatalf("start script should not introduce a second admin password placeholder: %s", trimmed)
		}
		if strings.Contains(trimmed, "JWT_SECRET") && !strings.Contains(trimmed, "AYB_AUTH_JWT_SECRET") {
			t.Fatalf("start script should not introduce a second JWT placeholder: %s", trimmed)
		}
	}
}

func TestGridlNodeSystemdUnitManagesComposeStack(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	content := readGridlNodeFile(t, "ayb.service")

	requireContainsAll(t, content, []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"WantedBy=multi-user.target",
		"/opt/gridl-node/deploy/gridl-node/start-gridl-node.sh",
		"/opt/gridl-node/deploy/gridl-node/docker-compose.yml",
	})

	// Must not run the AYB binary directly — systemd manages docker-compose.
	requireDoesNotContainAny(t, content, []string{
		"ExecStart=/usr/local/bin/ayb",
		"ExecStart=ayb start",
	})
}

func TestGridlNodeReadmeReferencesContractDoc(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	content := readGridlNodeFile(t, "README.md")

	requireContainsAll(t, content, []string{
		"docs/gridl-integration-contract.md",
		"/health",
		"/api/admin/status",
		"/api/admin/auth",
		"AYB_ADMIN_PASSWORD",
		"AYB_AUTH_JWT_SECRET",
		"POSTGRES_PASSWORD",
		"/opt/gridl-node",
		"most other routes use a bearer token",
	})

	// README must keep /api/admin/... separate from /admin (SPA).
	requireContainsAll(t, content, []string{
		"/api/admin/",
	})
}

func TestGridlNodeOperatorEnvExampleDocumentsRequiredSecrets(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	content := readGridlNodeFile(t, "operator.env.example")

	requireContainsAll(t, content, []string{
		"AYB_ADMIN_PASSWORD",
		"AYB_AUTH_JWT_SECRET",
		"POSTGRES_PASSWORD",
	})

	// Must not use legacy shareborough naming or bare compose placeholders as operator inputs.
	requireDoesNotContainAny(t, content, []string{
		"SHAREBOROUGH_",
	})
}

func TestGridlNodeContractDocExists(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	requireRepoFilesExist(t, "docs/gridl-integration-contract.md")
}

func TestGridlNodeContractDocMatchesServerBehavior(t *testing.T) {
	t.Parallel()
	skipIfGridlNodePackageAbsent(t)

	repoRoot := findRepoRoot(t)
	contractPath := filepath.Join(repoRoot, "docs", "gridl-integration-contract.md")
	data, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read %s failed: %v", contractPath, err)
	}
	content := string(data)

	requireContainsAll(t, content, []string{
		"There is **no** `AYB_ADMIN_ENABLED` env var.",
		"admin auth remains unconfigured",
		"`POST /api/admin/auth` returns `404`",
		"requests return `404`",
	})
	requireDoesNotContainAny(t, content, []string{
		"generates a random password and prints it to stdout",
		"these endpoints return `503`",
	})
}
