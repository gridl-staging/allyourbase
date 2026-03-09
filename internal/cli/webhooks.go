package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var webhooksCmd = &cobra.Command{
	Use:   "webhooks",
	Short: "Manage webhooks on the running AYB server",
}

var webhooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all webhooks",
	RunE:  runWebhooksList,
}

var webhooksCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new webhook",
	RunE:  runWebhooksCreate,
}

var webhooksDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a webhook",
	Args:  cobra.ExactArgs(1),
	RunE:  runWebhooksDelete,
}

func init() {
	webhooksCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	webhooksCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	webhooksCreateCmd.Flags().String("webhook-url", "", "Webhook destination URL (required)")
	webhooksCreateCmd.Flags().String("events", "", "Comma-separated events: create,update,delete (default all)")
	webhooksCreateCmd.Flags().String("tables", "", "Comma-separated table filter (default all tables)")
	webhooksCreateCmd.Flags().String("secret", "", "HMAC-SHA256 signing secret")
	webhooksCreateCmd.Flags().Bool("disabled", false, "Create in disabled state")

	webhooksCmd.AddCommand(webhooksListCmd)
	webhooksCmd.AddCommand(webhooksCreateCmd)
	webhooksCmd.AddCommand(webhooksDeleteCmd)
}

func runWebhooksList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/webhooks", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var hooks []struct {
		ID        string   `json:"id"`
		URL       string   `json:"url"`
		HasSecret bool     `json:"hasSecret"`
		Events    []string `json:"events"`
		Tables    []string `json:"tables"`
		Enabled   bool     `json:"enabled"`
		CreatedAt string   `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &hooks); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(hooks) == 0 {
		fmt.Println("No webhooks configured.")
		return nil
	}

	// Build string rows for table and CSV output.
	cols := []string{"ID", "URL", "Events", "Tables", "Enabled", "Secret"}
	rows := make([][]string, len(hooks))
	for i, h := range hooks {
		tables := "*"
		if len(h.Tables) > 0 {
			tables = strings.Join(h.Tables, ",")
		}
		secret := "no"
		if h.HasSecret {
			secret = "yes"
		}
		rows[i] = []string{h.ID, h.URL, strings.Join(h.Events, ","), tables, fmt.Sprintf("%v", h.Enabled), secret}
	}

	if outFmt == "csv" {
		return writeCSVStdout(cols, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
	fmt.Printf("\n%d webhook(s)\n", len(hooks))
	return nil
}

func runWebhooksCreate(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	whURL, _ := cmd.Flags().GetString("webhook-url")
	events, _ := cmd.Flags().GetString("events")
	tables, _ := cmd.Flags().GetString("tables")
	secret, _ := cmd.Flags().GetString("secret")
	disabled, _ := cmd.Flags().GetBool("disabled")

	if whURL == "" {
		return fmt.Errorf("--webhook-url is required")
	}

	payload := map[string]any{
		"url":     whURL,
		"enabled": !disabled,
	}
	if secret != "" {
		payload["secret"] = secret
	}
	if events != "" {
		payload["events"] = strings.Split(events, ",")
	}
	if tables != "" {
		payload["tables"] = strings.Split(tables, ",")
	}

	body, _ := json.Marshal(payload)
	resp, respBody, err := adminRequest(cmd, "POST", "/api/webhooks", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, respBody)
	}

	if outFmt == "json" {
		os.Stdout.Write(respBody)
		fmt.Println()
		return nil
	}

	var created struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return fmt.Errorf("parsing webhook response: %w", err)
	}
	fmt.Printf("Webhook created: %s → %s\n", created.ID, created.URL)
	return nil
}

func runWebhooksDelete(cmd *cobra.Command, args []string) error {
	id := args[0]

	resp, body, err := adminRequest(cmd, "DELETE", "/api/webhooks/"+id, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("Webhook %s deleted.\n", id)
		return nil
	}
	return serverError(resp.StatusCode, body)
}

// serverError extracts an error message from an API error response.
func serverError(status int, body []byte) error {
	if status == http.StatusUnauthorized {
		return fmt.Errorf("authentication required (401)\n\n" +
			"  The server requires admin authentication. AYB reads the token\n" +
			"  automatically from ~/.ayb/admin-token when the server is running.\n\n" +
			"  If that file is missing, set the token manually:\n" +
			"    export AYB_ADMIN_TOKEN=$(cat ~/.ayb/admin-token)\n\n" +
			"  Or reset the admin password:\n" +
			"    ayb admin reset-password")
	}
	var errResp map[string]any
	if json.Unmarshal(body, &errResp) == nil {
		if msg, ok := errResp["message"].(string); ok {
			return fmt.Errorf("server error (%d): %s", status, msg)
		}
	}
	return fmt.Errorf("server error (%d): %s", status, string(body))
}
