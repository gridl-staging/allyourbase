// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/cli/db_replicas.go.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type dbReplicasResponse struct {
	Replicas []dbReplicaEntry `json:"replicas"`
}

type dbReplicaEntry struct {
	URL           string `json:"url"`
	State         string `json:"state"`
	LagBytes      int64  `json:"lag_bytes"`
	LastCheckedAt string `json:"last_checked_at"`
}

type dbReplicaRecord struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	SSLMode     string `json:"ssl_mode"`
	Weight      int    `json:"weight"`
	MaxLagBytes int64  `json:"max_lag_bytes"`
	Role        string `json:"role"`
	State       string `json:"state"`
}

type dbReplicaAddResponse struct {
	Status   string           `json:"status"`
	Record   dbReplicaRecord  `json:"record"`
	Replicas []dbReplicaEntry `json:"replicas"`
}

type dbReplicaPromoteResponse struct {
	Status   string           `json:"status"`
	Primary  dbReplicaRecord  `json:"primary"`
	Replicas []dbReplicaEntry `json:"replicas"`
}

var dbReplicasCmd = &cobra.Command{
	Use:   "replicas",
	Short: "Manage replica routing and health",
}

var dbReplicasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured replicas and current health status",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDBReplicas(cmd, "GET", "/api/admin/replicas")
	},
}

var dbReplicasCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Trigger an immediate replica health check and show results",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDBReplicas(cmd, "POST", "/api/admin/replicas/check")
	},
}

var dbReplicasAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new read replica to the topology",
	RunE:  runDBReplicasAdd,
}

var dbReplicasRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a replica from the topology",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBReplicasRemove,
}

var dbReplicasPromoteCmd = &cobra.Command{
	Use:   "promote [name]",
	Short: "Promote a replica to primary",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBReplicasPromote,
}

var dbReplicasFailoverCmd = &cobra.Command{
	Use:   "failover",
	Short: "Initiate failover to a replica",
	RunE:  runDBReplicasFailover,
}

func init() {
	dbReplicasCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	dbReplicasCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	dbReplicasAddCmd.Flags().String("name", "", "Replica name")
	dbReplicasAddCmd.Flags().String("host", "", "Replica host (required)")
	dbReplicasAddCmd.Flags().Int("port", 5432, "Replica port")
	dbReplicasAddCmd.Flags().String("database", "", "Database name")
	dbReplicasAddCmd.Flags().String("ssl-mode", "", "SSL mode")
	dbReplicasAddCmd.Flags().Int("weight", 1, "Routing weight")
	dbReplicasAddCmd.Flags().Int64("max-lag-bytes", 10*1024*1024, "Maximum replication lag in bytes")

	dbReplicasRemoveCmd.Flags().Bool("force", false, "Force removal even if last active replica")

	dbReplicasFailoverCmd.Flags().String("target", "", "Target replica name (auto-selects if omitted)")
	dbReplicasFailoverCmd.Flags().Bool("force", false, "Force failover even if primary is healthy")

	dbReplicasCmd.AddCommand(dbReplicasListCmd)
	dbReplicasCmd.AddCommand(dbReplicasCheckCmd)
	dbReplicasCmd.AddCommand(dbReplicasAddCmd)
	dbReplicasCmd.AddCommand(dbReplicasRemoveCmd)
	dbReplicasCmd.AddCommand(dbReplicasPromoteCmd)
	dbReplicasCmd.AddCommand(dbReplicasFailoverCmd)
	dbCmd.AddCommand(dbReplicasCmd)
}

// TODO: Document runDBReplicas.
func runDBReplicas(cmd *cobra.Command, method, path string) error {
	resp, body, err := adminRequest(cmd, method, path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("admin request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if outputFormat(cmd) == "json" {
		return writePrettyJSON(cmd, body)
	}
	var payload dbReplicasResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("decoding replica response: %w", err)
		}
	}
	return writeReplicaTable(cmd, payload.Replicas)
}

// TODO: Document runDBReplicasAdd.
func runDBReplicasAdd(cmd *cobra.Command, _ []string) error {
	host, _ := cmd.Flags().GetString("host")
	if host == "" {
		return fmt.Errorf("--host is required")
	}
	name, _ := cmd.Flags().GetString("name")
	port, _ := cmd.Flags().GetInt("port")
	database, _ := cmd.Flags().GetString("database")
	sslMode, _ := cmd.Flags().GetString("ssl-mode")
	weight, _ := cmd.Flags().GetInt("weight")
	maxLagBytes, _ := cmd.Flags().GetInt64("max-lag-bytes")

	payload := map[string]any{
		"name":          name,
		"host":          host,
		"port":          port,
		"database":      database,
		"ssl_mode":      sslMode,
		"weight":        weight,
		"max_lag_bytes": maxLagBytes,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/replicas", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("add replica failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return writeReplicaAddOutput(cmd, respBody)
}

// TODO: Document runDBReplicasRemove.
func runDBReplicasRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	force, _ := cmd.Flags().GetBool("force")

	query := url.Values{}
	query.Set("force", fmt.Sprintf("%t", force))
	path := "/api/admin/replicas/" + url.PathEscape(name) + "?" + query.Encode()
	resp, body, err := adminRequest(cmd, "DELETE", path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("remove replica failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Replica %q removed\n", name)
	return nil
}

func runDBReplicasPromote(cmd *cobra.Command, args []string) error {
	name := args[0]
	path := "/api/admin/replicas/" + url.PathEscape(name) + "/promote"
	resp, body, err := adminRequest(cmd, "POST", path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("promote replica failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return writeReplicaPromoteOutput(cmd, body)
}

// TODO: Document runDBReplicasFailover.
func runDBReplicasFailover(cmd *cobra.Command, _ []string) error {
	target, _ := cmd.Flags().GetString("target")
	force, _ := cmd.Flags().GetBool("force")

	payload := map[string]any{
		"target": target,
		"force":  force,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	resp, respBody, err := adminRequest(cmd, "POST", "/api/admin/replicas/failover", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("failover failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return writePrettyJSON(cmd, respBody)
}

func writePrettyJSON(cmd *cobra.Command, body []byte) error {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("decoding json response: %w", err)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeReplicaAddOutput(cmd *cobra.Command, body []byte) error {
	if outputFormat(cmd) == "json" {
		return writePrettyJSON(cmd, body)
	}

	var payload dbReplicaAddResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decoding add replica response: %w", err)
	}

	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Replica %q added\n", payload.Record.Name)
	return err
}

func writeReplicaPromoteOutput(cmd *cobra.Command, body []byte) error {
	if outputFormat(cmd) == "json" {
		return writePrettyJSON(cmd, body)
	}

	var payload dbReplicaPromoteResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decoding promote replica response: %w", err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Primary promoted to %q\n", payload.Primary.Name); err != nil {
		return err
	}
	return writeReplicaTable(cmd, payload.Replicas)
}

// TODO: Document writeReplicaTable.
func writeReplicaTable(cmd *cobra.Command, replicas []dbReplicaEntry) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if len(replicas) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No replicas configured")
		return nil
	}
	_, _ = fmt.Fprintln(w, "URL\tState\tLag\tLast Checked")
	for _, row := range replicas {
		last := row.LastCheckedAt
		if strings.TrimSpace(last) == "" {
			last = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", row.URL, row.State, row.LagBytes, last)
	}
	return w.Flush()
}
