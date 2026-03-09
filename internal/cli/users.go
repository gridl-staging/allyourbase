package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users on the running AYB server",
}

var usersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered users",
	RunE:  runUsersList,
}

var usersDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUsersDelete,
}

func init() {
	usersCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	usersCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	usersListCmd.Flags().String("search", "", "Search by email")
	usersListCmd.Flags().Int("page", 1, "Page number")
	usersListCmd.Flags().Int("per-page", 20, "Items per page")

	usersCmd.AddCommand(usersListCmd)
	usersCmd.AddCommand(usersDeleteCmd)
}

func runUsersList(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	search, _ := cmd.Flags().GetString("search")
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")

	qs := url.Values{}
	qs.Set("page", fmt.Sprintf("%d", page))
	qs.Set("perPage", fmt.Sprintf("%d", perPage))
	if search != "" {
		qs.Set("search", search)
	}

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/users?"+qs.Encode(), nil)
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
			ID            string `json:"id"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"emailVerified"`
			CreatedAt     string `json:"createdAt"`
		} `json:"items"`
		Page       int `json:"page"`
		PerPage    int `json:"perPage"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No users found.")
		return nil
	}

	// Build string rows for table and CSV output.
	cols := []string{"ID", "Email", "Verified", "Created"}
	rows := make([][]string, len(result.Items))
	for i, u := range result.Items {
		verified := "no"
		if u.EmailVerified {
			verified = "yes"
		}
		rows[i] = []string{u.ID, u.Email, verified, u.CreatedAt}
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
	fmt.Printf("\nPage %d/%d (%d total users)\n", result.Page, result.TotalPages, result.TotalItems)
	return nil
}

func runUsersDelete(cmd *cobra.Command, args []string) error {
	id := args[0]

	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/users/"+id, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("User %s deleted.\n", id)
		return nil
	}
	return serverError(resp.StatusCode, body)
}
