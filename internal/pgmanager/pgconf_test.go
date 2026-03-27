package pgmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWritePostgresConf(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	runtimeDir := t.TempDir()

	err := writePostgresConf(dataDir, 25432, runtimeDir, []string{"pg_stat_statements", "pg_cron"})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	testutil.NoError(t, err)

	s := string(content)
	testutil.Contains(t, s, "listen_addresses = '127.0.0.1'")
	testutil.Contains(t, s, "port = 25432")
	testutil.Contains(t, s, "shared_preload_libraries = 'pg_stat_statements,pg_cron'")
	testutil.Contains(t, s, "cron.database_name = 'ayb'")
	testutil.Contains(t, s, "logging_collector = off")
	testutil.Contains(t, s, "unix_socket_directories = '"+runtimeDir+"'")
}

func TestWritePostgresConfDefaultPort(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	err := writePostgresConf(dataDir, 15432, t.TempDir(), []string{"pg_stat_statements"})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	testutil.NoError(t, err)

	testutil.Contains(t, string(content), "port = 15432")
}

func TestWritePostgresConfEmptySharedPreload(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	err := writePostgresConf(dataDir, 15432, t.TempDir(), nil)
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	testutil.NoError(t, err)

	// Should not contain shared_preload_libraries when list is empty.
	testutil.True(t, !strings.Contains(string(content), "shared_preload_libraries"),
		"empty list should omit shared_preload_libraries directive")
	testutil.True(t, !strings.Contains(string(content), "cron.database_name"),
		"empty preload list should omit cron.database_name")
}

func TestWritePostgresConfCommaJoins(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	err := writePostgresConf(dataDir, 5432, t.TempDir(), []string{"a", "b", "c"})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	testutil.NoError(t, err)

	testutil.Contains(t, string(content), "shared_preload_libraries = 'a,b,c'")
}

func TestWritePostgresConfOverwrites(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	runtimeDir := t.TempDir()

	// Write first version.
	err := writePostgresConf(dataDir, 5432, runtimeDir, []string{"old_lib"})
	testutil.NoError(t, err)

	// Overwrite with new version.
	err = writePostgresConf(dataDir, 9999, runtimeDir, []string{"new_lib"})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	testutil.NoError(t, err)

	s := string(content)
	testutil.Contains(t, s, "port = 9999")
	testutil.Contains(t, s, "shared_preload_libraries = 'new_lib'")
	testutil.True(t, !strings.Contains(s, "old_lib"), "old content should be overwritten")
}

func TestWritePostgresConfOmitsCronDatabaseNameWithoutPgCron(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	err := writePostgresConf(dataDir, 5432, t.TempDir(), []string{"pg_stat_statements"})
	testutil.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dataDir, "postgresql.conf"))
	testutil.NoError(t, err)

	testutil.True(t, !strings.Contains(string(content), "cron.database_name"),
		"cron.database_name should only be written when pg_cron is preloaded")
}
