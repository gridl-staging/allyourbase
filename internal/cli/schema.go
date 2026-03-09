package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [table]",
	Short: "Inspect database schema from the running AYB server",
	Long: `Display database schema information from the running AYB server.

Without arguments, lists all tables with their column count and kind.
With a table name, shows full detail: columns, types, foreign keys, indexes.

Examples:
  ayb schema
  ayb schema posts
  ayb schema --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSchema,
}

func init() {
	schemaCmd.Flags().String("admin-token", "", "Admin/JWT token (or set AYB_ADMIN_TOKEN)")
	schemaCmd.Flags().String("url", "", "Server URL (default http://127.0.0.1:8090)")
}

type schemaTable struct {
	Schema      string         `json:"schema"`
	Name        string         `json:"name"`
	Kind        string         `json:"kind"`
	Comment     string         `json:"comment,omitempty"`
	Columns     []schemaColumn `json:"columns"`
	PrimaryKey  []string       `json:"primaryKey"`
	ForeignKeys []schemaFK     `json:"foreignKeys,omitempty"`
	Indexes     []schemaIndex  `json:"indexes,omitempty"`
}

type schemaColumn struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Nullable     bool   `json:"nullable"`
	Default      string `json:"default,omitempty"`
	IsPrimaryKey bool   `json:"isPrimaryKey"`
}

type schemaFK struct {
	ConstraintName    string   `json:"constraintName"`
	Columns           []string `json:"columns"`
	ReferencedSchema  string   `json:"referencedSchema"`
	ReferencedTable   string   `json:"referencedTable"`
	ReferencedColumns []string `json:"referencedColumns"`
}

type schemaIndex struct {
	Name       string `json:"name"`
	IsUnique   bool   `json:"isUnique"`
	IsPrimary  bool   `json:"isPrimary"`
	Method     string `json:"method"`
	Definition string `json:"definition"`
}

func runSchema(cmd *cobra.Command, args []string) error {
	outFmt := outputFormat(cmd)
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")

	if token == "" {
		token = adminToken()
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	req, err := http.NewRequest("GET", baseURL+"/api/schema", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, respBody)
	}

	var cache struct {
		Tables    map[string]json.RawMessage `json:"tables"`
		Functions map[string]json.RawMessage `json:"functions"`
		Schemas   []string                   `json:"schemas"`
	}
	if err := json.Unmarshal(respBody, &cache); err != nil {
		return fmt.Errorf("parsing schema: %w", err)
	}

	// Parse tables into typed structs.
	tables := make(map[string]schemaTable, len(cache.Tables))
	for key, raw := range cache.Tables {
		var t schemaTable
		if err := json.Unmarshal(raw, &t); err != nil {
			return fmt.Errorf("parsing table %s: %w", key, err)
		}
		tables[key] = t
	}

	// If a specific table was requested, show detail.
	if len(args) == 1 {
		return showTableDetail(args[0], tables, outFmt)
	}

	// Otherwise, list all tables.
	return listTables(tables, outFmt)
}

func listTables(tables map[string]schemaTable, outFmt string) error {
	if outFmt == "json" {
		// Build sorted list for JSON.
		list := make([]map[string]any, 0, len(tables))
		for _, t := range tables {
			list = append(list, map[string]any{
				"schema":  t.Schema,
				"name":    t.Name,
				"kind":    t.Kind,
				"columns": len(t.Columns),
			})
		}
		sort.Slice(list, func(i, j int) bool {
			si := list[i]["schema"].(string) + "." + list[i]["name"].(string)
			sj := list[j]["schema"].(string) + "." + list[j]["name"].(string)
			return si < sj
		})
		out, _ := json.MarshalIndent(list, "", "  ")
		os.Stdout.Write(out)
		fmt.Println()
		return nil
	}

	// Sort by schema.name for display.
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		fmt.Println("No tables found.")
		return nil
	}

	if outFmt == "csv" {
		cols := []string{"Schema", "Name", "Kind", "Columns", "PK"}
		rows := make([][]string, len(keys))
		for i, key := range keys {
			t := tables[key]
			pk := "-"
			if len(t.PrimaryKey) > 0 {
				pk = strings.Join(t.PrimaryKey, ",")
			}
			rows[i] = []string{t.Schema, t.Name, t.Kind, fmt.Sprintf("%d", len(t.Columns)), pk}
		}
		return writeCSVStdout(cols, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "Schema\tName\tKind\tColumns\tPK")
	fmt.Fprintln(w, "---\t---\t---\t---\t---")
	for _, key := range keys {
		t := tables[key]
		pk := "-"
		if len(t.PrimaryKey) > 0 {
			pk = strings.Join(t.PrimaryKey, ",")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", t.Schema, t.Name, t.Kind, len(t.Columns), pk)
	}
	w.Flush()
	fmt.Printf("\n%d table(s)\n", len(keys))
	return nil
}

func showTableDetail(name string, tables map[string]schemaTable, outFmt string) error {
	// Find the table — try exact key first, then unqualified name.
	var found *schemaTable
	if t, ok := tables[name]; ok {
		found = &t
	} else if t, ok := tables["public."+name]; ok {
		found = &t
	} else {
		for _, t := range tables {
			if t.Name == name {
				tt := t
				found = &tt
				break
			}
		}
	}

	if found == nil {
		return fmt.Errorf("table %q not found", name)
	}

	if outFmt == "json" {
		out, _ := json.MarshalIndent(found, "", "  ")
		os.Stdout.Write(out)
		fmt.Println()
		return nil
	}

	if outFmt == "csv" {
		cols := []string{"Name", "Type", "Nullable", "Default", "PK"}
		rows := make([][]string, len(found.Columns))
		for i, col := range found.Columns {
			nullable := ""
			if col.Nullable {
				nullable = "YES"
			}
			pk := ""
			if col.IsPrimaryKey {
				pk = "PK"
			}
			rows[i] = []string{col.Name, col.Type, nullable, col.Default, pk}
		}
		return writeCSVStdout(cols, rows)
	}

	// Header
	fmt.Printf("%s.%s (%s)\n", found.Schema, found.Name, found.Kind)
	if found.Comment != "" {
		fmt.Printf("  %s\n", found.Comment)
	}
	fmt.Println()

	// Columns
	fmt.Println("Columns:")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "  Name\tType\tNullable\tDefault\tPK")
	fmt.Fprintln(w, "  ---\t---\t---\t---\t---")
	for _, col := range found.Columns {
		nullable := ""
		if col.Nullable {
			nullable = "YES"
		}
		pk := ""
		if col.IsPrimaryKey {
			pk = "PK"
		}
		def := col.Default
		if len(def) > 30 {
			def = def[:27] + "..."
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", col.Name, col.Type, nullable, def, pk)
	}
	w.Flush()

	// Foreign keys
	if len(found.ForeignKeys) > 0 {
		fmt.Println("\nForeign Keys:")
		for _, fk := range found.ForeignKeys {
			fmt.Printf("  %s: (%s) → %s.%s(%s)\n",
				fk.ConstraintName,
				strings.Join(fk.Columns, ", "),
				fk.ReferencedSchema,
				fk.ReferencedTable,
				strings.Join(fk.ReferencedColumns, ", "),
			)
		}
	}

	// Indexes
	if len(found.Indexes) > 0 {
		fmt.Println("\nIndexes:")
		for _, idx := range found.Indexes {
			flags := ""
			if idx.IsPrimary {
				flags = " [PRIMARY]"
			} else if idx.IsUnique {
				flags = " [UNIQUE]"
			}
			fmt.Printf("  %s (%s)%s\n", idx.Name, idx.Method, flags)
		}
	}

	return nil
}
