package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// Handles the functions list CLI command, listing deployed functions with pagination support, showing name, visibility, timeout, last invocation time, and creation time.
func runFunctionsList(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	page, _ := cmd.Flags().GetInt("page")
	perPage, _ := cmd.Flags().GetInt("per-page")

	if page <= 0 {
		return fmt.Errorf("--page must be greater than 0")
	}
	if perPage <= 0 {
		return fmt.Errorf("--per-page must be greater than 0")
	}

	path := fmt.Sprintf("/api/admin/functions?page=%d&perPage=%d", page, perPage)
	resp, body, err := adminRequest(cmd, "GET", path, nil)
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

	var functions []struct {
		Name          string     `json:"name"`
		Public        bool       `json:"public"`
		Timeout       int64      `json:"timeout"`
		LastInvokedAt *time.Time `json:"lastInvokedAt,omitempty"`
		CreatedAt     time.Time  `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &functions); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(functions) == 0 {
		fmt.Println("No edge functions deployed.")
		return nil
	}

	cols := []string{"Name", "Visibility", "Timeout", "Last Invoked", "Created"}
	rows := make([][]string, len(functions))
	for i, fn := range functions {
		visibility := "private"
		if fn.Public {
			visibility = "public"
		}

		timeout := "-"
		if fn.Timeout > 0 {
			timeout = time.Duration(fn.Timeout).String()
		}

		lastInvoked := "-"
		if fn.LastInvokedAt != nil {
			lastInvoked = fn.LastInvokedAt.UTC().Format(time.RFC3339)
		}

		created := "-"
		if !fn.CreatedAt.IsZero() {
			created = fn.CreatedAt.UTC().Format(time.RFC3339)
		}

		rows[i] = []string{fn.Name, visibility, timeout, lastInvoked, created}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}
