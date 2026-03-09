package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print resolved configuration",
	Long: `Load and print the resolved AYB configuration as TOML.
Shows the result of merging defaults, ayb.toml, environment variables, and flags.`,
	RunE: runConfig,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific configuration value",
	Long: `Get a specific configuration value by dotted key path.
Examples: server.port, database.url, auth.enabled, storage.backend`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value in ayb.toml",
	Long: `Set a configuration value in the ayb.toml config file.
Creates the file if it doesn't exist.
Examples:
  ayb config set server.port 3000
  ayb config set auth.enabled true
  ayb config set database.url postgresql://localhost:5432/mydb`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

func init() {
	configCmd.Flags().String("config", "", "Path to ayb.toml config file")
	configGetCmd.Flags().String("config", "", "Path to ayb.toml config file")
	configSetCmd.Flags().String("config", "", "Path to ayb.toml config file")

	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	jsonOut, _ := cmd.Flags().GetBool("json")

	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	masked := cfg.MaskedCopy()

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(masked)
	}

	out, err := masked.ToTOML()
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	fmt.Print(out)
	fmt.Fprintln(os.Stderr, "# Secrets are redacted. Edit ayb.toml to view or change them.")
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	value, err := config.GetValue(cfg, args[0])
	if err != nil {
		return err
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"key": args[0], "value": value})
	}

	fmt.Println(value)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = "ayb.toml"
	}

	key := args[0]
	value := args[1]

	// Validate the key is recognized.
	if !config.IsValidKey(key) {
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	if err := config.SetValue(configPath, key, value); err != nil {
		return fmt.Errorf("setting config value: %w", err)
	}

	fmt.Printf("%s = %s\n", key, value)
	fmt.Printf("Written to %s\n", configPath)

	// Validate the resulting config.
	cfg, err := config.Load(configPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: config file has errors: %v\n", err)
	} else if err := cfg.Validate(); err != nil {
		// Only warn, don't fail — user may be setting values incrementally.
		parts := strings.SplitN(err.Error(), ": ", 2)
		if len(parts) > 1 {
			fmt.Fprintf(os.Stderr, "Note: %s\n", parts[1])
		}
	}

	return nil
}
