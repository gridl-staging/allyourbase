package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var matviewsCmd = &cobra.Command{
	Use:   "matviews",
	Short: "Manage materialized view registrations",
}

var matviewsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered materialized views",
	RunE:  runMatviewsList,
}

var matviewsRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a materialized view for refresh management",
	RunE:  runMatviewsRegister,
}

var matviewsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update refresh mode for a registered materialized view",
	Args:  cobra.ExactArgs(1),
	RunE:  runMatviewsUpdate,
}

var matviewsUnregisterCmd = &cobra.Command{
	Use:   "unregister <id>",
	Short: "Unregister a materialized view",
	Args:  cobra.ExactArgs(1),
	RunE:  runMatviewsUnregister,
}

var matviewsRefreshCmd = &cobra.Command{
	Use:   "refresh <id|schema.view>",
	Short: "Trigger an immediate refresh of a materialized view",
	Args:  cobra.ExactArgs(1),
	RunE:  runMatviewsRefresh,
}

func init() {
	matviewsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	matviewsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	matviewsRegisterCmd.Flags().String("schema", "public", "Schema name")
	matviewsRegisterCmd.Flags().String("view", "", "View name (required)")
	matviewsRegisterCmd.Flags().String("mode", "standard", "Refresh mode (standard or concurrent)")

	matviewsUpdateCmd.Flags().String("mode", "", "Refresh mode (standard or concurrent, required)")

	matviewsCmd.AddCommand(matviewsListCmd)
	matviewsCmd.AddCommand(matviewsRegisterCmd)
	matviewsCmd.AddCommand(matviewsUpdateCmd)
	matviewsCmd.AddCommand(matviewsUnregisterCmd)
	matviewsCmd.AddCommand(matviewsRefreshCmd)

	rootCmd.AddCommand(matviewsCmd)
}

func runMatviewsList(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/matviews", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("server error: %s", string(body))
	}

	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if outFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	}

	if len(result.Items) == 0 {
		fmt.Println("No materialized views registered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSCHEMA\tVIEW\tMODE\tLAST REFRESH\tSTATUS")
	for _, mv := range result.Items {
		id, _ := mv["id"].(string)
		schema, _ := mv["schemaName"].(string)
		view, _ := mv["viewName"].(string)
		mode, _ := mv["refreshMode"].(string)
		lastRefresh, _ := mv["lastRefreshAt"].(string)
		status, _ := mv["lastRefreshStatus"].(string)
		if lastRefresh == "" {
			lastRefresh = "never"
		} else if len(lastRefresh) > 19 {
			lastRefresh = lastRefresh[:19]
		}
		if status == "" {
			status = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			id, schema, view, mode, lastRefresh, status)
	}
	return w.Flush()
}

func runMatviewsRegister(cmd *cobra.Command, _ []string) error {
	schema, _ := cmd.Flags().GetString("schema")
	view, _ := cmd.Flags().GetString("view")
	mode, _ := cmd.Flags().GetString("mode")

	if view == "" {
		return fmt.Errorf("--view is required")
	}
	if mode != "standard" && mode != "concurrent" {
		return fmt.Errorf("--mode must be 'standard' or 'concurrent'")
	}

	payload := map[string]any{
		"schema":      schema,
		"viewName":    view,
		"refreshMode": mode,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/matviews", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return fmt.Errorf("register failed: %s", string(respBody))
	}

	var reg map[string]any
	if err := json.Unmarshal(respBody, &reg); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Materialized view %q registered (id: %s, mode: %s)\n",
		reg["viewName"], reg["id"], reg["refreshMode"])
	return nil
}

func runMatviewsUpdate(cmd *cobra.Command, args []string) error {
	id := args[0]
	mode, _ := cmd.Flags().GetString("mode")

	if mode == "" {
		return fmt.Errorf("--mode is required")
	}
	if mode != "standard" && mode != "concurrent" {
		return fmt.Errorf("--mode must be 'standard' or 'concurrent'")
	}

	payload := map[string]any{
		"refreshMode": mode,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, respBody, err := adminRequest(cmd, "PUT", "/api/admin/matviews/"+url.PathEscape(id), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("update failed: %s", string(respBody))
	}

	var reg map[string]any
	if err := json.Unmarshal(respBody, &reg); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Materialized view %q updated (mode: %s)\n",
		reg["viewName"], reg["refreshMode"])
	return nil
}

func runMatviewsUnregister(cmd *cobra.Command, args []string) error {
	id := args[0]

	resp, respBody, err := adminRequest(cmd, "DELETE", "/api/admin/matviews/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("unregister failed: %s", string(respBody))
	}
	fmt.Printf("Materialized view %s unregistered\n", id)
	return nil
}

func runMatviewsRefresh(cmd *cobra.Command, args []string) error {
	id, err := resolveMatviewRefreshID(cmd, args[0])
	if err != nil {
		return err
	}

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/matviews/"+url.PathEscape(id)+"/refresh", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("refresh failed: %s", string(respBody))
	}

	var result struct {
		Registration map[string]any `json:"registration"`
		DurationMs   int            `json:"durationMs"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Refreshed %q (%dms)\n",
		result.Registration["viewName"], result.DurationMs)
	return nil
}

func resolveMatviewRefreshID(cmd *cobra.Command, target string) (string, error) {
	if !strings.Contains(target, ".") {
		return target, nil
	}

	parts := strings.SplitN(target, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid matview reference %q (expected schema.view)", target)
	}

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/matviews", nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to resolve matview %q: %s", target, string(body))
	}

	var result struct {
		Items []struct {
			ID         string `json:"id"`
			SchemaName string `json:"schemaName"`
			ViewName   string `json:"viewName"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	schemaName := parts[0]
	viewName := parts[1]
	for _, item := range result.Items {
		if item.SchemaName == schemaName && item.ViewName == viewName {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("materialized view %q is not registered", target)
}
