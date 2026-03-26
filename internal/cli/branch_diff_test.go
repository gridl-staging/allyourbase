package cli

import (
	"bytes"
	"testing"

	"github.com/allyourbase/ayb/internal/schemadiff"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func newBranchDiffOutputTestCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("output", "table", "")

	for k, v := range flags {
		testutil.NoError(t, cmd.Flags().Set(k, v))
	}

	return cmd
}

func TestValidateBranchDiffOutputFormat_valid(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"table", "json", "sql", ""} {
		normalized, err := validateBranchDiffOutputFormat(format)
		testutil.NoError(t, err)
		if format == "" {
			testutil.Equal(t, "table", normalized)
		} else {
			testutil.Equal(t, format, normalized)
		}
	}
}

func TestValidateBranchDiffOutputFormat_invalid(t *testing.T) {
	t.Parallel()
	_, err := validateBranchDiffOutputFormat("yaml")
	testutil.ErrorContains(t, err, "invalid output format")
}

func TestRunBranchDiff_rejectsInvalidOutputMode(t *testing.T) {
	t.Parallel()
	cmd := newBranchDiffOutputTestCmd(t, map[string]string{"output": "yaml"})

	err := runBranchDiff(cmd, []string{"branch-a", "branch-b"})
	testutil.ErrorContains(t, err, "invalid output format")
}

func TestPrintSQLChanges_noChanges(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := printSQLChanges(&buf, schemadiff.ChangeSet{})
	testutil.NoError(t, err)
	testutil.Equal(t, "-- No schema differences found.\n", buf.String())
}

func TestPrintTableChanges_noChanges(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := printTableChanges(&buf, schemadiff.ChangeSet{}, "a", "b")
	testutil.NoError(t, err)
	testutil.Equal(t, "No schema differences between \"a\" and \"b\".\n", buf.String())
}
