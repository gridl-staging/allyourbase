package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print AYB version",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"version": buildVersion,
				"commit":  buildCommit,
				"date":    buildDate,
			})
		}
		fmt.Printf("%s ayb %s (commit: %s, built: %s)\n", ui.BrandEmoji, buildVersion, buildCommit, buildDate)
		return nil
	},
}
