package cli

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// Handles the functions delete CLI command, deleting a function by name or ID with optional user confirmation via --force flag.
func runFunctionsDelete(cmd *cobra.Command, args []string) error {
	nameOrID := strings.TrimSpace(args[0])
	if nameOrID == "" {
		return fmt.Errorf("function name or ID is required")
	}

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		confirmed, err := confirmFunctionDeletion(cmd, nameOrID)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	// Resolve to ID.
	functionID, err := resolveFunctionID(cmd, nameOrID)
	if err != nil {
		return err
	}

	deletePath := "/api/admin/functions/" + functionID
	resp, body, err := adminRequest(cmd, "DELETE", deletePath, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	fmt.Printf("Deleted function %q\n", nameOrID)
	return nil
}

// Prompts the user to confirm deletion of a function by writing to stderr and reading from stdin, returning true if confirmed with y or yes.
func confirmFunctionDeletion(cmd *cobra.Command, nameOrID string) (bool, error) {
	fmt.Fprintf(cmd.ErrOrStderr(), "Delete edge function %q? [y/N] ", nameOrID)
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading confirmation: %w", err)
		}
		return false, fmt.Errorf("deletion requires confirmation; use --force to proceed")
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Deletion cancelled.")
		return false, nil
	}

	return true, nil
}
