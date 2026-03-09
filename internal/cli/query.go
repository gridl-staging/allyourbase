package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <table>",
	Short: "Query records from a table on the running AYB server",
	Long: `Query records from a collection via the running AYB server's REST API.

Examples:
  ayb query posts
  ayb query users --filter "email LIKE '%@example.com'" --sort -created_at --limit 5
  ayb query posts --fields id,title,created_at --json`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

func init() {
	queryCmd.Flags().String("filter", "", "Filter expression (e.g. \"status='active' AND age>21\")")
	queryCmd.Flags().String("sort", "", "Sort fields (e.g. \"-created_at,+title\")")
	queryCmd.Flags().String("fields", "", "Comma-separated column list")
	queryCmd.Flags().String("expand", "", "Comma-separated FK relationships to expand")
	queryCmd.Flags().Int("page", 1, "Page number")
	queryCmd.Flags().Int("limit", 20, "Items per page (max 500)")
	queryCmd.Flags().String("admin-token", "", "Admin/JWT token (or set AYB_ADMIN_TOKEN)")
	queryCmd.Flags().String("url", "", "Server URL (default http://127.0.0.1:8090)")
}

func runQuery(cmd *cobra.Command, args []string) error {
	table := args[0]
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")
	filter, _ := cmd.Flags().GetString("filter")
	sort, _ := cmd.Flags().GetString("sort")
	fields, _ := cmd.Flags().GetString("fields")
	expand, _ := cmd.Flags().GetString("expand")
	page, _ := cmd.Flags().GetInt("page")
	limit, _ := cmd.Flags().GetInt("limit")

	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	qs := url.Values{}
	if filter != "" {
		qs.Set("filter", filter)
	}
	if sort != "" {
		qs.Set("sort", sort)
	}
	if fields != "" {
		qs.Set("fields", fields)
	}
	if expand != "" {
		qs.Set("expand", expand)
	}
	qs.Set("page", fmt.Sprintf("%d", page))
	qs.Set("perPage", fmt.Sprintf("%d", limit))

	reqURL := fmt.Sprintf("%s/api/collections/%s?%s", baseURL, table, qs.Encode())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
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
		return fmt.Errorf("reading response: %w", err)
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

	// Parse list response and display as table.
	var result struct {
		Items      []map[string]any `json:"items"`
		Page       int              `json:"page"`
		PerPage    int              `json:"perPage"`
		TotalItems int              `json:"totalItems"`
		TotalPages int              `json:"totalPages"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No records found.")
		return nil
	}

	// Determine columns from first row (or from --fields).
	var cols []string
	if fields != "" {
		cols = strings.Split(fields, ",")
	} else {
		// Use keys from first item, preserving a reasonable order.
		for k := range result.Items[0] {
			cols = append(cols, k)
		}
	}

	if outFmt == "csv" {
		rows := make([][]string, len(result.Items))
		for i, item := range result.Items {
			vals := make([]string, len(cols))
			for j, col := range cols {
				v := item[col]
				if v == nil {
					vals[j] = ""
				} else {
					vals[j] = fmt.Sprint(v)
				}
			}
			rows[i] = vals
		}
		return writeCSVStdout(cols, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, item := range result.Items {
		vals := make([]string, len(cols))
		for i, col := range cols {
			v := item[col]
			if v == nil {
				vals[i] = "NULL"
			} else {
				vals[i] = fmt.Sprint(v)
			}
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	w.Flush()
	fmt.Printf("\nPage %d/%d (%d total records)\n", result.Page, result.TotalPages, result.TotalItems)
	return nil
}
