package migrations

import (
	"io/fs"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// TestEmailTemplatesMigrationFileExists verifies the migration file is embedded.
// Constraint and schema behavior is validated by the integration test
// (TestEmailTemplatesMigrationConstraintsAndUniqueness).
func TestEmailTemplatesMigrationFileExists(t *testing.T) {
	t.Parallel()

	_, err := fs.ReadFile(embeddedMigrations, "sql/026_ayb_email_templates.sql")
	testutil.NoError(t, err)
}
