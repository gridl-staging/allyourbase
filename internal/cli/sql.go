// Package cli defines the sql command for executing arbitrary SQL against a running AYB server via its admin API. It handles authentication, query input, and supports multiple output formats.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
~/.ayb/admin-token file (auto-saved by ayb start) for loopback servers only.
For backward compatibility, the file may contain either a bearer token or a
legacy password value.

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

// TODO: Document runSQL.
func runSQL(cmd *cobra.Command, args []string) error {
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")

	if baseURL == "" {
		baseURL = serverURL()
	}
	token = resolveCLIAdminToken(token, baseURL)
	if token == "" && !isLoopbackAdminURL(baseURL) {
		return fmt.Errorf("saved admin auth from ~/.ayb/admin-token is only used for local loopback servers; pass --admin-token or set AYB_ADMIN_TOKEN for %s", baseURL)
	}

	query, err := readSQLQuery(args, os.Stdin)
	if err != nil {
		return err
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

	strRows := stringifySQLRows(result.Rows, outFmt)

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

func resolveCLIAdminToken(explicitToken, baseURL string) string {
	if explicitToken != "" {
		return explicitToken
	}
	if token := os.Getenv("AYB_ADMIN_TOKEN"); token != "" {
		return token
	}
	return resolveSavedAdminToken(baseURL)
}

func readSQLQuery(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// TODO: Document stringifySQLRows.
func stringifySQLRows(rows [][]json.RawMessage, outFmt string) [][]string {
	strRows := make([][]string, len(rows))
	for i, row := range rows {
		vals := make([]string, len(row))
		for j, cell := range row {
			var v any
			if err := json.Unmarshal(cell, &v); err != nil {
				vals[j] = string(cell)
				continue
			}
			if v == nil {
				if outFmt == "csv" {
					vals[j] = ""
				} else {
					vals[j] = "NULL"
				}
				continue
			}
			vals[j] = fmt.Sprint(v)
		}
		strRows[i] = vals
	}
	return strRows
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

// resolveSavedAdminToken resolves auth from ~/.ayb/admin-token.
// New servers save a bearer token; older versions saved a password.
func resolveSavedAdminToken(baseURL string) string {
	if !isLoopbackAdminURL(baseURL) {
		return ""
	}
	_, saved, err := readSavedAdminTokenFile()
	if err != nil || saved == "" {
		return ""
	}
	return exchangeSavedAdminAuth(baseURL, saved)
}

func readSavedAdminTokenFile() (string, string, error) {
	tokenPath, err := aybAdminTokenPath()
	if err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return tokenPath, "", err
	}
	return tokenPath, strings.TrimSpace(string(data)), nil
}

func exchangeSavedAdminAuth(baseURL, saved string) string {
	if exchanged, err := adminLogin(baseURL, saved); err == nil {
		exchanged = strings.TrimSpace(exchanged)
		if exchanged != "" {
			return exchanged
		}
	}
	return saved
}

func isLoopbackAdminURL(baseURL string) bool {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.TrimSpace(parsedURL.Hostname())
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// serverURL returns the base URL for the running AYB server.
func serverURL() string {
	_, port, err := readAYBPID()
	if err == nil && port > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	return "http://127.0.0.1:8090"
}
