package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// resolveFunctionID resolves a name-or-id argument to a function UUID string.
// If the argument is a valid UUID, it's returned directly.
// Otherwise, it looks up the function by name via the admin API.
func resolveFunctionID(cmd *cobra.Command, nameOrID string) (string, error) {
	if _, err := uuid.Parse(nameOrID); err == nil {
		return nameOrID, nil
	}

	lookupPath := "/api/admin/functions/by-name/" + url.PathEscape(nameOrID)
	resp, body, err := adminRequest(cmd, "GET", lookupPath, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", serverError(resp.StatusCode, body)
	}

	var fn struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &fn); err != nil {
		return "", fmt.Errorf("parsing function lookup: %w", err)
	}
	if fn.ID == "" {
		return "", fmt.Errorf("function lookup returned empty ID")
	}
	return fn.ID, nil
}

func validateScaffoldFunctionName(name string) error {
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("function name %q must not contain path separators", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("function name %q is invalid", name)
	}
	return nil
}

func functionsJSTemplate() string {
	return `export default function handler(req) {
  return {
    statusCode: 200,
    body: JSON.stringify({ message: "Hello from edge function!" }),
    headers: { "Content-Type": "application/json" },
  };
}
`
}

// Returns a TypeScript edge function handler template with typed EdgeRequest parameter and HTTP response structure.
func functionsTSTemplate() string {
	return `type EdgeRequest = {
  method: string;
  path: string;
  headers?: Record<string, string>;
  body?: string;
};

export default function handler(req: EdgeRequest) {
  return {
    statusCode: 200,
    body: JSON.stringify({ message: "Hello from edge function!", path: req.path }),
    headers: { "Content-Type": "application/json" },
  };
}
`
}

// Handles the functions new CLI command, scaffolding a new function source file in the current directory with handler boilerplate in JavaScript or TypeScript.
func runFunctionsNew(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("function name is required")
	}
	if err := validateScaffoldFunctionName(name); err != nil {
		return err
	}

	useTypescript, _ := cmd.Flags().GetBool("typescript")

	extension := ".js"
	content := functionsJSTemplate()
	if useTypescript {
		extension = ".ts"
		content = functionsTSTemplate()
	}

	path := name + extension
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file %q already exists", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking target file: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing scaffold file: %w", err)
	}

	fmt.Printf("Created edge function scaffold: %s\n", path)
	return nil
}
