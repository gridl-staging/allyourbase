package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPushMigrationSQLConstraints(t *testing.T) {
	t.Parallel()

	read := func(t *testing.T, name string) string {
		t.Helper()
		b, err := fs.ReadFile(embeddedMigrations, "sql/"+name)
		testutil.NoError(t, err)
		return string(b)
	}

	sql027 := read(t, "027_ayb_device_tokens.sql")
	testutil.True(t, strings.Contains(sql027, "_ayb_device_tokens"),
		"027 must create _ayb_device_tokens table")
	testutil.True(t, strings.Contains(sql027, "CHECK (provider IN ('fcm', 'apns'))"),
		"027 must enforce allowed provider values")
	testutil.True(t, strings.Contains(sql027, "CHECK (platform IN ('android', 'ios'))"),
		"027 must enforce allowed platform values")
	testutil.True(t, strings.Contains(sql027, "CHECK (length(token) > 0 AND length(token) <= 4096)"),
		"027 must enforce token length bounds")
	testutil.True(t, strings.Contains(sql027, "UNIQUE (app_id, provider, token)"),
		"027 must enforce app/provider/token uniqueness")
	testutil.True(t, strings.Contains(sql027, "UNIQUE (id, app_id, user_id, provider)"),
		"027 must support composite FK from deliveries to keep denormalized fields consistent")
	testutil.True(t, strings.Contains(sql027, "idx_ayb_device_tokens_user_active"),
		"027 must index user active token lookups")
	testutil.True(t, strings.Contains(sql027, "idx_ayb_device_tokens_app_user_active"),
		"027 must index app/user active token lookups")
	testutil.True(t, strings.Contains(sql027, "idx_ayb_device_tokens_active_refreshed"),
		"027 must index stale token cleanup by refreshed timestamp")

	sql028 := read(t, "028_ayb_push_deliveries.sql")
	testutil.True(t, strings.Contains(sql028, "_ayb_push_deliveries"),
		"028 must create _ayb_push_deliveries table")
	testutil.True(t, strings.Contains(sql028, "REFERENCES _ayb_jobs(id) ON DELETE SET NULL"),
		"028 must enforce job FK set-null delete behavior")
	testutil.True(t, strings.Contains(sql028, "FOREIGN KEY (device_token_id, app_id, user_id, provider)"),
		"028 must enforce delivery token/app/user/provider consistency")
	testutil.True(t, strings.Contains(sql028, "REFERENCES _ayb_device_tokens(id, app_id, user_id, provider) ON DELETE CASCADE"),
		"028 must enforce denormalized columns match the referenced device token")
	testutil.True(t, strings.Contains(sql028, "CHECK (provider IN ('fcm', 'apns'))"),
		"028 must enforce allowed provider values")
	testutil.True(t, strings.Contains(sql028, "CHECK (length(title) > 0 AND length(title) <= 256)"),
		"028 must enforce title length bounds")
	testutil.True(t, strings.Contains(sql028, "CHECK (length(body) > 0 AND length(body) <= 4096)"),
		"028 must enforce body length bounds")
	testutil.True(t, strings.Contains(sql028, "CHECK (data_payload IS NULL OR length(data_payload::text) <= 8192)"),
		"028 must enforce data payload size bounds")
	testutil.True(t, strings.Contains(sql028, "CHECK (status IN ('pending', 'sent', 'failed', 'invalid_token'))"),
		"028 must enforce allowed status values")
	testutil.True(t, strings.Contains(sql028, "idx_ayb_push_deliveries_app_user_created"),
		"028 must index app/user delivery history lookups")
	testutil.True(t, strings.Contains(sql028, "idx_ayb_push_deliveries_status"),
		"028 must index status filters")
}
