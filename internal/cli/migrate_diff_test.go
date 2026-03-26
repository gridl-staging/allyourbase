package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/schemadiff"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

// helpers shared across migrate diff/generate tests

func newMigrateDiffCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("migrations-dir", "", "")
	cmd.Flags().String("output", "table", "")
	cmd.Flags().Bool("sql", false, "")
	for k, v := range flags {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func newMigrateGenerateCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().String("migrations-dir", "", "")
	for k, v := range flags {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestRunMigrateDiff_missingConfig(t *testing.T) {
	t.Parallel()

	// config.Load silently ignores missing files (no error on not-found).
	// With no db URL, the error will be "database URL".
	cmd := newMigrateDiffCmd(t, map[string]string{
		"config": "/nonexistent/ayb.toml",
	})
	err := runMigrateDiff(cmd, nil)
	testutil.ErrorContains(t, err, "database URL")
}

func TestRunMigrateDiff_missingDatabaseURL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ayb.toml")
	_ = os.WriteFile(cfgPath, []byte("[server]\nport = 8080\n"), 0o644)

	cmd := newMigrateDiffCmd(t, map[string]string{
		"config":         cfgPath,
		"migrations-dir": dir,
	})
	err := runMigrateDiff(cmd, nil)
	testutil.ErrorContains(t, err, "database URL")
}

func TestRunMigrateDiff_outputSQLFlag_requiresDB(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ayb.toml")
	_ = os.WriteFile(cfgPath, []byte("[server]\nport = 8080\n"), 0o644)

	var buf bytes.Buffer
	cmd := newMigrateDiffCmd(t, map[string]string{
		"config":         cfgPath,
		"migrations-dir": dir,
		"sql":            "true",
	})
	cmd.SetOut(&buf)

	err := runMigrateDiff(cmd, nil)
	testutil.ErrorContains(t, err, "database URL")
}

func TestRunMigrateGenerate_missingConfig(t *testing.T) {
	t.Parallel()

	// config.Load silently ignores missing files; error manifests as missing db URL.
	cmd := newMigrateGenerateCmd(t, map[string]string{
		"config": "/nonexistent/ayb.toml",
	})
	err := runMigrateGenerate(cmd, []string{"add_users"})
	testutil.ErrorContains(t, err, "database URL")
}

func TestRunMigrateGenerate_missingDatabaseURL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ayb.toml")
	_ = os.WriteFile(cfgPath, []byte("[server]\nport = 8080\n"), 0o644)

	cmd := newMigrateGenerateCmd(t, map[string]string{
		"config":         cfgPath,
		"migrations-dir": dir,
	})
	err := runMigrateGenerate(cmd, []string{"add_users"})
	testutil.ErrorContains(t, err, "database URL")
}

func TestLoadBaselineSnapshot_missingFileReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	snap, err := loadBaselineSnapshot(dir)
	testutil.NoError(t, err)
	testutil.NotNil(t, snap)
	testutil.Equal(t, 0, len(snap.Tables))
	testutil.Equal(t, 0, len(snap.Enums))
}

func TestMigrateDiffCmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place.
	found := false
	for _, sub := range migrateCmd.Commands() {
		if sub.Name() == "diff" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'diff' subcommand to be registered under migrate")
}

func TestMigrateGenerateCmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place.
	found := false
	for _, sub := range migrateCmd.Commands() {
		if sub.Name() == "generate" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'generate' subcommand to be registered under migrate")
}

func TestMigrateGenerateCmd_requiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	err := migrateGenerateCmd.Args(migrateGenerateCmd, []string{})
	if err == nil {
		t.Error("expected error for 0 args, got nil")
	}

	err2 := migrateGenerateCmd.Args(migrateGenerateCmd, []string{"a", "b"})
	if err2 == nil {
		t.Error("expected error for 2 args, got nil")
	}
}

// TestMigrateSnapshotBaseline_roundtrip verifies save/load of baseline in a temp dir.
func TestMigrateSnapshotBaseline_roundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	snap := &schemadiff.Snapshot{
		Extensions: []schemadiff.SnapExtension{{Name: "pgvector", Version: "0.7.0"}},
	}

	baselinePath := filepath.Join(dir, snapshotBaseline)
	err := schemadiff.SaveSnapshot(baselinePath, snap)
	testutil.NoError(t, err)

	loaded, err := loadBaselineSnapshot(dir)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(loaded.Extensions))
	testutil.Equal(t, "pgvector", loaded.Extensions[0].Name)
}

func TestChangeDetail_allTypes(t *testing.T) {
	t.Parallel()

	// Verify changeDetail does not panic for any change type.
	for _, ct := range []schemadiff.ChangeType{
		schemadiff.ChangeCreateTable,
		schemadiff.ChangeDropTable,
		schemadiff.ChangeAddColumn,
		schemadiff.ChangeDropColumn,
		schemadiff.ChangeAlterColumnType,
		schemadiff.ChangeAlterColumnDefault,
		schemadiff.ChangeAlterColumnNullable,
		schemadiff.ChangeCreateIndex,
		schemadiff.ChangeDropIndex,
		schemadiff.ChangeAddForeignKey,
		schemadiff.ChangeDropForeignKey,
		schemadiff.ChangeAddCheckConstraint,
		schemadiff.ChangeDropCheckConstraint,
		schemadiff.ChangeCreateEnum,
		schemadiff.ChangeAlterEnumAddValue,
		schemadiff.ChangeAddRLSPolicy,
		schemadiff.ChangeDropRLSPolicy,
		schemadiff.ChangeEnableExtension,
		schemadiff.ChangeDisableExtension,
	} {
		c := schemadiff.Change{Type: ct}
		_ = changeDetail(c)
	}
}
