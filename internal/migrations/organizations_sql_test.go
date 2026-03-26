package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOrganizationsMigrationUpdatesTenantMembershipRoleConstraint(t *testing.T) {
	t.Parallel()

	bodyBytes, err := fs.ReadFile(embeddedMigrations, "sql/164_ayb_organizations.sql")
	testutil.NoError(t, err)
	body := string(bodyBytes)

	testutil.True(
		t,
		strings.Contains(body, "AND conname = '_ayb_tenant_memberships_role_check'"),
		"164 must locate the existing tenant membership role check by constraint name",
	)
	testutil.True(
		t,
		!strings.Contains(body, "pg_get_constraintdef(oid) LIKE '%role IN (''owner'', ''admin'', ''member'', ''viewer'')%'"),
		"164 must not only match already-correct viewer constraint definitions when dropping the old check",
	)
	testutil.True(
		t,
		!strings.Contains(body, "CREATE INDEX IF NOT EXISTS idx_ayb_organizations_slug ON _ayb_organizations (slug);"),
		"164 must not create a redundant standalone slug index when the UNIQUE constraint already creates one",
	)
}
