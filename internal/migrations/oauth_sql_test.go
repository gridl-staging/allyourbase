package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthMigrationSQLConstraints(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql019 := read(t, "019_ayb_oauth_clients.sql")
	testutil.True(t, strings.Contains(sql019, "CHECK (client_id ~ '^ayb_cid_[0-9a-f]{48}$')"),
		"019 must enforce ayb_cid_ + 24-byte hex client_id format")
	testutil.True(t, strings.Contains(sql019, "CHECK (client_type IN ('confidential', 'public'))"),
		"019 must enforce allowed client_type values")
	testutil.True(t, strings.Contains(sql019, "CHECK ((client_type = 'confidential' AND client_secret_hash IS NOT NULL) OR (client_type = 'public' AND client_secret_hash IS NULL))"),
		"019 must enforce client_secret_hash presence by client_type")
	testutil.True(t, strings.Contains(sql019, "CHECK (scopes <@ ARRAY['readonly', 'readwrite', '*']::TEXT[])"),
		"019 must enforce allowed scope values")
	testutil.True(t, strings.Contains(sql019, "CHECK (cardinality(scopes) >= 1)"),
		"019 must enforce at least one scope")

	sql020 := read(t, "020_ayb_oauth_authorization_codes.sql")
	testutil.True(t, strings.Contains(sql020, "CHECK (scope IN ('readonly', 'readwrite', '*'))"),
		"020 must enforce allowed scope values")
	testutil.True(t, strings.Contains(sql020, "CHECK (code_challenge_method = 'S256')"),
		"020 must enforce S256-only PKCE method")

	sql021 := read(t, "021_ayb_oauth_tokens.sql")
	testutil.True(t, strings.Contains(sql021, "CHECK (token_type IN ('access', 'refresh'))"),
		"021 must enforce access/refresh token types")
	testutil.True(t, strings.Contains(sql021, "CHECK (scope IN ('readonly', 'readwrite', '*'))"),
		"021 must enforce allowed scope values")

	sql022 := read(t, "022_ayb_oauth_consents.sql")
	testutil.True(t, strings.Contains(sql022, "CHECK (scope IN ('readonly', 'readwrite', '*'))"),
		"022 must enforce allowed scope values")
}
