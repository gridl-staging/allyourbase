package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove AYB from your system",
	Long: `Remove Allyourbase from your system. Removes the binary, cached Postgres
binaries, and cleans up PATH entries from your shell profile.

Your database data (~/.ayb/data) is preserved by default. Use --purge to
remove everything including your embedded database.`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().Bool("purge", false, "Remove everything including embedded database data")
	uninstallCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
}

func runUninstall(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	purge, _ := cmd.Flags().GetBool("purge")
	yes, _ := cmd.Flags().GetBool("yes")

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("detecting home directory: %w", err)
	}

	aybDir := filepath.Join(home, ".ayb")
	binPath := filepath.Join(aybDir, "bin", "ayb")

	// Check if server is running.
	if isServerRunning() {
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"status":  "error",
				"message": "AYB server is running — stop it first with: ayb stop",
			})
		}
		return fmt.Errorf("AYB server is running — stop it first with: ayb stop")
	}

	// Check if there's anything to uninstall.
	if _, err := os.Stat(aybDir); os.IsNotExist(err) {
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"status":  "not_installed",
				"message": "nothing to uninstall",
			})
		}
		fmt.Println("Nothing to uninstall (~/.ayb does not exist).")
		return nil
	}

	// Confirm purge if requested.
	if purge && !yes {
		fmt.Println("This will delete your embedded database and all data in ~/.ayb.")
		fmt.Print("Continue? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	var removed []string

	// Remove binary.
	if _, err := os.Stat(binPath); err == nil {
		os.Remove(binPath)
		removed = append(removed, binPath)
	}

	// Remove bin dir if empty.
	binDir := filepath.Join(aybDir, "bin")
	if isEmpty(binDir) {
		os.Remove(binDir)
	}

	// Remove cached Postgres binaries (~/.ayb/pg).
	pgDir := filepath.Join(aybDir, "pg")
	if _, err := os.Stat(pgDir); err == nil {
		os.RemoveAll(pgDir)
		removed = append(removed, pgDir)
	}

	// Remove runtime dir (~/.ayb/run).
	runDir := filepath.Join(aybDir, "run")
	if _, err := os.Stat(runDir); err == nil {
		os.RemoveAll(runDir)
		removed = append(removed, runDir)
	}

	// Remove stale PID file.
	pidPath := filepath.Join(aybDir, "ayb.pid")
	if _, err := os.Stat(pidPath); err == nil {
		os.Remove(pidPath)
		removed = append(removed, pidPath)
	}

	// Purge: remove data directory and the entire ~/.ayb.
	dataPreserved := false
	if purge {
		dataDir := filepath.Join(aybDir, "data")
		if _, err := os.Stat(dataDir); err == nil {
			os.RemoveAll(dataDir)
			removed = append(removed, dataDir)
		}
		// Remove the entire ~/.ayb if it's now empty (or purge was requested).
		os.RemoveAll(aybDir)
		removed = append(removed, aybDir)
	} else {
		// Check if data dir exists to notify user.
		dataDir := filepath.Join(aybDir, "data")
		if _, err := os.Stat(dataDir); err == nil {
			dataPreserved = true
		}
		// Try to remove ~/.ayb if it's empty.
		if isEmpty(aybDir) {
			os.Remove(aybDir)
		}
	}

	// Clean up PATH entries from shell profiles.
	profilesCleaned := cleanShellProfiles(home, filepath.Join(aybDir, "bin"))

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"status":           "uninstalled",
			"removed":          removed,
			"profiles_cleaned": profilesCleaned,
			"data_preserved":   dataPreserved,
		})
	}

	fmt.Println("AYB uninstalled.")
	if len(removed) > 0 {
		fmt.Println("\nRemoved:")
		for _, r := range removed {
			fmt.Printf("  %s\n", r)
		}
	}
	if len(profilesCleaned) > 0 {
		fmt.Println("\nCleaned PATH from:")
		for _, p := range profilesCleaned {
			fmt.Printf("  %s\n", p)
		}
	}
	if dataPreserved {
		fmt.Printf("\nYour data directory was preserved: %s\n", filepath.Join(aybDir, "data"))
		fmt.Println("To remove it: rm -rf ~/.ayb")
	}

	return nil
}

// isServerRunning checks if an AYB server is currently running.
func isServerRunning() bool {
	pid, _, err := readAYBPID()
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if the process exists without actually signaling it.
	return proc.Signal(syscall.Signal(0)) == nil
}

// cleanShellProfiles removes AYB PATH entries from shell config files.
// Returns the list of files that were modified.
func cleanShellProfiles(home, binDir string) []string {
	profiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}

	var cleaned []string
	for _, profile := range profiles {
		if removeAYBLines(profile, binDir) {
			cleaned = append(cleaned, profile)
		}
	}
	return cleaned
}

// removeAYBLines removes the "# Allyourbase" comment and the PATH export line
// from the given file. Returns true if the file was modified.
func removeAYBLines(path, binDir string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	modified := false

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])

		// Skip "# Allyourbase" comment followed by a PATH line containing our bin dir.
		if trimmed == "# Allyourbase" && i+1 < len(lines) && strings.Contains(lines[i+1], binDir) {
			i++ // skip the export line too
			modified = true
			// Also skip a leading blank line if we left one.
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				i++
			}
			continue
		}

		// Also catch standalone PATH lines that reference our bin dir.
		if strings.Contains(trimmed, binDir) && (strings.HasPrefix(trimmed, "export PATH") || strings.HasPrefix(trimmed, "set -gx PATH")) {
			modified = true
			continue
		}

		out = append(out, lines[i])
	}

	if !modified {
		return false
	}

	// Remove trailing blank lines we may have created.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	out = append(out, "") // ensure trailing newline

	os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
	return true
}

// isEmpty checks if a directory exists and is empty.
func isEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) == 0
}
