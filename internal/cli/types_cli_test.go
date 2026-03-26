package cli

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func TestTypesOpenAPICmd_isRegistered(t *testing.T) {
	// Not parallel: cobra.Command.Commands() sorts in-place.
	found := false
	for _, sub := range typesCmd.Commands() {
		if sub.Use == "openapi" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ayb types openapi subcommand is not registered")
	}
}

func TestTypesOpenAPICmd_hasOutputFlag(t *testing.T) {
	t.Parallel()
	f := typesOpenAPICmd.Flags().Lookup("output")
	if f == nil {
		t.Error("types openapi command missing --output flag")
	}
}

func TestTypesOpenAPICmd_hasDatabaseURLFlag(t *testing.T) {
	t.Parallel()
	f := typesOpenAPICmd.Flags().Lookup("database-url")
	if f == nil {
		t.Error("types openapi command missing --database-url flag")
	}
}

func TestRunTypesOpenAPI_missingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	cmd := &cobra.Command{}
	cmd.Flags().String("database-url", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	testutil.NoError(t, cmd.Flags().Set("database-url", ""))

	err := runTypesOpenAPI(cmd, nil)
	testutil.ErrorContains(t, err, "database-url")
}
