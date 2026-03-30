package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Clean(filepath.Join("..", "..", rel))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func requireContainsAll(t *testing.T, file string, needles ...string) {
	t.Helper()
	body := readRepoFile(t, file)
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			t.Fatalf("%s missing expected content: %q", file, needle)
		}
	}
}

func TestStage1AppScopingGuidesDocumented(t *testing.T) {
	requireContainsAll(t, "docs-site/guide/api-reference.md",
		"## Admin: Apps",
		"GET    /api/admin/apps",
		"POST   /api/admin/apps",
		"## Admin: API keys",
		"POST   /api/admin/api-keys",
		"\"appId\"",
		"429 Too Many Requests",
	)

	requireContainsAll(t, "docs-site/guide/admin-dashboard.md",
		"### Apps management",
		"### API key app scoping",
		"### Per-app rate limits",
	)

	requireContainsAll(t, "docs-site/guide/configuration.md",
		"## Per-app API key scoping",
		"No server configuration is required",
	)
}

func TestStage1AppScopingFeatureTrackersUpdated(t *testing.T) {
	requireContainsAll(t, "_dev/FEATURES.md",
		"| App registry (`/admin/apps`) | ✅ | ✅ | ✅ |",
		"| API key management (`/admin/api-keys`) | ✅ | ✅ | ✅ |",
	)
}
