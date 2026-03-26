package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestStorageQuotasMigrationsSQL(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql047 := read(t, "047_ayb_storage_quotas.sql")

	testutil.True(t, strings.Contains(sql047, "CREATE TABLE IF NOT EXISTS _ayb_storage_usage"),
		"047 must create _ayb_storage_usage table")
	testutil.True(t, strings.Contains(sql047, "user_id    UUID PRIMARY KEY REFERENCES _ayb_users(id) ON DELETE CASCADE"),
		"047 must define user_id as PK with FK to _ayb_users")
	testutil.True(t, strings.Contains(sql047, "bytes_used BIGINT NOT NULL DEFAULT 0 CHECK (bytes_used >= 0)"),
		"047 must define bytes_used as non-negative bigint")
	testutil.True(t, strings.Contains(sql047, "updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()"),
		"047 must define updated_at timestamp")
	testutil.True(t, strings.Contains(sql047, "ALTER TABLE _ayb_users ADD COLUMN IF NOT EXISTS storage_quota_mb INT"),
		"047 must add storage_quota_mb column to _ayb_users")
}
