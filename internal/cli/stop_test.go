package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/spf13/cobra"
)

func TestRunStopUsesRequestedPortForOrphanDetection(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	aybDir := filepath.Join(homeDir, ".ayb")
	testutil.NoError(t, os.MkdirAll(aybDir, 0o755))

	originalPortInUse := stopPortInUse
	stopPortInUse = func(port int) bool {
		return port == 8090
	}
	defer func() {
		stopPortInUse = originalPortInUse
	}()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Int("port", 0, "")
	testutil.NoError(t, cmd.Flags().Set("port", "18090"))

	output := captureStdout(t, func() {
		testutil.NoError(t, runStop(cmd, nil))
	})
	testutil.Contains(t, output, "No AYB server is running")
	testutil.False(t, strings.Contains(output, "port 8090 is in use"))
}
