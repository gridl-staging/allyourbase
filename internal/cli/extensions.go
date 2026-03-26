// Package cli extensions.go provides CLI commands to list, enable, and disable PostgreSQL extensions on a managed database, with optional persistence to configuration.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/spf13/cobra"
)

var extensionsCmd = &cobra.Command{
	Use:   "extensions",
	Short: "Manage PostgreSQL extensions",
	Long:  `List, enable, and disable PostgreSQL extensions on the connected database.`,
}

var extensionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available and installed extensions",
	Long: `List all PostgreSQL extensions available on the server.

Examples:
  ayb extensions list
  ayb extensions list --json
  ayb extensions list --output csv`,
	RunE: runExtensionsList,
}

var extensionsEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a PostgreSQL extension",
	Long: `Install a PostgreSQL extension. Runs CREATE EXTENSION IF NOT EXISTS.

If --config is specified, also persists the extension to the managed_pg.extensions
list in the given ayb.toml so it is re-enabled on next startup.

Examples:
  ayb extensions enable pgvector
  ayb extensions enable pg_trgm --config ayb.toml`,
	Args: cobra.ExactArgs(1),
	RunE: runExtensionsEnable,
}

var extensionsDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a PostgreSQL extension",
	Long: `Remove a PostgreSQL extension. Runs DROP EXTENSION IF EXISTS.

If --config is specified, also removes the extension from the managed_pg.extensions
list in the given ayb.toml.

Examples:
  ayb extensions disable pgvector
  ayb extensions disable pgvector --config ayb.toml`,
	Args: cobra.ExactArgs(1),
	RunE: runExtensionsDisable,
}

func init() {
	extensionsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	extensionsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	extensionsEnableCmd.Flags().String("config", "", "Path to ayb.toml to persist the change")
	extensionsDisableCmd.Flags().String("config", "", "Path to ayb.toml to persist the change")

	extensionsCmd.AddCommand(extensionsListCmd)
	extensionsCmd.AddCommand(extensionsEnableCmd)
	extensionsCmd.AddCommand(extensionsDisableCmd)

	rootCmd.AddCommand(extensionsCmd)
}

// --- handlers ---

// runExtensionsList retrieves extensions from the server via the admin API and prints them in JSON, CSV, or table format according to the --output flag.
func runExtensionsList(cmd *cobra.Command, _ []string) error {
	resp, body, err := adminRequest(cmd, "GET", "/api/admin/extensions", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Extensions []extensionItem `json:"extensions"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	format := outputFormat(cmd)
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Extensions)
	case "csv":
		return writeCSVStdout(
			[]string{"Name", "Installed", "Available", "InstalledVersion", "DefaultVersion"},
			extensionsToCSVRows(result.Extensions),
		)
	default:
		return printExtensionsTable(result.Extensions)
	}
}

// runExtensionsEnable enables a PostgreSQL extension on the server via the admin API and optionally adds it to the ayb.toml config file if the --config flag is provided.
func runExtensionsEnable(cmd *cobra.Command, args []string) error {
	name := args[0]
	payload, _ := json.Marshal(map[string]string{"name": name})

	resp, body, err := adminRequest(cmd, "POST", "/api/admin/extensions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	if cfgPath, _ := cmd.Flags().GetString("config"); cfgPath != "" {
		if err := config.AddExtension(cfgPath, name); err != nil {
			return fmt.Errorf("persisting extension to config: %w", err)
		}
	}

	fmt.Printf("Extension %q enabled.\n", name)
	return nil
}

// runExtensionsDisable disables a PostgreSQL extension on the server via the admin API and optionally removes it from the ayb.toml config file if the --config flag is provided.
func runExtensionsDisable(cmd *cobra.Command, args []string) error {
	name := args[0]

	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/extensions/"+name, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return serverError(resp.StatusCode, body)
	}

	if cfgPath, _ := cmd.Flags().GetString("config"); cfgPath != "" {
		if err := config.RemoveExtension(cfgPath, name); err != nil {
			return fmt.Errorf("persisting removal to config: %w", err)
		}
	}

	fmt.Printf("Extension %q disabled.\n", name)
	return nil
}

// --- output helpers ---

type extensionItem struct {
	Name             string `json:"name"`
	Installed        bool   `json:"installed"`
	Available        bool   `json:"available"`
	InstalledVersion string `json:"installed_version,omitempty"`
	DefaultVersion   string `json:"default_version,omitempty"`
}

// printExtensionsTable formats extensions as a table and prints them to stdout, displaying name, installation status, installed version, and available version for each extension.
func printExtensionsTable(exts []extensionItem) error {
	if len(exts) == 0 {
		fmt.Println("No extensions found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tINSTALLED\tVERSION\tAVAILABLE VERSION")
	for _, e := range exts {
		installed := "no"
		if e.Installed {
			installed = "yes"
		}
		ver := "-"
		if e.InstalledVersion != "" {
			ver = e.InstalledVersion
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Name, installed, ver, e.DefaultVersion)
	}
	w.Flush()
	fmt.Printf("\n%d extension(s)\n", len(exts))
	return nil
}

func extensionsToCSVRows(exts []extensionItem) [][]string {
	rows := make([][]string, len(exts))
	for i, e := range exts {
		installed := "false"
		if e.Installed {
			installed = "true"
		}
		available := "false"
		if e.Available {
			available = "true"
		}
		rows[i] = []string{e.Name, installed, available, e.InstalledVersion, e.DefaultVersion}
	}
	return rows
}
