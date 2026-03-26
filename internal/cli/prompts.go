// Package cli provides commands for managing AI prompt templates, supporting listing, retrieval, creation, and rendering of versioned prompts.
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

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Manage AI prompt templates",
	Long: `Manage versioned AI prompt templates.

Examples:
  ayb prompts list
  ayb prompts get <id>
  ayb prompts create --name greeting --template "Hello {{name}}"
  ayb prompts delete <id>
  ayb prompts render <id> --var name=World`,
}

var promptsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all prompt templates",
	RunE:  runPromptsList,
}

var promptsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a prompt template by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runPromptsGet,
}

var promptsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new prompt template",
	RunE:  runPromptsCreate,
}

var promptsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a prompt template",
	Args:  cobra.ExactArgs(1),
	RunE:  runPromptsDelete,
}

var promptsRenderCmd = &cobra.Command{
	Use:   "render <id>",
	Short: "Render a prompt template with variables",
	Args:  cobra.ExactArgs(1),
	RunE:  runPromptsRender,
}

func init() {
	promptsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	promptsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	promptsCreateCmd.Flags().String("name", "", "Prompt name (required)")
	promptsCreateCmd.Flags().String("template", "", "Prompt template (required)")
	promptsCreateCmd.Flags().String("model", "", "Default model")
	promptsCreateCmd.Flags().String("provider", "", "Default provider")

	promptsRenderCmd.Flags().StringSlice("var", nil, "Variable in key=value format (repeatable)")

	promptsCmd.AddCommand(promptsListCmd)
	promptsCmd.AddCommand(promptsGetCmd)
	promptsCmd.AddCommand(promptsCreateCmd)
	promptsCmd.AddCommand(promptsDeleteCmd)
	promptsCmd.AddCommand(promptsRenderCmd)
}

// runPromptsList lists all prompt templates, fetching from the admin API and displaying results in the requested output format (json, csv, or table).
func runPromptsList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/ai/prompts", nil)
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
		Prompts []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Version  int    `json:"version"`
			Model    string `json:"model"`
			Provider string `json:"provider"`
		} `json:"prompts"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Prompts) == 0 {
		fmt.Println("No prompts configured.")
		return nil
	}

	cols := []string{"ID", "Name", "Version", "Model", "Provider"}
	rows := make([][]string, len(result.Prompts))
	for i, p := range result.Prompts {
		rows[i] = []string{p.ID, p.Name, fmt.Sprintf("%d", p.Version), p.Model, p.Provider}
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
	fmt.Printf("\n%d prompt(s)\n", len(result.Prompts))
	return nil
}

// runPromptsGet retrieves a single prompt template by ID and displays its details including the template text and configuration.
func runPromptsGet(cmd *cobra.Command, args []string) error {
	id := args[0]
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/ai/prompts/"+id, nil)
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

	var p struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Version  int    `json:"version"`
		Template string `json:"template"`
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("ID:       %s\n", p.ID)
	fmt.Printf("Name:     %s\n", p.Name)
	fmt.Printf("Version:  %d\n", p.Version)
	fmt.Printf("Model:    %s\n", p.Model)
	fmt.Printf("Provider: %s\n", p.Provider)
	fmt.Printf("Template:\n%s\n", p.Template)
	return nil
}

// runPromptsCreate creates a new prompt template with the provided name and template text, optionally setting a default model and provider.
func runPromptsCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	template, _ := cmd.Flags().GetString("template")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")

	if name == "" || template == "" {
		return fmt.Errorf("--name and --template are required")
	}

	payload := map[string]any{"name": name, "template": template}
	if model != "" {
		payload["model"] = model
	}
	if provider != "" {
		payload["provider"] = provider
	}

	data, _ := json.Marshal(payload)
	resp, body, err := adminRequest(cmd, "POST", "/api/admin/ai/prompts", bytes.NewReader(data))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, body)
	}

	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(body, &created)
	fmt.Printf("Prompt %q created (id: %s).\n", created.Name, created.ID)
	return nil
}

func runPromptsDelete(cmd *cobra.Command, args []string) error {
	id := args[0]

	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/ai/prompts/"+id, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return serverError(resp.StatusCode, body)
	}

	fmt.Printf("Prompt %s deleted.\n", id)
	return nil
}

// runPromptsRender renders a prompt template by ID, substituting variables provided in key=value format, and outputs the rendered result.
func runPromptsRender(cmd *cobra.Command, args []string) error {
	id := args[0]
	vars, _ := cmd.Flags().GetStringSlice("var")

	variables := make(map[string]any)
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid variable format %q; expected key=value", v)
		}
		variables[parts[0]] = parts[1]
	}

	payload, _ := json.Marshal(map[string]any{"variables": variables})
	resp, body, err := adminRequest(cmd, "POST", "/api/admin/ai/prompts/"+id+"/render", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Rendered string `json:"rendered"`
	}
	json.Unmarshal(body, &result)
	fmt.Println(result.Rendered)
	return nil
}
