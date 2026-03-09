package sbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// internalTablePrefixes lists Supabase-internal schemas/table prefixes to skip.
var internalTablePrefixes = []string{
	"_supabase_",
	"_realtime_",
	"_analytics_",
	"_pgsodium_",
	"_prisma_",
}

// internalTableNames lists exact table names known to be managed by Supabase.
var internalTableNames = map[string]struct{}{
	"schema_migrations":       {},
	"supabase_migrations":     {},
	"buckets_vectors":         {},
	"vector_indexes":          {},
	"storage.buckets_vectors": {},
	"storage.vector_indexes":  {},
}

// isInternalTable returns true if the table name belongs to a Supabase internal system.
func isInternalTable(name string) bool {
	if _, ok := internalTableNames[name]; ok {
		return true
	}
	for _, prefix := range internalTablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isAYBTable returns true if the table name belongs to AYB's internal tables.
func isAYBTable(name string) bool {
	return strings.HasPrefix(name, "_ayb_")
}

// introspectTables queries information_schema for public schema tables,
// skipping Supabase internals and AYB system tables.
func introspectTables(ctx context.Context, db *sql.DB) ([]TableInfo, error) {
	// Get all public tables.
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tables: %w", err)
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning table name: %w", err)
		}
		if isInternalTable(name) || isAYBTable(name) {
			continue
		}
		tableNames = append(tableNames, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var tables []TableInfo
	for _, name := range tableNames {
		ti, err := introspectTable(ctx, db, name)
		if err != nil {
			return nil, fmt.Errorf("introspecting table %s: %w", name, err)
		}
		tables = append(tables, *ti)
	}

	return tables, nil
}

// introspectTable gets detailed column/constraint info for a single table.
func introspectTable(ctx context.Context, db *sql.DB, tableName string) (*TableInfo, error) {
	ti := &TableInfo{Name: tableName}

	// Columns.
	colRows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, COALESCE(column_default, ''), ordinal_position
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, tableName)
	if err != nil {
		return nil, fmt.Errorf("querying columns: %w", err)
	}
	defer colRows.Close()

	for colRows.Next() {
		var c ColumnInfo
		var nullable string
		if err := colRows.Scan(&c.Name, &c.DataType, &nullable, &c.DefaultValue, &c.OrdinalPos); err != nil {
			return nil, fmt.Errorf("scanning column: %w", err)
		}
		c.IsNullable = nullable == "YES"
		ti.Columns = append(ti.Columns, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}

	// Primary key.
	err = db.QueryRowContext(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary
		LIMIT 1
	`, "public."+tableName).Scan(&ti.PrimaryKey)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("querying primary key: %w", err)
	}

	// Foreign keys.
	fkRows, err := db.QueryContext(ctx, `
		SELECT tc.constraint_name, kcu.column_name,
		       ccu.table_name AS ref_table, ccu.column_name AS ref_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
		WHERE tc.table_schema = 'public'
		  AND tc.table_name = $1
		  AND tc.constraint_type = 'FOREIGN KEY'
		ORDER BY tc.constraint_name
	`, tableName)
	if err != nil {
		return nil, fmt.Errorf("querying foreign keys: %w", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var fk ForeignKeyInfo
		if err := fkRows.Scan(&fk.ConstraintName, &fk.ColumnName, &fk.RefTable, &fk.RefColumn); err != nil {
			return nil, fmt.Errorf("scanning foreign key: %w", err)
		}
		ti.ForeignKeys = append(ti.ForeignKeys, fk)
	}
	if err := fkRows.Err(); err != nil {
		return nil, err
	}

	// Row count (approximate is fine for pre-flight; exact for small tables).
	err = db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %q`, tableName)).Scan(&ti.RowCount)
	if err != nil {
		return nil, fmt.Errorf("counting rows: %w", err)
	}

	return ti, nil
}

