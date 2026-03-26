package cli

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBranchCmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place, racing with other callers.
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "branch" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'branch' subcommand registered on rootCmd")
}

func TestBranchCreateCmd_isSubcommand(t *testing.T) {
	found := false
	for _, cmd := range branchCmd.Commands() {
		if cmd.Use == "create <name>" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'create <name>' subcommand registered on branchCmd")
}

func TestBranchCreateCmd_hasFromFlag(t *testing.T) {
	t.Parallel()
	f := branchCreateCmd.Flags().Lookup("from")
	testutil.NotNil(t, f)
}

func TestBranchCreateCmd_hasDatabaseURLFlag(t *testing.T) {
	t.Parallel()
	f := branchCreateCmd.Flags().Lookup("database-url")
	testutil.NotNil(t, f)
}

func TestBranchCreateCmd_requiresArgs(t *testing.T) {
	t.Parallel()
	err := branchCreateCmd.Args(branchCreateCmd, []string{})
	testutil.Error(t, err)
}

func TestBranchListCmd_isSubcommand(t *testing.T) {
	found := false
	for _, cmd := range branchCmd.Commands() {
		if cmd.Use == "list" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'list' subcommand registered on branchCmd")
}

func TestBranchDeleteCmd_isSubcommand(t *testing.T) {
	found := false
	for _, cmd := range branchCmd.Commands() {
		if cmd.Use == "delete <name>" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'delete <name>' subcommand registered on branchCmd")
}

func TestBranchDeleteCmd_hasYesFlag(t *testing.T) {
	t.Parallel()
	f := branchDeleteCmd.Flags().Lookup("yes")
	testutil.NotNil(t, f)
}

func TestBranchDeleteCmd_hasForceFlag(t *testing.T) {
	t.Parallel()
	f := branchDeleteCmd.Flags().Lookup("force")
	testutil.NotNil(t, f)
}

func TestBranchDeleteCmd_requiresArgs(t *testing.T) {
	t.Parallel()
	err := branchDeleteCmd.Args(branchDeleteCmd, []string{})
	testutil.Error(t, err)
}

func TestBranchDiffCmd_isSubcommand(t *testing.T) {
	found := false
	for _, cmd := range branchCmd.Commands() {
		if cmd.Use == "diff <branchA> <branchB>" {
			found = true
			break
		}
	}
	testutil.True(t, found, "expected 'diff <branchA> <branchB>' subcommand registered on branchCmd")
}

func TestBranchDiffCmd_requiresTwoArgs(t *testing.T) {
	t.Parallel()
	err := branchDiffCmd.Args(branchDiffCmd, []string{"only-one"})
	testutil.Error(t, err)
}

// These tests use t.Setenv so they must NOT use t.Parallel().

func TestRunBranchCreate_missingDatabaseURL(t *testing.T) {
	cmd := *branchCreateCmd
	cmd.SetArgs([]string{"test-branch"})
	t.Setenv("AYB_DATABASE_URL", "")
	err := cmd.RunE(&cmd, []string{"test-branch"})
	testutil.Error(t, err)
}

func TestRunBranchCreate_invalidName(t *testing.T) {
	cmd := *branchCreateCmd
	cmd.SetArgs([]string{"INVALID"})
	t.Setenv("AYB_DATABASE_URL", "postgres://localhost/test")
	err := cmd.RunE(&cmd, []string{"INVALID"})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "invalid branch name")
}

func TestStartCmd_hasBranchFlag(t *testing.T) {
	t.Parallel()
	f := startCmd.Flags().Lookup("branch")
	testutil.NotNil(t, f)
}

func TestRunBranchDelete_requiresConfirmation(t *testing.T) {
	t.Parallel()
	cmd := *branchDeleteCmd
	err := cmd.RunE(&cmd, []string{"some-branch"})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "confirmation required")
}
