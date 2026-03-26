// Package cli audit_cli.go implements the audit command and its export subcommand for exporting and formatting audit log entries from the server.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit log tools",
}

var auditExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export audit log entries",
	RunE:  runAuditExport,
}

type auditExportEntry struct {
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	UserID    string          `json:"user_id"`
	APIKeyID  string          `json:"api_key_id"`
	TableName string          `json:"table_name"`
	Operation string          `json:"operation"`
	RecordID  json.RawMessage `json:"record_id"`
	OldValues json.RawMessage `json:"old_values"`
	NewValues json.RawMessage `json:"new_values"`
	IPAddress string          `json:"ip_address"`
}

func init() {
	auditCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	auditCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	auditExportCmd.Flags().String("from", "", "Start date/time filter (YYYY-MM-DD or RFC3339)")
	auditExportCmd.Flags().String("to", "", "End date/time filter (YYYY-MM-DD or RFC3339)")
	auditExportCmd.Flags().String("table", "", "Filter by table name")
	auditExportCmd.Flags().String("user-id", "", "Filter by user ID")
	auditExportCmd.Flags().String("operation", "", "Filter by operation: INSERT|UPDATE|DELETE")
	auditExportCmd.Flags().Int("limit", 100, "Maximum rows per export call")
	auditExportCmd.Flags().Int("offset", 0, "Pagination offset")
	auditExportCmd.Flags().String("format", "json", "Export format: json or csv")

	auditCmd.AddCommand(auditExportCmd)
	rootCmd.AddCommand(auditCmd)
}

// runAuditExport handles the audit export command by querying the server's audit log with optional filters (date range, table name, user ID, operation type, and pagination) and outputs the results in JSON or CSV format.
func runAuditExport(cmd *cobra.Command, _ []string) error {
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	table, _ := cmd.Flags().GetString("table")
	userID, _ := cmd.Flags().GetString("user-id")
	operation, _ := cmd.Flags().GetString("operation")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	format, _ := cmd.Flags().GetString("format")
	format = strings.ToLower(strings.TrimSpace(format))

	if format != "json" && format != "csv" {
		return fmt.Errorf("invalid format %q: must be json or csv", format)
	}

	query := make(url.Values)
	if strings.TrimSpace(from) != "" {
		query.Set("from", strings.TrimSpace(from))
	}
	if strings.TrimSpace(to) != "" {
		query.Set("to", strings.TrimSpace(to))
	}
	if strings.TrimSpace(table) != "" {
		query.Set("table", strings.TrimSpace(table))
	}
	if strings.TrimSpace(userID) != "" {
		query.Set("user_id", strings.TrimSpace(userID))
	}
	if op := strings.ToUpper(strings.TrimSpace(operation)); op != "" {
		query.Set("operation", op)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", offset))
	}

	path := "/api/admin/audit"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	resp, body, err := adminRequest(cmd, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var payload struct {
		Items []auditExportEntry `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload.Items)
	}

	cols := []string{"id", "timestamp", "user_id", "api_key_id", "table_name", "operation", "record_id", "old_values", "new_values", "ip_address"}
	rows := make([][]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		rows = append(rows, []string{
			item.ID,
			item.Timestamp,
			item.UserID,
			item.APIKeyID,
			item.TableName,
			item.Operation,
			rawJSONToString(item.RecordID),
			rawJSONToString(item.OldValues),
			rawJSONToString(item.NewValues),
			item.IPAddress,
		})
	}
	return writeCSVStdout(cols, rows)
}

func rawJSONToString(v json.RawMessage) string {
	if len(v) == 0 || string(v) == "null" {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, v); err == nil {
		return compact.String()
	}
	return string(v)
}
