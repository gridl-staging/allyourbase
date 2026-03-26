// Package cli provides PostgreSQL foreign data wrapper management commands for listing, creating, importing from, and dropping FDW servers.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/fdw"
	"github.com/spf13/cobra"
)

type dbFDWServersResponse struct {
	Servers []dbFDWServerEntry `json:"servers"`
}

type dbFDWServerEntry struct {
	Name      string `json:"name"`
	FDWType   string `json:"fdw_type"`
	CreatedAt string `json:"created_at"`
}

type dbFDWTablesResponse struct {
	Tables []dbFDWTableEntry `json:"tables"`
}

type dbFDWTableEntry struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	ServerName string `json:"server_name"`
}

var dbFDWCmd = &cobra.Command{
	Use:   "fdw",
	Short: "Manage PostgreSQL foreign data wrappers",
}

var dbFDWListServersCmd = &cobra.Command{
	Use:   "list-servers",
	Short: "List configured FDW servers",
	RunE:  runDBFDWListServers,
}

var dbFDWCreateServerCmd = &cobra.Command{
	Use:   "create-server <name>",
	Short: "Create an FDW server",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBFDWCreateServer,
}

var dbFDWImportTablesCmd = &cobra.Command{
	Use:   "import-tables <server-name>",
	Short: "Import foreign tables from a remote schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBFDWImportTables,
}

var dbFDWDropServerCmd = &cobra.Command{
	Use:   "drop-server <name>",
	Short: "Drop an FDW server",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBFDWDropServer,
}

func init() {
	dbFDWCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	dbFDWCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	dbFDWCreateServerCmd.Flags().String("type", "", "FDW type: postgres_fdw or file_fdw")
	dbFDWCreateServerCmd.Flags().String("host", "", "Remote host (postgres_fdw)")
	dbFDWCreateServerCmd.Flags().String("port", "", "Remote port (postgres_fdw)")
	dbFDWCreateServerCmd.Flags().String("dbname", "", "Remote database name (postgres_fdw)")
	dbFDWCreateServerCmd.Flags().String("filename", "", "Local file path (file_fdw)")
	dbFDWCreateServerCmd.Flags().String("user", "", "Remote user (postgres_fdw)")
	dbFDWCreateServerCmd.Flags().String("password", "", "Remote password (postgres_fdw)")
	_ = dbFDWCreateServerCmd.MarkFlagRequired("type")

	dbFDWImportTablesCmd.Flags().String("schema", "", "Remote schema name")
	dbFDWImportTablesCmd.Flags().String("local-schema", "", "Local schema name")
	dbFDWImportTablesCmd.Flags().String("tables", "", "Comma-separated table names to import")

	dbFDWDropServerCmd.Flags().Bool("cascade", false, "Drop dependent foreign tables as well")

	dbFDWCmd.AddCommand(dbFDWListServersCmd)
	dbFDWCmd.AddCommand(dbFDWCreateServerCmd)
	dbFDWCmd.AddCommand(dbFDWImportTablesCmd)
	dbFDWCmd.AddCommand(dbFDWDropServerCmd)
	dbCmd.AddCommand(dbFDWCmd)
}

// runDBFDWListServers retrieves configured FDW servers from the admin API and outputs them as JSON or formatted table.
func runDBFDWListServers(cmd *cobra.Command, _ []string) error {
	resp, body, err := adminRequest(cmd, "GET", "/api/admin/fdw/servers", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("admin request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if outputFormat(cmd) == "json" {
		return writePrettyJSON(cmd, body)
	}

	var payload dbFDWServersResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("decoding fdw server response: %w", err)
		}
	}
	return writeFDWServersTable(cmd, payload.Servers)
}

