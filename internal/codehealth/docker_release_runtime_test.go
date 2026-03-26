package codehealth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedDockerImageBindsToAllInterfaces(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, "Dockerfile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}

	content := string(data)
	requireContainsAll(t, content, []string{
		`ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]`,
		`ENV AYB_SERVER_HOST=0.0.0.0`,
		`CMD ["ayb", "start", "--foreground"]`,
	})
}

func TestReleaseEvidenceArtifactsStayIgnored(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}

	requireContainsAll(t, string(data), []string{
		"_dev/release/evidence/*",
	})
}
