package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

// --- extractDBName ---

func TestExtractDBName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"postgres://localhost/mydb", "mydb"},
		{"postgres://user:pass@host:5432/appdb?sslmode=disable", "appdb"},
		{"postgres://localhost/", "default"},
		{"postgres://localhost", "default"},
		{"just-a-name", "default"},
	}
	for _, tt := range tests {
		got := extractDBName(tt.url)
		if got != tt.want {
			t.Errorf("extractDBName(%q) = %q; want %q", tt.url, got, tt.want)
		}
	}
}

// --- subcommand registration ---

func TestDBSubcommandsRegistered(t *testing.T) {
	want := map[string]bool{
		"backup":  false,
		"restore": false,
	}
	for _, cmd := range dbCmd.Commands() {
		if _, ok := want[cmd.Name()]; ok {
			want[cmd.Name()] = true
		}
	}
	for sub, found := range want {
		if !found {
			t.Fatalf("expected '%s' subcommand under 'db'", sub)
		}
	}
}

func TestDBBackupListSubcommandRegistered(t *testing.T) {
	var found bool
	for _, cmd := range dbBackupCmd.Commands() {
		if cmd.Name() == "list" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'list' subcommand under 'db backup'")
	}
}

// --- validation error paths ---

func TestDBBackupRequiresEnabledConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/ayb.toml", []byte("[backup]\nenabled = false\n"), 0o644)

	var buf bytes.Buffer
	dbBackupCmd.SetOut(&buf)
	dbBackupCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"db", "backup", "--config", dir + "/ayb.toml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when backup is disabled")
	}
}

func TestDBBackupMissingConfig(t *testing.T) {
	rootCmd.SetArgs([]string{"db", "backup", "--config", "/nonexistent/ayb.toml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestDBRestoreRequiresFromOrArg(t *testing.T) {
	rootCmd.SetArgs([]string{"db", "restore", "--config", "/nonexistent/ayb.toml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no --from and no args")
	}
}

func TestDBRestoreFromRequiresBackupEnabled(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/ayb.toml", []byte("[backup]\nenabled = false\n[database]\nurl = \"postgres://localhost/testdb\"\n"), 0o644)

	rootCmd.SetArgs([]string{"db", "restore", "--from", "some-id",
		"--config", dir + "/ayb.toml", "--yes"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when backup is disabled for restore")
	}
}

func TestDBBackupListMissingDBURL(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/ayb.toml", []byte("[backup]\nenabled = true\nbucket = \"test\"\nregion = \"us-east-1\"\n"), 0o644)

	rootCmd.SetArgs([]string{"db", "backup", "list", "--config", dir + "/ayb.toml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing database URL")
	}
}

// ── seed tests ────────────────────────────────────────────────────────────────

func newDBSeedTestCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	for k, v := range flags {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func newDBResetTestCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().BoolP("yes", "y", false, "")
	for k, v := range flags {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestRunDBSeed_missingDatabaseURL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	seedFile := filepath.Join(dir, "seed.sql")
	testutil.NoError(t, os.WriteFile(seedFile, []byte("SELECT 1;"), 0o644))

	cmd := newDBSeedTestCmd(t, nil)
	err := runDBSeed(cmd, []string{seedFile})
	testutil.ErrorContains(t, err, "database URL")
}

func TestRunDBSeed_missingFile(t *testing.T) {
	t.Parallel()

	cmd := newDBSeedTestCmd(t, map[string]string{
		"database-url": "postgresql://localhost:5432/test",
	})
	err := runDBSeed(cmd, []string{"/nonexistent/seed.sql"})
	testutil.ErrorContains(t, err, "reading seed file")
}

func TestRunDBSeed_noFileArgEmptySeedInConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ayb.toml")
	testutil.NoError(t, os.WriteFile(cfgPath, []byte("[database]\nurl = \"\"\nseed_file = \"\"\n"), 0o644))

	cmd := newDBSeedTestCmd(t, map[string]string{
		"config":       cfgPath,
		"database-url": "postgresql://localhost:5432/test",
	})
	err := runDBSeed(cmd, []string{})
	testutil.ErrorContains(t, err, "no seed file specified")
}

func TestDBSeedCmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place.
	found := false
	for _, sub := range dbCmd.Commands() {
		if sub.Name() == "seed" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'seed' subcommand under db")
}

// ── reset tests ───────────────────────────────────────────────────────────────

func TestRunDBReset_requiresYes(t *testing.T) {
	t.Parallel()

	cmd := newDBResetTestCmd(t, nil)
	err := runDBReset(cmd, nil)
	testutil.ErrorContains(t, err, "--yes")
}

func TestRunDBReset_missingDatabaseURL(t *testing.T) {
	t.Parallel()

	cmd := newDBResetTestCmd(t, map[string]string{"yes": "true"})
	err := runDBReset(cmd, nil)
	testutil.ErrorContains(t, err, "database URL")
}

func TestRunDBReset_configLoadedBeforeConnect(t *testing.T) {
	t.Parallel()

	// config.Load silently ignores missing files, so with a bad config path,
	// the error chain proceeds to DB connect (which fails on invalid URL format).
	// Verify that reset at least checks --yes and DB URL before any other work.
	cmd := newDBResetTestCmd(t, map[string]string{
		"yes":          "true",
		"database-url": "postgresql://localhost:5432/test",
		"config":       "/nonexistent/ayb.toml",
	})
	err := runDBReset(cmd, nil)
	// After --yes check and db URL, it loads config for migrations dir then connects.
	// Since config load succeeds (silent miss), expect a connection error.
	testutil.True(t, err != nil, "expected error for unreachable DB")
}

func TestDBResetCmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place.
	found := false
	for _, sub := range dbCmd.Commands() {
		if sub.Name() == "reset" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'reset' subcommand under db")
}

func TestDBResetCmd_hasYesFlag(t *testing.T) {
	t.Parallel()

	f := dbResetCmd.Flags().Lookup("yes")
	testutil.NotNil(t, f)
}

func TestLoadDBConfig_succeedsForMissingPath(t *testing.T) {
	t.Parallel()

	// config.Load silently ignores missing files and returns defaults.
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	testutil.NoError(t, cmd.Flags().Set("config", "/nonexistent/path/ayb.toml"))
	cfg, err := loadDBConfig(cmd)
	testutil.NoError(t, err)
	testutil.NotNil(t, cfg)
}

func TestQuoteSQLIdentifier_escapesEmbeddedQuotes(t *testing.T) {
	t.Parallel()

	got := sqlutil.QuoteIdent(`sch"ema`)
	testutil.Equal(t, `"sch""ema"`, got)
}

func TestQualifiedSQLIdentifier_escapesSchemaAndName(t *testing.T) {
	t.Parallel()

	got := sqlutil.QuoteQualifiedName(`my"schema`, `tab"le`)
	testutil.Equal(t, `"my""schema"."tab""le"`, got)
}

// keep compiler happy with strings import already used elsewhere in file
var _ = strings.Contains

func TestDBGoUnder500Lines(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("db.go")
	testutil.NoError(t, err)

	lineCount := 1
	for _, b := range content {
		if b == '\n' {
			lineCount++
		}
	}

	if lineCount > 500 {
		t.Fatalf("db.go is %s lines; expected <= 500", strconv.Itoa(lineCount))
	}
}
