package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// Handles the functions logs CLI command, listing or following (via polling) execution logs for a function with optional status and trigger type filters.
func runFunctionsLogs(cmd *cobra.Command, args []string) error {
	nameOrID := strings.TrimSpace(args[0])
	if nameOrID == "" {
		return fmt.Errorf("function name or ID is required")
	}

	status, _ := cmd.Flags().GetString("status")
	triggerType, _ := cmd.Flags().GetString("trigger-type")
	limit, _ := cmd.Flags().GetInt("limit")
	follow, _ := cmd.Flags().GetBool("follow")

	status = strings.ToLower(strings.TrimSpace(status))
	triggerType = strings.ToLower(strings.TrimSpace(triggerType))

	if err := validateFunctionsLogFilters(status, triggerType, limit); err != nil {
		return err
	}

	functionID, err := resolveFunctionID(cmd, nameOrID)
	if err != nil {
		return err
	}

	outFmt := outputFormat(cmd)
	if !follow {
		body, logs, err := fetchFunctionsLogs(cmd, functionID, status, triggerType, limit, nil)
		if err != nil {
			return err
		}
		if outFmt == "json" {
			os.Stdout.Write(body)
			fmt.Println()
			return nil
		}
		if len(logs) == 0 {
			fmt.Println("No logs found.")
			return nil
		}
		return printFunctionsLogsTable(logs)
	}

	// Follow mode: poll with a since cursor until the context is cancelled.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	seenIDs := map[string]struct{}{}
	var since *time.Time

	pollCount := 0
	for {
		pollCount++
		_, logs, err := fetchFunctionsLogs(cmd, functionID, status, triggerType, limit, since)
		if err != nil {
			return err
		}

		newLogs := filterUnseenFunctionLogs(logs, seenIDs)
		if len(newLogs) > 0 {
			if outFmt == "json" {
				if err := printFunctionsLogsJSONLines(newLogs); err != nil {
					return err
				}
			} else {
				if err := printFunctionsLogsTable(newLogs); err != nil {
					return err
				}
			}
			since = advanceFunctionLogsSinceCursor(since, newLogs)
		}

		if functionsLogsFollowMaxPolls > 0 && pollCount >= functionsLogsFollowMaxPolls {
			return nil
		}

		if err := sleepUntilNextFunctionsLogsPoll(ctx); err != nil {
			return nil
		}
	}
}

type functionsLogEntry struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	DurationMs    int    `json:"durationMs"`
	TriggerType   string `json:"triggerType"`
	RequestMethod string `json:"requestMethod"`
	RequestPath   string `json:"requestPath"`
	Error         string `json:"error"`
	CreatedAt     string `json:"createdAt"`
}

// Validates that log filter parameters (status, trigger type, and limit) are within allowed values, returning an error if invalid.
func validateFunctionsLogFilters(status, triggerType string, limit int) error {
	if limit <= 0 {
		return fmt.Errorf("--limit must be greater than 0")
	}
	if status != "" {
		if _, ok := validLogStatuses[status]; !ok {
			return fmt.Errorf("--status must be one of: success, error")
		}
	}
	if triggerType != "" {
		if _, ok := validLogTriggerTypes[triggerType]; !ok {
			return fmt.Errorf("--trigger-type must be one of: http, db, cron, storage, function")
		}
	}
	return nil
}

// Fetches execution logs for a function from the admin API, optionally filtered by status, trigger type, and since timestamp, returning both the raw response body and parsed log entries.
func fetchFunctionsLogs(cmd *cobra.Command, functionID, status, triggerType string, limit int, since *time.Time) ([]byte, []functionsLogEntry, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	if status != "" {
		q.Set("status", status)
	}
	if triggerType != "" {
		q.Set("trigger_type", triggerType)
	}
	if since != nil {
		q.Set("since", since.UTC().Format(time.RFC3339))
	}

	logsPath := "/api/admin/functions/" + functionID + "/logs"
	if encoded := q.Encode(); encoded != "" {
		logsPath += "?" + encoded
	}

	resp, body, err := adminRequest(cmd, "GET", logsPath, nil)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, serverError(resp.StatusCode, body)
	}

	var logs []functionsLogEntry
	if err := json.Unmarshal(body, &logs); err != nil {
		return nil, nil, fmt.Errorf("parsing response: %w", err)
	}
	return body, logs, nil
}

// Formats and outputs function execution logs as a tab-separated table with columns for status, trigger type, HTTP method, path, duration, error, and timestamp.
func printFunctionsLogsTable(logs []functionsLogEntry) error {
	cols := []string{"Status", "Trigger", "Method", "Path", "Duration", "Error", "Time"}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, log := range logs {
		errStr := "-"
		if log.Error != "" {
			errStr = truncate(log.Error, 40)
		}
		trigger := log.TriggerType
		if trigger == "" {
			trigger = "-"
		}
		method := log.RequestMethod
		if method == "" {
			method = "-"
		}
		path := log.RequestPath
		if path == "" {
			path = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%dms\t%s\t%s\n",
			log.Status, trigger, method, path, log.DurationMs, errStr, log.CreatedAt)
	}
	return w.Flush()
}

func printFunctionsLogsJSONLines(logs []functionsLogEntry) error {
	enc := json.NewEncoder(os.Stdout)
	for _, log := range logs {
		if err := enc.Encode(log); err != nil {
			return fmt.Errorf("writing follow output: %w", err)
		}
	}
	return nil
}

func filterUnseenFunctionLogs(logs []functionsLogEntry, seenIDs map[string]struct{}) []functionsLogEntry {
	newLogs := make([]functionsLogEntry, 0, len(logs))
	for _, log := range logs {
		if log.ID != "" {
			if _, seen := seenIDs[log.ID]; seen {
				continue
			}
			seenIDs[log.ID] = struct{}{}
		}
		newLogs = append(newLogs, log)
	}
	return newLogs
}

func advanceFunctionLogsSinceCursor(current *time.Time, logs []functionsLogEntry) *time.Time {
	next := current
	for _, log := range logs {
		ts, err := time.Parse(time.RFC3339, log.CreatedAt)
		if err != nil {
			continue
		}
		ts = ts.UTC()
		if next == nil || ts.After(*next) {
			value := ts
			next = &value
		}
	}
	return next
}

func sleepUntilNextFunctionsLogsPoll(ctx context.Context) error {
	timer := time.NewTimer(functionsLogsFollowPollInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncate(s string, max int) string {
	if max <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
