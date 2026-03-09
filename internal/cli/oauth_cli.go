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

var oauthCmd = &cobra.Command{
	Use:   "oauth",
	Short: "Manage OAuth 2.0 provider resources",
}

var oauthClientsCmd = &cobra.Command{
	Use:   "clients",
	Short: "Manage OAuth 2.0 clients",
}

var oauthClientsCreateCmd = &cobra.Command{
	Use:   "create <app-id>",
	Short: "Register a new OAuth client for an app",
	Args:  cobra.ExactArgs(1),
	RunE:  runOAuthClientsCreate,
}

var oauthClientsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all OAuth clients",
	RunE:  runOAuthClientsList,
}

var oauthClientsDeleteCmd = &cobra.Command{
	Use:   "delete <client-id>",
	Short: "Revoke an OAuth client (soft-delete)",
	Args:  cobra.ExactArgs(1),
	RunE:  runOAuthClientsDelete,
}

var oauthClientsRotateSecretCmd = &cobra.Command{
	Use:   "rotate-secret <client-id>",
	Short: "Regenerate the client secret for a confidential OAuth client",
	Args:  cobra.ExactArgs(1),
	RunE:  runOAuthClientsRotateSecret,
}

func init() {
	oauthClientsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	oauthClientsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	oauthClientsCreateCmd.Flags().String("name", "", "Client name (required)")
	oauthClientsCreateCmd.Flags().StringSlice("redirect-uris", nil, "Redirect URIs (comma-separated, required)")
	oauthClientsCreateCmd.Flags().StringSlice("scopes", nil, "Scopes: readonly, readwrite, * (comma-separated, required)")
	oauthClientsCreateCmd.Flags().String("type", "confidential", "Client type: confidential or public")

	oauthClientsCmd.AddCommand(oauthClientsCreateCmd)
	oauthClientsCmd.AddCommand(oauthClientsListCmd)
	oauthClientsCmd.AddCommand(oauthClientsDeleteCmd)
	oauthClientsCmd.AddCommand(oauthClientsRotateSecretCmd)

	oauthCmd.AddCommand(oauthClientsCmd)
	rootCmd.AddCommand(oauthCmd)
}

func runOAuthClientsCreate(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	appID := args[0]

	name, _ := cmd.Flags().GetString("name")
	redirectURIs, _ := cmd.Flags().GetStringSlice("redirect-uris")
	scopes, _ := cmd.Flags().GetStringSlice("scopes")
	clientType, _ := cmd.Flags().GetString("type")

	if name == "" {
		return fmt.Errorf("--name is required")
	}
	redirectURIs = filterEmpty(redirectURIs)
	scopes = filterEmpty(scopes)
	if len(redirectURIs) == 0 {
		return fmt.Errorf("--redirect-uris is required")
	}
	if len(scopes) == 0 {
		return fmt.Errorf("--scopes is required")
	}

	payload := map[string]any{
		"appId":        appID,
		"name":         name,
		"clientType":   clientType,
		"redirectUris": redirectURIs,
		"scopes":       scopes,
	}
	body, _ := json.Marshal(payload)

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/oauth/clients", bytes.NewReader(body))
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

	var result struct {
		ClientSecret string `json:"clientSecret"`
		Client       struct {
			ID         string   `json:"id"`
			ClientID   string   `json:"clientId"`
			Name       string   `json:"name"`
			ClientType string   `json:"clientType"`
			Scopes     []string `json:"scopes"`
		} `json:"client"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("OAuth client created: %s (%s)\n", result.Client.ClientID, result.Client.Name)
	fmt.Printf("Type: %s\n", result.Client.ClientType)
	fmt.Printf("Scopes: %s\n", strings.Join(result.Client.Scopes, ", "))

	if result.ClientSecret != "" {
		fmt.Printf("\nClient Secret: %s\n", result.ClientSecret)
		fmt.Println("\nSave this secret — it will not be shown again.")
	}
	return nil
}

func runOAuthClientsList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/oauth/clients", nil)
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
			ID           string   `json:"id"`
			AppID        string   `json:"appId"`
			ClientID     string   `json:"clientId"`
			Name         string   `json:"name"`
			RedirectURIs []string `json:"redirectUris"`
			Scopes       []string `json:"scopes"`
			ClientType   string   `json:"clientType"`
			CreatedAt    string   `json:"createdAt"`
			RevokedAt    *string  `json:"revokedAt"`
		} `json:"items"`
		TotalItems int `json:"totalItems"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No OAuth clients registered.")
		return nil
	}

	cols := []string{"Client ID", "Name", "App ID", "Type", "Scopes", "Created", "Status"}
	rows := make([][]string, len(result.Items))
	for i, c := range result.Items {
		status := "active"
		if c.RevokedAt != nil {
			status = "revoked"
		}
		rows[i] = []string{
			c.ClientID,
			c.Name,
			c.AppID,
			c.ClientType,
			strings.Join(c.Scopes, ","),
			c.CreatedAt,
			status,
		}
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
	fmt.Printf("\n%d oauth client(s)\n", result.TotalItems)
	return nil
}

func runOAuthClientsDelete(cmd *cobra.Command, args []string) error {
	clientID := args[0]

	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/oauth/clients/"+clientID, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("OAuth client %s revoked.\n", clientID)
		return nil
	}
	return serverError(resp.StatusCode, body)
}

func runOAuthClientsRotateSecret(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	clientID := args[0]

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/oauth/clients/"+clientID+"/rotate-secret", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, respBody)
	}

	if outFmt == "json" {
		os.Stdout.Write(respBody)
		fmt.Println()
		return nil
	}

	var result struct {
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("New Client Secret: %s\n", result.ClientSecret)
	fmt.Println("\nSave this secret — it will not be shown again.")
	return nil
}

// filterEmpty removes empty strings from a slice.
func filterEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
