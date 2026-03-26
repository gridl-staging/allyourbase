package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Handles the functions deploy CLI command, creating or updating an edge function from a local source file with optional entry point, timeout, and visibility settings.
func runFunctionsDeploy(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("function name is required")
	}

	sourceFile, _ := cmd.Flags().GetString("source")
	if sourceFile == "" {
		return fmt.Errorf("--source flag is required")
	}

	sourceBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}
	source := string(sourceBytes)

	entryPoint, _ := cmd.Flags().GetString("entry-point")
	timeoutMs, _ := cmd.Flags().GetInt("timeout")
	if err := validateDeployVisibilityFlags(cmd); err != nil {
		return err
	}

	outFmt := outputFormat(cmd)

	// Check if function already exists by name.
	lookupPath := "/api/admin/functions/by-name/" + url.PathEscape(name)
	lookupResp, lookupBody, err := adminRequest(cmd, "GET", lookupPath, nil)
	if err != nil {
		return err
	}

	var action string
	var resp *http.Response
	var body []byte

	if lookupResp.StatusCode == http.StatusOK {
		// Function exists → update via PUT.
		var existing struct {
			ID     string `json:"id"`
			Public bool   `json:"public"`
		}
		if err := json.Unmarshal(lookupBody, &existing); err != nil {
			return fmt.Errorf("parsing existing function: %w", err)
		}
		if existing.ID == "" {
			return fmt.Errorf("function lookup returned empty ID")
		}

		isPublic, err := resolveDeployPublicValue(cmd, existing.Public)
		if err != nil {
			return err
		}

		payload := map[string]any{
			"source":      source,
			"entry_point": entryPoint,
			"timeout_ms":  timeoutMs,
			"public":      isPublic,
		}
		payloadBytes, _ := json.Marshal(payload)

		updatePath := "/api/admin/functions/" + existing.ID
		resp, body, err = adminRequest(cmd, "PUT", updatePath, bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		action = "Updated"
	} else if lookupResp.StatusCode == http.StatusNotFound {
		// Function does not exist → create via POST.
		isPublic, err := resolveDeployPublicValue(cmd, false)
		if err != nil {
			return err
		}

		payload := map[string]any{
			"name":        name,
			"source":      source,
			"entry_point": entryPoint,
			"timeout_ms":  timeoutMs,
			"public":      isPublic,
		}
		payloadBytes, _ := json.Marshal(payload)

		resp, body, err = adminRequest(cmd, "POST", "/api/admin/functions", bytes.NewReader(payloadBytes))
		if err != nil {
			return err
		}
		action = "Created"
	} else {
		return serverError(lookupResp.StatusCode, lookupBody)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, body)
	}

	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("%s function %q (ID: %s)\n", action, result.Name, result.ID)
	return nil
}

// Determines the public or private visibility for a function deployment by checking the --public and --private flags, defaulting to the provided value if neither flag is set.
func resolveDeployPublicValue(cmd *cobra.Command, defaultValue bool) (bool, error) {
	publicChanged := cmd.Flags().Changed("public")
	privateChanged := cmd.Flags().Changed("private")

	if publicChanged {
		return cmd.Flags().GetBool("public")
	}

	if privateChanged {
		privateValue, err := cmd.Flags().GetBool("private")
		if err != nil {
			return false, err
		}
		if privateValue {
			return false, nil
		}
	}

	return defaultValue, nil
}

func validateDeployVisibilityFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("public") && cmd.Flags().Changed("private") {
		return fmt.Errorf("cannot use --public and --private together")
	}
	return nil
}
