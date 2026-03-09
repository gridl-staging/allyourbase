package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show AYB server logs",
	Long: `Display recent server logs or stream them in real-time.

Examples:
  ayb logs                   # Show last 100 log lines
  ayb logs -n 50             # Show last 50 log lines
  ayb logs --follow          # Stream logs in real-time
  ayb logs --level error     # Filter by log level`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().IntP("lines", "n", 100, "Number of log lines to show")
	logsCmd.Flags().BoolP("follow", "f", false, "Stream logs in real-time")
	logsCmd.Flags().String("level", "", "Filter by log level (debug, info, warn, error)")
}

func runLogs(cmd *cobra.Command, args []string) error {
	lines, _ := cmd.Flags().GetInt("lines")
	follow, _ := cmd.Flags().GetBool("follow")
	level, _ := cmd.Flags().GetString("level")

	url := serverURL()
	if url == "" {
		return fmt.Errorf("cannot determine server URL (is AYB running?)")
	}

	// Build request URL
	endpoint := url + "/api/admin/logs"
	params := "?lines=" + strconv.Itoa(lines)
	if follow {
		params += "&follow=true"
	}
	if level != "" {
		params += "&level=" + level
	}

	client := &http.Client{Timeout: 0} // no timeout for streaming
	if !follow {
		client.Timeout = 10 * time.Second
	}

	req, err := http.NewRequest("GET", endpoint+params, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Add admin token if available
	token := adminToken()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Server doesn't support logs endpoint yet — show helpful message
		return fmt.Errorf("logs endpoint not available (server may need to be updated)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return serverError(resp.StatusCode, body)
	}

	format := outputFormat(cmd)
	if format == "json" {
		// Pass through raw JSON
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}

	// Stream text output
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

// adminToken returns the admin token, checking (in order):
//  1. AYB_ADMIN_TOKEN environment variable
//  2. ~/.ayb/admin-token file (contains the admin password, exchanged for a session token)
func adminToken() string {
	if v := os.Getenv("AYB_ADMIN_TOKEN"); v != "" {
		return v
	}
	if tokenPath, err := aybAdminTokenPath(); err == nil {
		if data, err := os.ReadFile(tokenPath); err == nil {
			password := strings.TrimSpace(string(data))
			if t, err := adminLogin(serverURL(), password); err == nil {
				return t
			}
		}
	}
	return ""
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show AYB server statistics",
	Long: `Display current server statistics including uptime, request counts,
active connections, and database pool info.

Examples:
  ayb stats             # Show stats in table format
  ayb stats --json      # Show stats as JSON`,
	RunE: runStats,
}

func runStats(cmd *cobra.Command, args []string) error {
	url := serverURL()
	if url == "" {
		return fmt.Errorf("cannot determine server URL (is AYB running?)")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url+"/api/admin/stats", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	token := adminToken()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("stats endpoint not available (server may need to be updated)")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return serverError(resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	format := outputFormat(cmd)
	if format == "json" {
		fmt.Println(string(body))
		return nil
	}

	// Parse JSON for table display
	var stats map[string]interface{}
	if err := json.Unmarshal(body, &stats); err != nil {
		// Not JSON, print raw
		fmt.Println(string(body))
		return nil
	}

	if format == "csv" {
		var cols []string
		var vals []string
		for k, v := range stats {
			cols = append(cols, k)
			vals = append(vals, fmt.Sprint(v))
		}
		return writeCSVStdout(cols, [][]string{vals})
	}

	// Table format
	fmt.Println("AYB Server Statistics")
	fmt.Println("─────────────────────")
	for k, v := range stats {
		fmt.Printf("  %-20s %v\n", k+":", v)
	}

	return nil
}

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage server secrets",
	Long:  `Manage sensitive server configuration like JWT secrets.`,
}

var secretsRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate the JWT secret",
	Long: `Generate a new JWT secret and update the configuration.
All existing tokens will be invalidated after rotation.

WARNING: This will sign out all currently authenticated users.

Examples:
  ayb secrets rotate                    # Rotate JWT secret
  ayb secrets rotate --config ayb.toml  # Rotate in specific config file`,
	RunE: runSecretsRotate,
}

func init() {
	secretsRotateCmd.Flags().String("config", "", "Path to ayb.toml config file")
	secretsCmd.AddCommand(secretsRotateCmd)
}

func runSecretsRotate(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	url := serverURL()
	if url == "" && configPath == "" {
		return fmt.Errorf("cannot determine server URL and no config file specified. Use --config flag or start the server first")
	}

	// If server is running, use the API
	if url != "" {
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("POST", url+"/api/admin/secrets/rotate", nil)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		token := adminToken()
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("connecting to server: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("secrets rotation endpoint not available (server may need to be updated)")
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
		}

		format := outputFormat(cmd)
		if format == "json" {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}
			fmt.Println(string(body))
			return nil
		}

		fmt.Println("JWT secret rotated successfully.")
		fmt.Println("All existing tokens have been invalidated.")
		return nil
	}

	// Offline mode: generate new secret and write to config
	return fmt.Errorf("offline secret rotation requires a running server. Start the server first")
}
