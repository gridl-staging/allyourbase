package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNotificationsMigrationSQLLoadable(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/121_ayb_notifications.sql")
	testutil.NoError(t, err)
	body := string(b)

	testutil.True(t, strings.Contains(body, "CREATE TABLE IF NOT EXISTS _ayb_notifications"), "121 must create notifications table")
	testutil.True(t, strings.Contains(body, "ENABLE ROW LEVEL SECURITY"), "121 must enable RLS")
	testutil.True(t, strings.Contains(body, "CREATE POLICY notif_user_owns ON _ayb_notifications"), "121 must define user ownership policy")
}