// introspectViews gets user-defined view definitions from the public schema.
func introspectViews(ctx context.Context, db *sql.DB) ([]ViewInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT viewname, definition
		FROM pg_views
		WHERE schemaname = 'public'
		ORDER BY viewname
	`)
	if err != nil {
		return nil, fmt.Errorf("querying views: %w", err)
	}
	defer rows.Close()

	var views []ViewInfo
	for rows.Next() {
		var v ViewInfo
		if err := rows.Scan(&v.Name, &v.Definition); err != nil {
			return nil, fmt.Errorf("scanning view: %w", err)
		}
		views = append(views, v)
	}
	return views, rows.Err()
}

// pgTypeName maps information_schema data types to PostgreSQL DDL type names.
// information_schema uses verbose names like "character varying" while DDL uses "varchar".
func pgTypeName(infoSchemaType string) string {
	switch infoSchemaType {
	case "character varying":
		return "varchar"
	case "character":
		return "char"
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	case "time without time zone":
		return "time"
	case "time with time zone":
		return "timetz"
	case "double precision":
		return "float8"
	case "boolean":
		return "bool"
	case "ARRAY":
		return "jsonb" // fallback: arrays become jsonb in the target
	case "USER-DEFINED":
		return "text" // fallback: enums, PostGIS geometry, etc. become text
	default:
		return infoSchemaType
	}
}

// createTableSQL generates a CREATE TABLE DDL statement from a TableInfo.
// This is a pure function with no DB dependencies, easy to unit test.
func createTableSQL(table TableInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS %q (\n", table.Name)

	for i, col := range table.Columns {
		typeName := pgTypeName(col.DataType)
		fmt.Fprintf(&sb, "  %q %s", col.Name, typeName)
		if !col.IsNullable {
			sb.WriteString(" NOT NULL")
		}
		if col.DefaultValue != "" {
			fmt.Fprintf(&sb, " DEFAULT %s", col.DefaultValue)
		}
		if i < len(table.Columns)-1 || table.PrimaryKey != "" || len(table.ForeignKeys) > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	if table.PrimaryKey != "" {
		hasFKs := len(table.ForeignKeys) > 0
		fmt.Fprintf(&sb, "  PRIMARY KEY (%q)", table.PrimaryKey)
		if hasFKs {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	for i, fk := range table.ForeignKeys {
		fmt.Fprintf(&sb, "  CONSTRAINT %q FOREIGN KEY (%q) REFERENCES %q(%q)",
			fk.ConstraintName, fk.ColumnName, fk.RefTable, fk.RefColumn)
		if i < len(table.ForeignKeys)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(");")
	return sb.String()
}

// createViewSQL generates a CREATE OR REPLACE VIEW statement.
func createViewSQL(view ViewInfo) string {
	return fmt.Sprintf("CREATE OR REPLACE VIEW %q AS %s", view.Name, view.Definition)
}

// copyTableData streams rows from source to target in batches.
// progressFn is called after each batch with the cumulative count.
func copyTableData(ctx context.Context, source *sql.DB, tx *sql.Tx, table TableInfo, progressFn func(int)) (int, error) {
	if len(table.Columns) == 0 {
		return 0, nil
	}

	// Build column list.
	colNames := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		colNames[i] = fmt.Sprintf("%q", c.Name)
	}
	colList := strings.Join(colNames, ", ")

	// SELECT from source.
	selectSQL := fmt.Sprintf("SELECT %s FROM %q ORDER BY 1", colList, table.Name)
	rows, err := source.QueryContext(ctx, selectSQL)
	if err != nil {
		return 0, fmt.Errorf("selecting from %s: %w", table.Name, err)
	}
	defer rows.Close()

	// Prepare INSERT statement with placeholders.
	placeholders := make([]string, len(table.Columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	insertSQL := fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		table.Name, colList, strings.Join(placeholders, ", "))

	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return 0, fmt.Errorf("preparing insert for %s: %w", table.Name, err)
	}
	defer stmt.Close()

	total := 0
	const batchSize = 1000

	for rows.Next() {
		// Scan into []any.
		vals := make([]any, len(table.Columns))
		ptrs := make([]any, len(table.Columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return total, fmt.Errorf("scanning row from %s: %w", table.Name, err)
		}

		result, err := stmt.ExecContext(ctx, vals...)
		if err != nil {
			return total, fmt.Errorf("inserting row into %s: %w", table.Name, err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			total++
		}

		if total%batchSize == 0 && progressFn != nil {
			progressFn(total)
		}
	}
	if err := rows.Err(); err != nil {
		return total, err
	}

	// Final progress callback.
	if progressFn != nil {
		progressFn(total)
	}

	return total, nil
}

// resetSequences resets owned sequences to max(pk)+1 for each table.
func resetSequences(ctx context.Context, tx *sql.Tx, tables []TableInfo) (int, error) {
	count := 0
	for _, t := range tables {
		if t.PrimaryKey == "" {
			continue
		}
		// Only reset if the column looks like a serial/identity.
		hasDefault := false
		for _, c := range t.Columns {
			if c.Name == t.PrimaryKey && strings.Contains(c.DefaultValue, "nextval") {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			continue
		}

		resetSQL := fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence(%s, %s), COALESCE(MAX(%q), 1)) FROM %q`,
			quoteLiteral(t.Name), quoteLiteral(t.PrimaryKey), t.PrimaryKey, t.Name,
		)
		if _, err := tx.ExecContext(ctx, resetSQL); err != nil {
			return count, fmt.Errorf("resetting sequence for %s.%s: %w", t.Name, t.PrimaryKey, err)
		}
		count++
	}
	return count, nil
}

// quoteLiteral escapes a string for use as a SQL string literal (single-quoted).
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
