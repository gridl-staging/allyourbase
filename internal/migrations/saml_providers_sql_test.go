package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSAMLProvidersMigrationFile(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/041_ayb_saml_providers.sql")
	testutil.NoError(t, err)
	sql := strings.Join(strings.Fields(string(b)), " ")

	testutil.True(t, strings.Contains(sql, "CREATE TABLE IF NOT EXISTS _ayb_saml_providers"), "expected _ayb_saml_providers table")
	testutil.True(t, strings.Contains(sql, "id UUID PRIMARY KEY DEFAULT gen_random_uuid()"), "expected id UUID primary key with gen_random_uuid default")
	testutil.True(t, strings.Contains(sql, "name TEXT NOT NULL UNIQUE"), "expected name column with unique constraint")
	testutil.True(t, strings.Contains(sql, "entity_id TEXT NOT NULL"), "expected entity_id column")
	testutil.True(t, strings.Contains(sql, "idp_metadata TEXT NOT NULL"), "expected idp_metadata column")
	testutil.True(t, strings.Contains(sql, "sp_cert TEXT"), "expected sp_cert column")
	testutil.True(t, strings.Contains(sql, "sp_key_enc BYTEA"), "expected sp_key_enc encrypted key column")
	testutil.True(t, strings.Contains(sql, "attribute_mapping JSONB"), "expected attribute_mapping column")
	testutil.True(t, strings.Contains(sql, "created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()"), "expected created_at timestamp")
	testutil.True(t, strings.Contains(sql, "updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()"), "expected updated_at timestamp")
}
