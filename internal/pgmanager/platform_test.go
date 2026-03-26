package pgmanager

import (
	"runtime"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPlatformKeyCurrentPlatform(t *testing.T) {
	t.Parallel()
	key, err := platformKey()
	testutil.NoError(t, err)
	expected := runtime.GOOS + "-" + runtime.GOARCH
	testutil.Equal(t, expected, key)
}

func TestPlatformKeyKnownPairs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos, goarch string
		want         string
	}{
		{"darwin", "arm64", "darwin-arm64"},
		{"darwin", "amd64", "darwin-amd64"},
		{"linux", "amd64", "linux-amd64"},
		{"linux", "arm64", "linux-arm64"},
	}
	for _, tt := range tests {
		testutil.Equal(t, tt.want, tt.goos+"-"+tt.goarch)
	}
}

func TestDownloadURLDefault(t *testing.T) {
	t.Parallel()
	got := downloadURL("", "16", "darwin-arm64")
	want := "https://github.com/gridlhq/allyourbase/releases/download/pg-16/ayb-postgres-16-darwin-arm64.tar.xz"
	testutil.Equal(t, want, got)
}

func TestDownloadURLDefaultLinux(t *testing.T) {
	t.Parallel()
	got := downloadURL("", "17", "linux-amd64")
	want := "https://github.com/gridlhq/allyourbase/releases/download/pg-17/ayb-postgres-17-linux-amd64.tar.xz"
	testutil.Equal(t, want, got)
}

func TestDownloadURLCustomBase(t *testing.T) {
	t.Parallel()
	base := "https://my-cdn.example.com/postgres/{version}/{platform}.tar.xz"
	got := downloadURL(base, "16", "linux-arm64")
	want := "https://my-cdn.example.com/postgres/16/linux-arm64.tar.xz"
	testutil.Equal(t, want, got)
}

func TestDownloadURLCustomBaseMultiplePlaceholders(t *testing.T) {
	t.Parallel()
	base := "https://cdn.example.com/pg-{version}-{platform}-{version}.tar.xz"
	got := downloadURL(base, "16", "darwin-arm64")
	want := "https://cdn.example.com/pg-16-darwin-arm64-16.tar.xz"
	testutil.Equal(t, want, got)
}

func TestSHA256SumsURLDefault(t *testing.T) {
	t.Parallel()
	got := sha256SumsURL("", "16")
	want := "https://github.com/gridlhq/allyourbase/releases/download/pg-16/SHA256SUMS"
	testutil.Equal(t, want, got)
}

func TestSHA256SumsURLCustomBase(t *testing.T) {
	t.Parallel()
	base := "https://cdn.example.com/postgres/{version}/{platform}.tar.xz"
	got := sha256SumsURL(base, "16")
	want := "https://cdn.example.com/postgres/16/SHA256SUMS"
	testutil.Equal(t, want, got)
}

func TestSHA256SumsURLCustomBaseNoVersion(t *testing.T) {
	t.Parallel()
	// URL without version in path — last segment replaced.
	base := "https://cdn.example.com/pg/{platform}.tar.xz"
	got := sha256SumsURL(base, "16")
	want := "https://cdn.example.com/pg/SHA256SUMS"
	testutil.Equal(t, want, got)
}
