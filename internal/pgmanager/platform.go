package pgmanager

import (
	"fmt"
	"runtime"
	"strings"
)

const defaultReleaseRepo = "gridlhq/allyourbase"

// supportedPlatforms lists the GOOS/GOARCH pairs we build PG binaries for.
var supportedPlatforms = map[string]bool{
	"darwin-arm64": true,
	"darwin-amd64": true,
	"linux-amd64":  true,
	"linux-arm64":  true,
}

// platformKey returns "{os}-{arch}" for the current platform.
func platformKey() (string, error) {
	key := runtime.GOOS + "-" + runtime.GOARCH
	if !supportedPlatforms[key] {
		return "", fmt.Errorf("unsupported platform: %s", key)
	}
	return key, nil
}

// downloadURL constructs the full binary tarball URL.
// When baseURL is empty, the default GitHub release URL is used.
// When baseURL is non-empty, {version} and {platform} placeholders are substituted.
func downloadURL(baseURL, version, platform string) string {
	if baseURL == "" {
		return fmt.Sprintf(
			"https://github.com/%s/releases/download/pg-%s/ayb-postgres-%s-%s.tar.xz",
			defaultReleaseRepo, version, version, platform,
		)
	}
	r := strings.NewReplacer("{version}", version, "{platform}", platform)
	return r.Replace(baseURL)
}

// sha256SumsURL returns the URL to the SHA256SUMS file for a given binary URL template and version.
// For the default (empty baseURL), it uses the GitHub release directory.
// For a custom baseURL, it substitutes {version} and replaces the last path segment with SHA256SUMS.
func sha256SumsURL(baseURL, version string) string {
	if baseURL == "" {
		return fmt.Sprintf(
			"https://github.com/%s/releases/download/pg-%s/SHA256SUMS",
			defaultReleaseRepo, version,
		)
	}
	// Substitute {version} only (not {platform}) to get the release directory path.
	versioned := strings.ReplaceAll(baseURL, "{version}", version)
	if i := strings.LastIndex(versioned, "/"); i >= 0 {
		return versioned[:i+1] + "SHA256SUMS"
	}
	return versioned + "/SHA256SUMS"
}
