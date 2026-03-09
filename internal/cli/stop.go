package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the AYB server",
	Long:  `Stop a running Allyourbase server gracefully.`,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	out := cmd.OutOrStdout()

	pid, _, err := readAYBPID()
	if err != nil {
		if os.IsNotExist(err) {
			// No PID file — check if something is actually listening on the
			// default port. This catches orphan processes (e.g. foreground
			// mode killed ungracefully, leaving embedded postgres alive).
			if portInUse(8090) {
				if jsonOut {
					return json.NewEncoder(out).Encode(map[string]any{
						"status":  "orphan",
						"message": "no PID file but port 8090 is in use",
						"port":    8090,
					})
				}
				fmt.Fprintln(out, "No PID file found, but port 8090 is in use.")
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "  An orphan process may be holding the port. Try:")
				fmt.Fprintln(out, "    lsof -ti :8090 | xargs kill   # find and kill the process")
				fmt.Fprintln(out, "    ayb start                     # then start fresh")
				return nil
			}
			if jsonOut {
				return json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "no AYB server is running"})
			}
			fmt.Fprintln(out, "No AYB server is running (no PID file found).")
			return nil
		}
		return fmt.Errorf("reading PID file: %w", err)
	}

	// Check if process is alive.
	proc, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist — clean up stale files.
		cleanupServerFiles()
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "stale PID file cleaned up"})
		}
		fmt.Fprintln(out, "No AYB server is running (stale PID file cleaned up).")
		return nil
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		cleanupServerFiles()
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "stale PID file cleaned up"})
		}
		fmt.Fprintln(out, "No AYB server is running (stale PID file cleaned up).")
		return nil
	}

	// Send SIGTERM for graceful shutdown.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to PID %d: %w", pid, err)
	}

	// Show spinner while waiting for shutdown.
	isTTY := colorEnabled()
	sp := ui.NewStepSpinner(os.Stderr, !isTTY)
	sp.Start("Stopping server...")

	// Wait for process to exit (up to 10 seconds).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			cleanupServerFiles()
			sp.Done()
			if jsonOut {
				return json.NewEncoder(out).Encode(map[string]any{"status": "stopped", "pid": pid})
			}
			fmt.Fprintf(out, "AYB server (PID %d) stopped.\n", pid)
			return nil
		}
	}

	// Graceful shutdown timed out — escalate to SIGKILL.
	sp.Fail()
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		// Process may have just died.
		cleanupServerFiles()
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{"status": "stopped", "pid": pid})
		}
		fmt.Fprintf(out, "AYB server (PID %d) stopped.\n", pid)
		return nil
	}
	time.Sleep(1 * time.Second)
	cleanupServerFiles()
	if jsonOut {
		return json.NewEncoder(out).Encode(map[string]any{
			"status": "killed", "pid": pid,
		})
	}
	fmt.Fprintf(out, "AYB server (PID %d) force-stopped (SIGKILL).\n", pid)
	return nil
}
