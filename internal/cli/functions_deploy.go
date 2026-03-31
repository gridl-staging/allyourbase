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

type functionsDeployInput struct {
	name       string
	source     string
	entryPoint string
	timeoutMs  int
	outFmt     string
}

type existingFunctionsDeployTarget struct {
	ID     string `json:"id"`
	Public bool   `json:"public"`
}

// Handles the functions deploy CLI command, creating or updating an edge function from a local source file with optional entry point, timeout, and visibility settings.
func runFunctionsDeploy(cmd *cobra.Command, args []string) error {
	input, err := loadFunctionsDeployInput(cmd, args)
	if err != nil {
		return err
	}
	lookupResp, lookupBody, err := lookupFunctionsDeployTarget(cmd, input.name)
	if err != nil {
		return err
	}
	action, resp, body, err := executeFunctionsDeploy(cmd, input, lookupResp, lookupBody)
	if err != nil {
		return err
	}
	return renderFunctionsDeployResponse(input.outFmt, action, resp, body)
}

func loadFunctionsDeployInput(cmd *cobra.Command, args []string) (functionsDeployInput, error) {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return functionsDeployInput{}, fmt.Errorf("function name is required")
	}

	sourceFile, _ := cmd.Flags().GetString("source")
	if sourceFile == "" {
		return functionsDeployInput{}, fmt.Errorf("--source flag is required")
	}

	sourceBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return functionsDeployInput{}, fmt.Errorf("reading source file: %w", err)
	}
	if err := validateDeployVisibilityFlags(cmd); err != nil {
		return functionsDeployInput{}, err
	}

	entryPoint, _ := cmd.Flags().GetString("entry-point")
	timeoutMs, _ := cmd.Flags().GetInt("timeout")
	return functionsDeployInput{
		name:       name,
		source:     string(sourceBytes),
		entryPoint: entryPoint,
		timeoutMs:  timeoutMs,
		outFmt:     outputFormat(cmd),
	}, nil
}

func lookupFunctionsDeployTarget(cmd *cobra.Command, name string) (*http.Response, []byte, error) {
	lookupPath := "/api/admin/functions/by-name/" + url.PathEscape(name)
	return adminRequest(cmd, "GET", lookupPath, nil)
}

func executeFunctionsDeploy(cmd *cobra.Command, input functionsDeployInput, lookupResp *http.Response, lookupBody []byte) (string, *http.Response, []byte, error) {
	action := "Created"
	method := "POST"
	path := "/api/admin/functions"
	defaultPublic := false
	includeName := true

	switch lookupResp.StatusCode {
	case http.StatusOK:
		var existing existingFunctionsDeployTarget
		if err := json.Unmarshal(lookupBody, &existing); err != nil {
			return "", nil, nil, fmt.Errorf("parsing existing function: %w", err)
		}
		if existing.ID == "" {
			return "", nil, nil, fmt.Errorf("function lookup returned empty ID")
		}
		if existing.ID != url.PathEscape(existing.ID) {
			return "", nil, nil, fmt.Errorf("function lookup returned unsafe ID %q", existing.ID)
		}

		action = "Updated"
		method = "PUT"
		path = "/api/admin/functions/" + existing.ID
		defaultPublic = existing.Public
		includeName = false
	case http.StatusNotFound:
	default:
		return "", nil, nil, serverError(lookupResp.StatusCode, lookupBody)
	}

	isPublic, err := resolveDeployPublicValue(cmd, defaultPublic)
	if err != nil {
		return "", nil, nil, err
	}
	payloadBytes, err := buildFunctionsDeployPayload(input, isPublic, includeName)
	if err != nil {
		return "", nil, nil, err
	}

	resp, body, err := adminRequest(cmd, method, path, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", nil, nil, err
	}
	return action, resp, body, nil
}

func buildFunctionsDeployPayload(input functionsDeployInput, isPublic bool, includeName bool) ([]byte, error) {
	payload := map[string]any{
		"source":      input.source,
		"entry_point": input.entryPoint,
		"timeout_ms":  input.timeoutMs,
		"public":      isPublic,
	}
	if includeName {
		payload["name"] = input.name
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding deploy payload: %w", err)
	}
	return payloadBytes, nil
}

func renderFunctionsDeployResponse(outFmt, action string, resp *http.Response, body []byte) error {
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