// runDBFDWCreateServer validates type-specific flags and creates an FDW server via the admin API.
func runDBFDWCreateServer(cmd *cobra.Command, args []string) error {
	name := args[0]
	fdwType, _ := cmd.Flags().GetString("type")

	opts := fdw.CreateServerOpts{
		Name:    name,
		FDWType: fdwType,
		Options: map[string]string{},
	}

	switch fdwType {
	case "postgres_fdw":
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetString("port")
		dbName, _ := cmd.Flags().GetString("dbname")
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")
		if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" || strings.TrimSpace(dbName) == "" {
			return fmt.Errorf("--host, --port, and --dbname are required for postgres_fdw")
		}
		if strings.TrimSpace(user) == "" || strings.TrimSpace(password) == "" {
			return fmt.Errorf("--user and --password are required for postgres_fdw")
		}
		opts.Options["host"] = host
		opts.Options["port"] = port
		opts.Options["dbname"] = dbName
		opts.UserMapping = fdw.UserMapping{
			User:     user,
			Password: password,
		}
	case "file_fdw":
		filename, _ := cmd.Flags().GetString("filename")
		if strings.TrimSpace(filename) == "" {
			return fmt.Errorf("--filename is required for file_fdw")
		}
		opts.Options["filename"] = filename
	default:
		return fmt.Errorf("unsupported --type %q (supported: postgres_fdw, file_fdw)", fdwType)
	}

	reqBody, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("encoding create-server request: %w", err)
	}

	resp, body, err := adminRequest(cmd, "POST", "/api/admin/fdw/servers", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("admin request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if outputFormat(cmd) == "json" {
		return writePrettyJSON(cmd, body)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created FDW server %s (%s)\n", name, fdwType)
	return nil
}

// runDBFDWImportTables imports foreign tables from a remote schema into a specified FDW server.
func runDBFDWImportTables(cmd *cobra.Command, args []string) error {
	serverName := args[0]
	remoteSchema, _ := cmd.Flags().GetString("schema")
	localSchema, _ := cmd.Flags().GetString("local-schema")
	tablesRaw, _ := cmd.Flags().GetString("tables")

	opts := fdw.ImportOpts{
		RemoteSchema: remoteSchema,
		LocalSchema:  localSchema,
	}
	if strings.TrimSpace(tablesRaw) != "" {
		parts := strings.Split(tablesRaw, ",")
		opts.TableNames = make([]string, 0, len(parts))
		for _, part := range parts {
			name := strings.TrimSpace(part)
			if name == "" {
				continue
			}
			opts.TableNames = append(opts.TableNames, name)
		}
	}

	reqBody, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("encoding import request: %w", err)
	}

	path := "/api/admin/fdw/servers/" + url.PathEscape(serverName) + "/import"
	resp, body, err := adminRequest(cmd, "POST", path, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("admin request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if outputFormat(cmd) == "json" {
		return writePrettyJSON(cmd, body)
	}

	var payload dbFDWTablesResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return fmt.Errorf("decoding import response: %w", err)
		}
	}
	return writeFDWTablesTable(cmd, payload.Tables)
}

// runDBFDWDropServer deletes an FDW server, optionally cascading to drop dependent foreign tables.
func runDBFDWDropServer(cmd *cobra.Command, args []string) error {
	name := args[0]
	cascade, _ := cmd.Flags().GetBool("cascade")

	path := "/api/admin/fdw/servers/" + url.PathEscape(name)
	if cascade {
		path += "?cascade=true"
	}

	resp, body, err := adminRequest(cmd, "DELETE", path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("admin request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if outputFormat(cmd) == "json" && len(body) > 0 {
		return writePrettyJSON(cmd, body)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Dropped FDW server %s\n", name)
	return nil
}

// writeFDWServersTable formats and writes FDW servers as a table with Name, Type, and Created columns, or a message if none are configured.
func writeFDWServersTable(cmd *cobra.Command, servers []dbFDWServerEntry) error {
	if len(servers) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No FDW servers configured")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "Name\tType\tCreated")
	for _, srv := range servers {
		created := srv.CreatedAt
		if strings.TrimSpace(created) == "" {
			created = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", srv.Name, srv.FDWType, created)
	}
	return w.Flush()
}

func writeFDWTablesTable(cmd *cobra.Command, tables []dbFDWTableEntry) error {
	if len(tables) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No foreign tables imported")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "Schema\tTable\tServer")
	for _, tbl := range tables {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", tbl.Schema, tbl.Name, tbl.ServerName)
	}
	return w.Flush()
}
