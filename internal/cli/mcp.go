package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	aybmcp "github.com/allyourbase/ayb/internal/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP (Model Context Protocol) server",
	Long: `Start a Model Context Protocol server that exposes AYB's API as
tools, resources, and prompts for AI coding assistants.

The MCP server connects to a running AYB instance and provides structured
access to your database for tools like Claude Code, Cursor, and Windsurf.

Stdio mode (for Claude Desktop / Claude Code):
  ayb mcp

With explicit server URL:
  ayb mcp --url http://localhost:8090

With admin token for SQL access:
  ayb mcp --admin-token YOUR_TOKEN

Configuration in Claude Desktop (claude_desktop_config.json):
  {
    "mcpServers": {
      "ayb": {
        "command": "ayb",
        "args": ["mcp", "--admin-token", "YOUR_TOKEN"]
      }
    }
  }`,
	RunE: runMCP,
}

func init() {
	mcpCmd.Flags().String("url", "", "AYB server URL (default: auto-detect or http://127.0.0.1:8090)")
	mcpCmd.Flags().String("admin-token", "", "Admin token for privileged operations (or set AYB_ADMIN_TOKEN)")
	mcpCmd.Flags().String("token", "", "User JWT for RLS-filtered access (or set AYB_TOKEN)")
}

func runMCP(cmd *cobra.Command, args []string) error {
	baseURL, _ := cmd.Flags().GetString("url")
	adminToken, _ := cmd.Flags().GetString("admin-token")
	userToken, _ := cmd.Flags().GetString("token")

	if baseURL == "" {
		baseURL = serverURL()
	}
	if adminToken == "" {
		adminToken = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if userToken == "" {
		userToken = os.Getenv("AYB_TOKEN")
	}

	srv := aybmcp.NewServer(aybmcp.Config{
		BaseURL:    baseURL,
		AdminToken: adminToken,
		UserToken:  userToken,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}
	return nil
}
