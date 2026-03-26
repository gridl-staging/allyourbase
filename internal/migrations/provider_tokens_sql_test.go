package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthProviderTokensMigrationFile(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/040_ayb_oauth_provider_tokens.sql")
	testutil.NoError(t, err)
	sql := strings.Join(strings.Fields(string(b)), " ")

	testutil.True(t, strings.Contains(sql, "CREATE TABLE IF NOT EXISTS _ayb_oauth_provider_tokens"), "expected oauth provider tokens table")
	testutil.True(t, strings.Contains(sql, "user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE"), "expected user_id FK cascade")
	testutil.True(t, strings.Contains(sql, "provider TEXT NOT NULL"), "expected provider column")
	testutil.True(t, strings.Contains(sql, "access_token_enc BYTEA"), "expected encrypted access token column")
	testutil.True(t, strings.Contains(sql, "refresh_token_enc BYTEA"), "expected encrypted refresh token column")
	testutil.True(t, strings.Contains(sql, "refresh_failure_count INT NOT NULL DEFAULT 0"), "expected refresh failure count default")
	testutil.True(t, strings.Contains(sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_oauth_provider_tokens_user_provider"), "expected unique user+provider index")
}
