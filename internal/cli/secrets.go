// Package cli secrets.go provides CLI commands for managing encrypted secrets with operations to create, retrieve, list, and delete.
package cli

import (
	"bufio"
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

var secretsSetCmd = &cobra.Command{
	Use:   "set <NAME> <VALUE>",
	Short: "Set a secret (creates or updates)",
	Long: `Set an encrypted secret. If the secret already exists, it is updated.

Pass "-" as the value to read from stdin (useful for piping).

Examples:
  ayb secrets set DB_PASSWORD s3cret
  echo "my-key" | ayb secrets set API_KEY -`,
	Args: cobra.ExactArgs(2),
	RunE: runSecretsSet,
}

var secretsGetCmd = &cobra.Command{
	Use:   "get <NAME>",
	Short: "Get a secret",
	Long: `Retrieve a secret by name. By default the value is masked.
Use --reveal to show the actual value.

Examples:
  ayb secrets get DB_PASSWORD
  ayb secrets get DB_PASSWORD --reveal`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretsGet,
}

var secretsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all secrets",
	Long: `List all secrets with name and timestamps.
Values are never shown in the list.

Examples:
  ayb secrets list
  ayb secrets list --json
  ayb secrets list --output csv`,
	RunE: runSecretsList,
}

var secretsDeleteCmd = &cobra.Command{
	Use:   "delete <NAME>",
	Short: "Delete a secret",
	Long: `Delete a secret by name. Prompts for confirmation unless --yes is passed.

Examples:
  ayb secrets delete DB_PASSWORD
  ayb secrets delete DB_PASSWORD --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretsDelete,
}

func init() {
	secretsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	secretsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	secretsGetCmd.Flags().Bool("reveal", false, "Show the actual secret value")
	secretsDeleteCmd.Flags().Bool("yes", false, "Skip confirmation prompt")

	secretsCmd.AddCommand(secretsSetCmd)
	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsListCmd)
	secretsCmd.AddCommand(secretsDeleteCmd)
}

// Sets or updates an encrypted secret by name. If the value argument is "-", reads from standard input instead. Returns an error if the API request fails.
func runSecretsSet(cmd *cobra.Command, args []string) error {
	name := args[0]
	value := args[1]

	if value == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		value = strings.TrimRight(string(data), "\r\n")
	}

	payload, _ := json.Marshal(map[string]string{"name": name, "value": value})

	resp, body, err := adminRequest(cmd, "POST", "/api/admin/secrets", bytes.NewReader(payload))
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusCreated {
		fmt.Printf("Secret %q created.\n", name)
		return nil
	}

	if resp.StatusCode == http.StatusConflict {
		// Secret exists — fall back to PUT for upsert
		payload, _ := json.Marshal(map[string]string{"value": value})
		resp, body, err = adminRequest(cmd, "PUT", "/api/admin/secrets/"+name, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Secret %q updated.\n", name)
			return nil
		}
		return serverError(resp.StatusCode, body)
	}

	return serverError(resp.StatusCode, body)
}

// Retrieves a secret by name and prints it in name=value format. The value is masked as "****" unless the reveal flag is set, in which case the actual value is displayed. Returns an error if the API request fails.
func runSecretsGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	reveal, _ := cmd.Flags().GetBool("reveal")

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/secrets/"+name, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if reveal {
		fmt.Printf("%s=%s\n", result.Name, result.Value)
	} else {
		fmt.Printf("%s=****\n", result.Name)
	}
	return nil
}

// Lists all configured secrets with creation and update timestamps. Supports json, csv, or table output format as determined by command flags. Secret values are never displayed. Returns an error if the API request fails.
func runSecretsList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/secrets", nil)
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

	var secrets []struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &secrets); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets configured.")
		return nil
	}

	cols := []string{"Name", "Created", "Updated"}
	rows := make([][]string, len(secrets))
	for i, s := range secrets {
		rows[i] = []string{s.Name, s.CreatedAt, s.UpdatedAt}
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
	fmt.Printf("\n%d secret(s)\n", len(secrets))
	return nil
}

// Deletes a secret by name, prompting for confirmation unless the yes flag is set. Returns an error if the API request fails or if the user declines confirmation.
func runSecretsDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	yes, _ := cmd.Flags().GetBool("yes")

	if !yes {
		confirmed, err := confirmSecretDeletion(cmd, name)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/secrets/"+name, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("Secret %q deleted.\n", name)
		return nil
	}
	return serverError(resp.StatusCode, body)
}

// Prompts the user to confirm deletion of the secret. Reads a response from standard input and accepts "y" or "yes" (case-insensitive). Returns true if confirmed, false if declined, and an error if input cannot be read or no response is provided.
func confirmSecretDeletion(cmd *cobra.Command, name string) (bool, error) {
	fmt.Fprintf(cmd.ErrOrStderr(), "Delete secret %q? [y/N] ", name)
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading confirmation: %w", err)
		}
		return false, fmt.Errorf("deletion requires confirmation; use --yes to proceed")
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Deletion cancelled.")
		return false, nil
	}
	return true, nil
}
