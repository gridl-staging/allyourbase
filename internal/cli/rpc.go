package cli

import (
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

var rpcCmd = &cobra.Command{
	Use:   "rpc <function> [--arg key=value ...]",
	Short: "Call a PostgreSQL function via the running AYB server",
	Long: `Call a PostgreSQL function via the RPC endpoint on the running AYB server.

Pass arguments with --arg flags (key=value pairs). Values are sent as strings
and the server handles type coercion.

Examples:
  ayb rpc increment_counter --arg count=5
  ayb rpc get_user_stats
  ayb rpc search_products --arg query=laptop --arg limit=10 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runRPC,
}

func init() {
	rpcCmd.Flags().StringArray("arg", nil, "Function argument as key=value (repeatable)")
	rpcCmd.Flags().String("admin-token", "", "Admin/JWT token (or set AYB_ADMIN_TOKEN)")
	rpcCmd.Flags().String("url", "", "Server URL (default http://127.0.0.1:8090)")
}

func parseRPCArgs(rawArgs []string) (map[string]any, error) {
	args := make(map[string]any, len(rawArgs))
	for _, a := range rawArgs {
		idx := strings.IndexByte(a, '=')
		if idx < 1 {
			return nil, fmt.Errorf("invalid argument %q: expected key=value", a)
		}
		key := a[:idx]
		val := a[idx+1:]

		// Try parsing as JSON value (numbers, booleans, null, objects, arrays).
		var parsed any
		if err := json.Unmarshal([]byte(val), &parsed); err == nil {
			args[key] = parsed
		} else {
			args[key] = val
		}
	}
	return args, nil
}

func runRPC(cmd *cobra.Command, args []string) error {
	funcName := args[0]
	outFmt := outputFormat(cmd)
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")
	rawArgs, _ := cmd.Flags().GetStringArray("arg")

	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	funcArgs, err := parseRPCArgs(rawArgs)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(funcArgs)
	req, err := http.NewRequest("POST", baseURL+"/api/rpc/"+funcName, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
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

	// Void function.
	if resp.StatusCode == http.StatusNoContent {
		if outFmt == "json" {
			fmt.Println(`{"status":"ok","result":null}`)
		} else {
			fmt.Println("(void) OK")
		}
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, respBody)
	}

	// JSON output mode: raw passthrough.
	if outFmt == "json" {
		os.Stdout.Write(respBody)
		fmt.Println()
		return nil
	}

	// Try to detect result shape.
	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	return formatRPCResult(result)
}

func formatRPCResult(result any) error {
	switch v := result.(type) {
	case []any:
		// Set-returning function: display as table.
		if len(v) == 0 {
			fmt.Println("(empty result set)")
			return nil
		}
		// Check if items are objects.
		first, ok := v[0].(map[string]any)
		if !ok {
			// Array of scalars.
			for _, item := range v {
				fmt.Println(formatScalar(item))
			}
			return nil
		}
		// Determine columns from first row.
		cols := make([]string, 0, len(first))
		for k := range first {
			cols = append(cols, k)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, strings.Join(cols, "\t"))
		fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
		for _, item := range v {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			vals := make([]string, len(cols))
			for i, col := range cols {
				vals[i] = formatScalar(row[col])
			}
			fmt.Fprintln(w, strings.Join(vals, "\t"))
		}
		w.Flush()
		fmt.Printf("\n%d row(s)\n", len(v))

	case map[string]any:
		// Object result: display as key-value pairs.
		for k, val := range v {
			fmt.Printf("%s: %s\n", k, formatScalar(val))
		}

	default:
		// Scalar result.
		fmt.Println(formatScalar(v))
	}
	return nil
}

func formatScalar(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
