package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var sqlCmd = &cobra.Command{
	Use:   "sql [query]",
	Short: "Execute SQL against the running AYB server",
	Long: `Execute arbitrary SQL via the running AYB server's admin API.
Requires admin authentication if an admin password is set.

Authentication is resolved in order: --admin-token flag, AYB_ADMIN_TOKEN env var,
~/.ayb/admin-token file (auto-saved by ayb start). The saved password is
exchanged for a session token automatically.

Examples:
  ayb sql "SELECT * FROM users LIMIT 10"
  ayb sql "SELECT count(*) FROM posts" --json
  echo "SELECT 1" | ayb sql`,
	RunE: runSQL,
}

func init() {
	sqlCmd.Flags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	sqlCmd.Flags().String("url", "", "Server URL (default http://127.0.0.1:8090)")
}

func runSQL(cmd *cobra.Command, args []string) error {
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")

	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	// If no explicit token, try to log in with the saved admin password.
	if token == "" {
		if tokenPath, err := aybAdminTokenPath(); err == nil {
			if data, err := os.ReadFile(tokenPath); err == nil {
				password := strings.TrimSpace(string(data))
				if t, err := adminLogin(baseURL, password); err == nil {
					token = t
				}
			}
		}
	}

	// Get query from args or stdin.
	var query string
	if len(args) > 0 {
		query = strings.Join(args, " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		query = strings.TrimSpace(string(data))
	}
	if query == "" {
		return fmt.Errorf("query is required (pass as argument or pipe to stdin)")
	}

	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequest("POST", baseURL+"/api/admin/sql/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading server response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		if json.Unmarshal(respBody, &errResp) == nil {
			if msg, ok := errResp["message"].(string); ok {
				return fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
			}
		}
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
	}

	outFmt := outputFormat(cmd)
	if outFmt == "json" {
		os.Stdout.Write(respBody)
		fmt.Println()
		return nil
	}

	// Parse and display as table.
	var result struct {
		Columns    []string            `json:"columns"`
		Rows       [][]json.RawMessage `json:"rows"`
		RowCount   int                 `json:"rowCount"`
		DurationMs float64             `json:"durationMs"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	// Build string rows for both table and CSV output.
	strRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		vals := make([]string, len(row))
		for j, cell := range row {
			var v any
			if err := json.Unmarshal(cell, &v); err != nil {
				// Fallback: use raw JSON string if cell can't be parsed.
				vals[j] = string(cell)
			} else if v == nil {
				if outFmt == "csv" {
					vals[j] = ""
				} else {
					vals[j] = "NULL"
				}
			} else {
				vals[j] = fmt.Sprint(v)
			}
		}
		strRows[i] = vals
	}

	if outFmt == "csv" {
		return writeCSVStdout(result.Columns, strRows)
	}

	useColor := colorEnabledFd(os.Stdout.Fd())
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, bold(strings.Join(result.Columns, "\t"), useColor))
	fmt.Fprintln(w, strings.Repeat("---\t", len(result.Columns)))
	for _, vals := range strRows {
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	w.Flush()
	fmt.Printf("\n%s\n", dim(fmt.Sprintf("(%d rows, %.1fms)", result.RowCount, result.DurationMs), useColor))
	return nil
}

// adminLogin exchanges an admin password for a bearer token via /api/admin/auth.
func adminLogin(baseURL, password string) (string, error) {
	body, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		return "", fmt.Errorf("encoding login request: %w", err)
	}
	resp, err := http.Post(baseURL+"/api/admin/auth", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %d", resp.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

// serverURL returns the base URL for the running AYB server.
func serverURL() string {
	_, port, err := readAYBPID()
	if err == nil && port > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	return "http://127.0.0.1:8090"
}
