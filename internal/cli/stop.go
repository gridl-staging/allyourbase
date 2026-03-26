// Package cli The file implements the stop command to gracefully shut down the AYB server, with automatic escalation to forced termination and cleanup of process files.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
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

var stopPortInUse = portInUse

func init() {
	stopCmd.Flags().Int("port", 0, "Server port to check for orphan-process detection (default: 8090)")
}

func reportStopWithoutPID(out io.Writer, jsonOut bool, orphanCheckPort int) error {
	// No PID file — check if something is actually listening on the configured
	// port. This catches orphan processes (e.g. foreground mode killed
	// ungracefully, leaving embedded postgres alive).
	if stopPortInUse(orphanCheckPort) {
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{
				"status":  "orphan",
				"message": fmt.Sprintf("no PID file but port %d is in use", orphanCheckPort),
				"port":    orphanCheckPort,
			})
		}
		fmt.Fprintf(out, "No PID file found, but port %d is in use.\n", orphanCheckPort)
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  An orphan process may be holding the port. Try:")
		fmt.Fprintf(out, "    lsof -ti :%d | xargs kill   # find and kill the process\n", orphanCheckPort)
		fmt.Fprintln(out, "    ayb start                     # then start fresh")
		return nil
	}
	if jsonOut {
		return json.NewEncoder(out).Encode(map[string]any{"status": "not_running", "message": "no AYB server is running"})
	}
	fmt.Fprintln(out, "No AYB server is running (no PID file found).")
	return nil
}

// runStop gracefully terminates the AYB server by sending SIGTERM to the process identified in the PID file. If graceful shutdown doesn't complete within 10 seconds, it escalates to SIGKILL. It handles orphan processes, stale PID files, cleans up server files, and returns JSON output when the json flag is set.
func runStop(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	portFlag, _ := cmd.Flags().GetInt("port")
	out := cmd.OutOrStdout()

	pid, _, err := readAYBPID()
	if err != nil {
		if os.IsNotExist(err) {
			orphanCheckPort := portFlag
			if orphanCheckPort == 0 {
				orphanCheckPort = 8090
			}
			return reportStopWithoutPID(out, jsonOut, orphanCheckPort)
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
