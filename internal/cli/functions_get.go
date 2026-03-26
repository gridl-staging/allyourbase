package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type functionsGetResponse struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	EntryPoint    string            `json:"entryPoint"`
	Timeout       int64             `json:"timeout"`
	LastInvokedAt *time.Time        `json:"lastInvokedAt,omitempty"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	Public        bool              `json:"public"`
	Source        string            `json:"source"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

type functionsGetOutput struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	EntryPoint    string            `json:"entryPoint"`
	Timeout       int64             `json:"timeout"`
	LastInvokedAt *time.Time        `json:"lastInvokedAt,omitempty"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	Public        bool              `json:"public"`
	Source        string            `json:"source"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	TriggerCount  int               `json:"triggerCount"`
}

// Handles the functions get CLI command, displaying detailed information about a function including ID, source code, masked environment variables, timeout, trigger count, and timestamps.
func runFunctionsGet(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	nameOrID := args[0]

	path := resolveFunctionsGetPath(nameOrID)
	resp, body, err := adminRequest(cmd, "GET", path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var fn functionsGetResponse
	if err := json.Unmarshal(body, &fn); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	if fn.ID == "" {
		return fmt.Errorf("response missing function id")
	}

	triggerCount, err := fetchFunctionTriggerCount(cmd, fn.ID)
	if err != nil {
		return err
	}

	output := functionsGetOutput{
		ID:            fn.ID,
		Name:          fn.Name,
		EntryPoint:    fn.EntryPoint,
		Timeout:       fn.Timeout,
		LastInvokedAt: fn.LastInvokedAt,
		EnvVars:       maskEnvVarValues(fn.EnvVars),
		Public:        fn.Public,
		Source:        fn.Source,
		CreatedAt:     fn.CreatedAt,
		UpdatedAt:     fn.UpdatedAt,
		TriggerCount:  triggerCount,
	}

	if outFmt == "json" {
		return json.NewEncoder(os.Stdout).Encode(output)
	}

	visibility := "private"
	if output.Public {
		visibility = "public"
	}

	timeout := "-"
	if output.Timeout > 0 {
		timeout = time.Duration(output.Timeout).String()
	}

	lastInvoked := "-"
	if output.LastInvokedAt != nil {
		lastInvoked = output.LastInvokedAt.UTC().Format(time.RFC3339)
	}

	created := "-"
	if !output.CreatedAt.IsZero() {
		created = output.CreatedAt.UTC().Format(time.RFC3339)
	}

	updated := "-"
	if !output.UpdatedAt.IsZero() {
		updated = output.UpdatedAt.UTC().Format(time.RFC3339)
	}

	fmt.Printf("ID: %s\n", output.ID)
	fmt.Printf("Name: %s\n", output.Name)
	fmt.Printf("Visibility: %s\n", visibility)
	fmt.Printf("Entry Point: %s\n", output.EntryPoint)
	fmt.Printf("Timeout: %s\n", timeout)
	fmt.Printf("Last Invoked: %s\n", lastInvoked)
	fmt.Printf("Created: %s\n", created)
	fmt.Printf("Updated: %s\n", updated)
	fmt.Printf("Trigger Count: %d\n", output.TriggerCount)
	if len(output.EnvVars) == 0 {
		fmt.Println("Env Vars: (none)")
	} else {
		fmt.Println("Env Vars:")
		keys := make([]string, 0, len(output.EnvVars))
		for k := range output.EnvVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s=%s\n", k, output.EnvVars[k])
		}
	}
	fmt.Println("Source:")
	fmt.Println(output.Source)
	return nil
}

func resolveFunctionsGetPath(nameOrID string) string {
	if _, err := uuid.Parse(nameOrID); err == nil {
		return "/api/admin/functions/" + nameOrID
	}
	return "/api/admin/functions/by-name/" + url.PathEscape(nameOrID)
}

// Fetches the total number of database, cron, and storage triggers for a function from the admin API and returns the sum.
func fetchFunctionTriggerCount(cmd *cobra.Command, functionID string) (int, error) {
	triggerTypes := []string{"db", "cron", "storage"}
	total := 0

	for _, triggerType := range triggerTypes {
		path := fmt.Sprintf(
			"/api/admin/functions/%s/triggers/%s",
			url.PathEscape(functionID),
			triggerType,
		)
		resp, body, err := adminRequest(cmd, "GET", path, nil)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode != http.StatusOK {
			return 0, serverError(resp.StatusCode, body)
		}

		var triggers []json.RawMessage
		if err := json.Unmarshal(body, &triggers); err != nil {
			return 0, fmt.Errorf("parsing %s trigger response: %w", triggerType, err)
		}
		total += len(triggers)
	}

	return total, nil
}

func maskEnvVarValues(envVars map[string]string) map[string]string {
	if len(envVars) == 0 {
		return nil
	}

	const maskedValue = "********"
	masked := make(map[string]string, len(envVars))
	for key := range envVars {
		masked[key] = maskedValue
	}
	return masked
}
