package cli

import (
	"time"

	"github.com/spf13/cobra"
)

var functionsCmd = &cobra.Command{
	Use:   "functions",
	Short: "Manage edge functions on the running AYB server",
	Long: `Manage edge functions deployed on the AYB server.

Subcommands let you list, inspect, scaffold, deploy, invoke, delete,
and view logs for edge functions. All commands require --admin-token
(or AYB_ADMIN_TOKEN env var) and --url pointing at the server.`,
}

var functionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployed edge functions",
	Long: `List all deployed edge functions with name, visibility, timeout,
last invocation time, and creation time.`,
	Example: `  ayb functions list
  ayb functions list --page 2 --per-page 10
  ayb functions list --json`,
	RunE: runFunctionsList,
}

var functionsGetCmd = &cobra.Command{
	Use:   "get <name-or-id>",
	Short: "Get edge function details by name or ID",
	Long: `Display detailed information about an edge function including source code,
environment variable keys (values masked), timeout, trigger count, and timestamps.`,
	Example: `  ayb functions get my-function
  ayb functions get aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa
  ayb functions get my-function --json`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsGet,
}

var functionsNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a local edge function source file",
	Long: `Create a new edge function source file in the current directory with
handler boilerplate. Generates JavaScript by default; use --typescript for TypeScript.`,
	Example: `  ayb functions new hello-world
  ayb functions new hello-world --typescript`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsNew,
}

var functionsDeployCmd = &cobra.Command{
	Use:   "deploy <name>",
	Short: "Deploy an edge function from a local source file",
	Long: `Deploy (create or update) an edge function. If a function with the given name
already exists, it is updated; otherwise a new function is created.`,
	Example: `  ayb functions deploy my-func --source ./my-func.js
  ayb functions deploy my-func --source ./my-func.ts --public --timeout 10000
  ayb functions deploy my-func --source ./my-func.js --entry-point handleRequest`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsDeploy,
}

var functionsDeleteCmd = &cobra.Command{
	Use:   "delete <name-or-id>",
	Short: "Delete an edge function",
	Long:  `Delete an edge function by name or ID. Prompts for confirmation unless --force is provided.`,
	Example: `  ayb functions delete my-func --force
  ayb functions delete aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa --force`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsDelete,
}

var functionsInvokeCmd = &cobra.Command{
	Use:   "invoke <name-or-id>",
	Short: "Invoke an edge function and print the response",
	Long: `Invoke an edge function via the admin API and display the response
including status code, headers, and body.`,
	Example: `  ayb functions invoke my-func
  ayb functions invoke my-func --method POST --body '{"key":"value"}'
  ayb functions invoke my-func --method PUT --body-file payload.json
  ayb functions invoke my-func --header "Authorization:Bearer token" --header "X-Custom:val"
  ayb functions invoke my-func --json`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsInvoke,
}

var functionsLogsCmd = &cobra.Command{
	Use:   "logs <name-or-id>",
	Short: "List execution logs for an edge function",
	Long: `List recent execution logs for an edge function. Supports filtering
by status and trigger type, and limiting the number of results.`,
	Example: `  ayb functions logs my-func
  ayb functions logs my-func --status error --limit 10
  ayb functions logs my-func --follow
  ayb functions logs my-func --trigger-type cron
  ayb functions logs my-func --json`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsLogs,
}

var (
	functionsLogsFollowPollInterval = 2 * time.Second
	functionsLogsFollowMaxPolls     = 0

	validInvokeMethods = map[string]struct{}{
		"GET":    {},
		"POST":   {},
		"PUT":    {},
		"DELETE": {},
		"PATCH":  {},
	}

	validLogStatuses = map[string]struct{}{
		"success": {},
		"error":   {},
	}

	validLogTriggerTypes = map[string]struct{}{
		"http":     {},
		"db":       {},
		"cron":     {},
		"storage":  {},
		"function": {},
	}
)

func init() {
	functionsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	functionsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	functionsListCmd.Flags().Int("page", 1, "Page number (1-based)")
	functionsListCmd.Flags().Int("per-page", 50, "Number of functions per page")
	functionsNewCmd.Flags().Bool("typescript", false, "Generate a TypeScript scaffold (.ts)")

	functionsDeployCmd.Flags().String("source", "", "Path to the source file to deploy (required)")
	functionsDeployCmd.Flags().String("entry-point", "", "Entry point function name")
	functionsDeployCmd.Flags().Int("timeout", 0, "Execution timeout in milliseconds")
	functionsDeployCmd.Flags().Bool("public", false, "Make the function publicly accessible")
	functionsDeployCmd.Flags().Bool("private", false, "Make the function private (default)")

	functionsDeleteCmd.Flags().Bool("force", false, "Skip confirmation prompt")

	functionsInvokeCmd.Flags().String("method", "GET", "HTTP method (GET, POST, PUT, DELETE, PATCH)")
	functionsInvokeCmd.Flags().String("path", "", "Request path (default /<function-name>)")
	functionsInvokeCmd.Flags().StringSlice("header", nil, "Request header as key:value (repeatable)")
	functionsInvokeCmd.Flags().String("body", "", "Request body string")
	functionsInvokeCmd.Flags().String("body-file", "", "Path to file containing request body")

	functionsLogsCmd.Flags().String("status", "", "Filter by status (success or error)")
	functionsLogsCmd.Flags().String("trigger-type", "", "Filter by trigger type (http, db, cron, storage, function)")
	functionsLogsCmd.Flags().Int("limit", 50, "Maximum number of log entries to return")
	functionsLogsCmd.Flags().Bool("follow", false, "Follow logs in real-time (polling)")

	functionsCmd.AddCommand(functionsListCmd)
	functionsCmd.AddCommand(functionsGetCmd)
	functionsCmd.AddCommand(functionsNewCmd)
	functionsCmd.AddCommand(functionsDeployCmd)
	functionsCmd.AddCommand(functionsDeleteCmd)
	functionsCmd.AddCommand(functionsInvokeCmd)
	functionsCmd.AddCommand(functionsLogsCmd)
}
