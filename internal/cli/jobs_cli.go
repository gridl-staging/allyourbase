package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Manage background jobs",
}

var jobsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List jobs",
	RunE:  runJobsList,
}

var jobsRetryCmd = &cobra.Command{
	Use:   "retry <job-id>",
	Short: "Retry a failed job",
	Args:  cobra.ExactArgs(1),
	RunE:  runJobsRetry,
}

var jobsCancelCmd = &cobra.Command{
	Use:   "cancel <job-id>",
	Short: "Cancel a queued job",
	Args:  cobra.ExactArgs(1),
	RunE:  runJobsCancel,
}

var schedulesCmd = &cobra.Command{
	Use:   "schedules",
	Short: "Manage job schedules",
}

var schedulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all schedules",
	RunE:  runSchedulesList,
}

var schedulesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new schedule",
	RunE:  runSchedulesCreate,
}

var schedulesUpdateCmd = &cobra.Command{
	Use:   "update <schedule-id>",
	Short: "Update a schedule",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchedulesUpdate,
}

var schedulesDeleteCmd = &cobra.Command{
	Use:   "delete <schedule-id>",
	Short: "Delete a schedule",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchedulesDelete,
}

var schedulesEnableCmd = &cobra.Command{
	Use:   "enable <schedule-id>",
	Short: "Enable a schedule",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchedulesEnable,
}

var schedulesDisableCmd = &cobra.Command{
	Use:   "disable <schedule-id>",
	Short: "Disable a schedule",
	Args:  cobra.ExactArgs(1),
	RunE:  runSchedulesDisable,
}

func init() {
	jobsCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	jobsCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	jobsListCmd.Flags().String("state", "", "Filter by state (queued, running, completed, failed, canceled)")
	jobsListCmd.Flags().String("type", "", "Filter by job type")
	jobsListCmd.Flags().Int("limit", 50, "Maximum results")

	jobsCmd.AddCommand(jobsListCmd)
	jobsCmd.AddCommand(jobsRetryCmd)
	jobsCmd.AddCommand(jobsCancelCmd)

	schedulesCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	schedulesCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	schedulesCreateCmd.Flags().String("name", "", "Schedule name (required)")
	schedulesCreateCmd.Flags().String("job-type", "", "Job type (required)")
	schedulesCreateCmd.Flags().String("cron", "", "Cron expression (required)")
	schedulesCreateCmd.Flags().String("timezone", "UTC", "Timezone (IANA)")
	schedulesCreateCmd.Flags().String("payload", "{}", "JSON payload")
	schedulesCreateCmd.Flags().Bool("enabled", true, "Enable schedule")

	schedulesUpdateCmd.Flags().String("cron", "", "Cron expression")
	schedulesUpdateCmd.Flags().String("timezone", "", "Timezone (IANA)")
	schedulesUpdateCmd.Flags().String("payload", "", "JSON payload")
	schedulesUpdateCmd.Flags().String("enabled", "", "Enable/disable (true/false)")

	schedulesCmd.AddCommand(schedulesListCmd)
	schedulesCmd.AddCommand(schedulesCreateCmd)
	schedulesCmd.AddCommand(schedulesUpdateCmd)
	schedulesCmd.AddCommand(schedulesDeleteCmd)
	schedulesCmd.AddCommand(schedulesEnableCmd)
	schedulesCmd.AddCommand(schedulesDisableCmd)

	rootCmd.AddCommand(jobsCmd)
	rootCmd.AddCommand(schedulesCmd)
}

func runJobsList(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	state, _ := cmd.Flags().GetString("state")
	jobType, _ := cmd.Flags().GetString("type")
	limit, _ := cmd.Flags().GetInt("limit")

	path := "/api/admin/jobs?"
	if state != "" {
		path += "state=" + state + "&"
	}
	if jobType != "" {
		path += "type=" + jobType + "&"
	}
	if limit > 0 {
		path += fmt.Sprintf("limit=%d&", limit)
	}

	resp, body, err := adminRequest(cmd, "GET", path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("server error: %s", string(body))
	}

	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if outFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	}

	if len(result.Items) == 0 {
		fmt.Println("No jobs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tSTATE\tATTEMPTS\tCREATED")
	for _, j := range result.Items {
		id, _ := j["id"].(string)
		typ, _ := j["type"].(string)
		st, _ := j["state"].(string)
		attempts, _ := j["attempts"].(float64)
		maxAttempts, _ := j["maxAttempts"].(float64)
		created, _ := j["createdAt"].(string)
		if len(created) > 19 {
			created = created[:19]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.0f/%.0f\t%s\n",
			id, typ, st, attempts, maxAttempts, created)
	}
	return w.Flush()
}

func runJobsRetry(cmd *cobra.Command, args []string) error {
	jobID := args[0]
	resp, body, err := adminRequest(cmd, "POST", "/api/admin/jobs/"+jobID+"/retry", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("retry failed: %s", string(body))
	}

	var job map[string]any
	if err := json.Unmarshal(body, &job); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Job %s reset to %s\n", job["id"], job["state"])
	return nil
}

