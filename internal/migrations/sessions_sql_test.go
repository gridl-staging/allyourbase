package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSessionMetadataMigrationFile(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/042_ayb_sessions_metadata.sql")
	testutil.NoError(t, err)
	sql := string(b)

	testutil.True(t, strings.Contains(sql, "ALTER TABLE _ayb_sessions"), "expected ALTER TABLE for _ayb_sessions")
	testutil.True(t, strings.Contains(sql, "ADD COLUMN IF NOT EXISTS user_agent TEXT"), "expected user_agent column")
	testutil.True(t, strings.Contains(sql, "ADD COLUMN IF NOT EXISTS ip_address TEXT"), "expected ip_address column")
	testutil.True(t, strings.Contains(sql, "ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ DEFAULT NOW()"), "expected last_active_at column")
}
