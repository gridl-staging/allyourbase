package storage

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRewritePublicURL(t *testing.T) {
	t.Parallel()
	origin := "https://api.example.com/api/storage/images/cat.jpg?download=1"
	cdn := "https://cdn.example.com"
	got := RewritePublicURL(origin, cdn)
	testutil.Equal(t, "https://cdn.example.com/api/storage/images/cat.jpg?download=1", got)
}

func TestRewritePublicURLEmptyCDN(t *testing.T) {
	t.Parallel()
	origin := "https://api.example.com/api/storage/images/cat.jpg?download=1"
	got := RewritePublicURL(origin, "")
	testutil.Equal(t, origin, got)
}

func TestRewritePublicURLStripsCDNUserInfo(t *testing.T) {
	t.Parallel()
	origin := "https://api.example.com/api/storage/images/cat.jpg?download=1"
	cdn := "https://user:secret@cdn.example.com"
	got := RewritePublicURL(origin, cdn)
	testutil.Equal(t, "https://cdn.example.com/api/storage/images/cat.jpg?download=1", got)
}