func runJobsCancel(cmd *cobra.Command, args []string) error {
	jobID := args[0]
	resp, body, err := adminRequest(cmd, "POST", "/api/admin/jobs/"+jobID+"/cancel", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("cancel failed: %s", string(body))
	}

	var job map[string]any
	if err := json.Unmarshal(body, &job); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Job %s canceled\n", job["id"])
	return nil
}

func runSchedulesList(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	resp, body, err := adminRequest(cmd, "GET", "/api/admin/schedules", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("server error: %s", string(body))
	}

	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if outFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	}

	if len(result.Items) == 0 {
		fmt.Println("No schedules found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tJOB TYPE\tCRON\tTIMEZONE\tENABLED")
	for _, s := range result.Items {
		name, _ := s["name"].(string)
		jobType, _ := s["jobType"].(string)
		cron, _ := s["cronExpr"].(string)
		tz, _ := s["timezone"].(string)
		enabled, _ := s["enabled"].(bool)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", name, jobType, cron, tz, enabled)
	}
	return w.Flush()
}

func runSchedulesCreate(cmd *cobra.Command, _ []string) error {
	name, _ := cmd.Flags().GetString("name")
	jobType, _ := cmd.Flags().GetString("job-type")
	cron, _ := cmd.Flags().GetString("cron")
	tz, _ := cmd.Flags().GetString("timezone")
	payloadStr, _ := cmd.Flags().GetString("payload")
	enabled, _ := cmd.Flags().GetBool("enabled")

	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if jobType == "" {
		return fmt.Errorf("--job-type is required")
	}
	if cron == "" {
		return fmt.Errorf("--cron is required")
	}

	payload := map[string]any{
		"name":     name,
		"jobType":  jobType,
		"cronExpr": cron,
		"timezone": tz,
		"enabled":  enabled,
	}
	if payloadStr != "" && payloadStr != "{}" {
		if !json.Valid([]byte(payloadStr)) {
			return fmt.Errorf("invalid --payload JSON")
		}
		payload["payload"] = json.RawMessage(payloadStr)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing schedule payload: %w", err)
	}
	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/schedules", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return fmt.Errorf("create failed: %s", string(respBody))
	}

	var sched map[string]any
	if err := json.Unmarshal(respBody, &sched); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Schedule %q created (id: %s)\n", sched["name"], sched["id"])
	return nil
}

func runSchedulesUpdate(cmd *cobra.Command, args []string) error {
	schedID := args[0]
	cronExpr, _ := cmd.Flags().GetString("cron")
	tz, _ := cmd.Flags().GetString("timezone")
	payloadStr, _ := cmd.Flags().GetString("payload")
	enabledStr, _ := cmd.Flags().GetString("enabled")

	update := map[string]any{}
	if cronExpr != "" {
		update["cronExpr"] = cronExpr
	}
	if tz != "" {
		update["timezone"] = tz
	}
	if payloadStr != "" {
		if !json.Valid([]byte(payloadStr)) {
			return fmt.Errorf("invalid --payload JSON")
		}
		update["payload"] = json.RawMessage(payloadStr)
	}
	if enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err != nil {
			return fmt.Errorf("invalid --enabled value %q: must be true or false", enabledStr)
		}
		update["enabled"] = enabled
	}

	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("serializing schedule update payload: %w", err)
	}
	resp, respBody, err := adminRequest(cmd, "PUT", "/api/admin/schedules/"+schedID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("update failed: %s", string(respBody))
	}

	var sched map[string]any
	if err := json.Unmarshal(respBody, &sched); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Schedule %q updated (cron: %s, tz: %s)\n", sched["name"], sched["cronExpr"], sched["timezone"])
	return nil
}

func runSchedulesDelete(cmd *cobra.Command, args []string) error {
	schedID := args[0]
	resp, respBody, err := adminRequest(cmd, "DELETE", "/api/admin/schedules/"+schedID, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("delete failed: %s", string(respBody))
	}
	fmt.Printf("Schedule %s deleted\n", schedID)
	return nil
}

func runSchedulesEnable(cmd *cobra.Command, args []string) error {
	schedID := args[0]
	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/schedules/"+schedID+"/enable", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("enable failed: %s", string(respBody))
	}

	var sched map[string]any
	if err := json.Unmarshal(respBody, &sched); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Schedule %q enabled\n", sched["name"])
	return nil
}

func runSchedulesDisable(cmd *cobra.Command, args []string) error {
	schedID := args[0]
	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/schedules/"+schedID+"/disable", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("disable failed: %s", string(respBody))
	}

	var sched map[string]any
	if err := json.Unmarshal(respBody, &sched); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Schedule %q disabled\n", sched["name"])
	return nil
}
