package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show AYB server status",
	Long:  `Show the running state of the Allyourbase server.`,
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().Int("port", 0, "Server port to check (default: read from PID file or 8090)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	portFlag, _ := cmd.Flags().GetInt("port")
	out := cmd.OutOrStdout()

	pid, port, err := readAYBPID()
	if err != nil {
		if os.IsNotExist(err) {
			if jsonOut {
				return json.NewEncoder(out).Encode(map[string]any{"status": "stopped"})
			}
			fmt.Fprintln(out, "AYB server is not running.")
			return nil
		}
		return fmt.Errorf("reading PID file: %w", err)
	}

	// Check if process is alive.
	proc, err := os.FindProcess(pid)
	if err != nil {
		cleanupServerFiles()
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{"status": "stopped"})
		}
		fmt.Fprintln(out, "AYB server is not running (stale PID file cleaned up).")
		return nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		cleanupServerFiles()
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{"status": "stopped"})
		}
		fmt.Fprintln(out, "AYB server is not running (stale PID file cleaned up).")
		return nil
	}

	// Use port flag if provided, otherwise use PID file port, fallback to 8090.
	if portFlag != 0 {
		port = portFlag
	}
	if port == 0 {
		port = 8090
	}

	// Probe health endpoint.
	healthy := false
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthURL)
	if err == nil {
		healthy = resp.StatusCode == http.StatusOK
		resp.Body.Close()
	}

	if jsonOut {
		return json.NewEncoder(out).Encode(map[string]any{
			"status":  "running",
			"pid":     pid,
			"port":    port,
			"healthy": healthy,
		})
	}

	useColor := colorEnabledFd(os.Stdout.Fd())
	fmt.Fprintf(out, "%s AYB server is running.\n", ui.BrandEmoji)
	fmt.Fprintf(out, "  PID:     %d\n", pid)
	fmt.Fprintf(out, "  Port:    %d\n", port)
	if healthy {
		if useColor {
			fmt.Fprintf(out, "  Health:  %s %s\n", ui.StyleSuccess.Render(ui.SymbolDot), "healthy")
		} else {
			fmt.Fprintf(out, "  Health:  %s healthy\n", ui.SymbolCheck)
		}
	} else {
		if useColor {
			fmt.Fprintf(out, "  Health:  %s %s\n", ui.StyleError.Render(ui.SymbolDot), "unreachable")
		} else {
			fmt.Fprintf(out, "  Health:  %s unreachable\n", ui.SymbolCross)
		}
	}
	return nil
}
