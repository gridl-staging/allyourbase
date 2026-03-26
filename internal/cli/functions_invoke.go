package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Handles the functions invoke CLI command, invoking an edge function via the admin API and displaying the response status, headers, and body.
func runFunctionsInvoke(cmd *cobra.Command, args []string) error {
	nameOrID := strings.TrimSpace(args[0])
	if nameOrID == "" {
		return fmt.Errorf("function name or ID is required")
	}

	method, _ := cmd.Flags().GetString("method")
	reqPath, _ := cmd.Flags().GetString("path")
	headers, _ := cmd.Flags().GetStringSlice("header")
	bodyStr, _ := cmd.Flags().GetString("body")
	bodyFile, _ := cmd.Flags().GetString("body-file")

	if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
		return fmt.Errorf("cannot use --body and --body-file together")
	}

	method, err := normalizeInvokeMethod(method)
	if err != nil {
		return err
	}

	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return fmt.Errorf("reading body file: %w", err)
		}
		bodyStr = string(data)
	}

	// Parse headers into map[string][]string.
	headerMap := make(map[string][]string)
	headerFlag := cmd.Flags().Lookup("header")
	headerExplicit := headerFlag != nil && headerFlag.Changed
	for _, h := range headers {
		if strings.TrimSpace(h) == "" {
			continue
		}
		idx := strings.Index(h, ":")
		if idx < 0 {
			if !headerExplicit {
				continue
			}
			return fmt.Errorf("invalid header format %q (expected key:value)", h)
		}
		key := strings.TrimSpace(h[:idx])
		value := strings.TrimSpace(h[idx+1:])
		if key == "" {
			if !headerExplicit {
				continue
			}
			if value == "" {
				continue
			}
			return fmt.Errorf("header name must not be empty")
		}
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("header %q contains invalid control characters", key)
		}
		headerMap[key] = append(headerMap[key], value)
	}

	functionID, err := resolveFunctionID(cmd, nameOrID)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"method":  method,
		"path":    reqPath,
		"headers": headerMap,
		"body":    bodyStr,
	}
	payloadBytes, _ := json.Marshal(payload)

	invokePath := "/api/admin/functions/" + functionID + "/invoke"
	resp, body, err := adminRequest(cmd, "POST", invokePath, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	outFmt := outputFormat(cmd)
	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		StatusCode int                 `json:"statusCode"`
		Headers    map[string][]string `json:"headers,omitempty"`
		Body       string              `json:"body,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("Status: %d\n", result.StatusCode)
	if len(result.Headers) > 0 {
		fmt.Println("Headers:")
		keys := make([]string, 0, len(result.Headers))
		for k := range result.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, v := range result.Headers[k] {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}
	}
	if result.Body != "" {
		fmt.Println("Body:")
		fmt.Println(result.Body)
	}
	return nil
}

func normalizeInvokeMethod(method string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	if _, ok := validInvokeMethods[normalized]; !ok {
		return "", fmt.Errorf("--method must be one of: GET, POST, PUT, DELETE, PATCH")
	}
	return normalized, nil
}
