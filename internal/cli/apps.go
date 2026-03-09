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

var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "Manage applications on the running AYB server",
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered applications",
	RunE:  runAppsList,
}

var appsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new application",
	Args:  cobra.ExactArgs(1),
	RunE:  runAppsCreate,
}

var appsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an application",
	Args:  cobra.ExactArgs(1),
	RunE:  runAppsDelete,
}

func init() {
	appsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	appsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	appsCreateCmd.Flags().String("description", "", "App description")
	appsCreateCmd.Flags().String("owner-id", "", "Owner user ID (required)")

	appsCmd.AddCommand(appsListCmd)
	appsCmd.AddCommand(appsCreateCmd)
	appsCmd.AddCommand(appsDeleteCmd)
}


func runAppsList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/apps", nil)
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

	var result struct {
		Items []struct {
			ID                     string `json:"id"`
			Name                   string `json:"name"`
			Description            string `json:"description"`
			OwnerUserID            string `json:"ownerUserId"`
			RateLimitRPS           int    `json:"rateLimitRps"`
			RateLimitWindowSeconds int    `json:"rateLimitWindowSeconds"`
			CreatedAt              string `json:"createdAt"`
		} `json:"items"`
		TotalItems int `json:"totalItems"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No apps registered.")
		return nil
	}

	cols := []string{"ID", "Name", "Description", "Owner", "Rate Limit", "Created"}
	rows := make([][]string, len(result.Items))
	for i, a := range result.Items {
		rateLimit := "none"
		if a.RateLimitRPS > 0 {
			rateLimit = fmt.Sprintf("%d req/%ds", a.RateLimitRPS, a.RateLimitWindowSeconds)
		}
		rows[i] = []string{a.ID, a.Name, a.Description, a.OwnerUserID, rateLimit, a.CreatedAt}
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
	fmt.Printf("\n%d app(s)\n", result.TotalItems)
	return nil
}

func runAppsCreate(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	name := args[0]
	description, _ := cmd.Flags().GetString("description")
	ownerID, _ := cmd.Flags().GetString("owner-id")

	if ownerID == "" {
		return fmt.Errorf("--owner-id is required")
	}

	payload := map[string]any{
		"name":        name,
		"description": description,
		"ownerUserId": ownerID,
	}
	body, _ := json.Marshal(payload)

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/apps", bytes.NewReader(body))
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

	var app struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &app); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("App created: %s (%s)\n", app.ID, app.Name)
	return nil
}

func runAppsDelete(cmd *cobra.Command, args []string) error {
	id := args[0]

	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/apps/"+id, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("App %s deleted.\n", id)
		return nil
	}
	return serverError(resp.StatusCode, body)
}
