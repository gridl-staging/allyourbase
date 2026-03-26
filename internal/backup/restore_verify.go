// Package backup provides post-restore validation by comparing primary and recovered database instances through connectivity checks, schema hash verification, and row count comparisons.
package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RestoreCheckResult captures pass/fail and details for a verification check.
type RestoreCheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details"`
}

// RestoreRowCountCheck captures sentinel row-count comparison per table.
type RestoreRowCountCheck struct {
	Table          string `json:"table"`
	PrimaryCount   int64  `json:"primary_count"`
	RecoveredCount int64  `json:"recovered_count"`
	Passed         bool   `json:"passed"`
	Details        string `json:"details"`
}

// RestoreVerification is the result payload for post-restore validation.
type RestoreVerification struct {
	Passed            bool                   `json:"passed"`
	ConnectivityCheck RestoreCheckResult     `json:"connectivity_check"`
	SchemaCheck       RestoreCheckResult     `json:"schema_check"`
	RowCountChecks    []RestoreRowCountCheck `json:"row_count_checks"`
}

type tableSchema struct {
	Schema     string
	Table      string
	Definition string
}

// RestoreVerifier compares primary vs restored instance state.
type RestoreVerifier struct {
	primaryDBURL  string
	recoveryDBURL string

	pingFn       func(ctx context.Context, connURL string) error
	listTablesFn func(ctx context.Context, connURL string) ([]tableSchema, error)
	countTableFn func(ctx context.Context, connURL, fullyQualifiedTable string) (int64, error)
}

func NewRestoreVerifier(primaryDBURL, recoveryDBURL string) *RestoreVerifier {
	return &RestoreVerifier{
		primaryDBURL:  primaryDBURL,
		recoveryDBURL: recoveryDBURL,
		pingFn:        pingDB,
		listTablesFn:  listTableSchemas,
		countTableFn:  countTableRows,
	}
}

// Verify checks that the restored database matches the primary instance by verifying connectivity, schema definitions, and row counts, returning a RestoreVerification with detailed results. It returns an error if initial connectivity checks fail.
func (v *RestoreVerifier) Verify(ctx context.Context) (*RestoreVerification, error) {
	if err := v.pingFn(ctx, v.recoveryDBURL); err != nil {
		return nil, fmt.Errorf("recovery connectivity check failed: %w", err)
	}
	if err := v.pingFn(ctx, v.primaryDBURL); err != nil {
		return nil, fmt.Errorf("primary connectivity check failed: %w", err)
	}

	result := &RestoreVerification{
		ConnectivityCheck: RestoreCheckResult{
			Name:    "connectivity",
			Passed:  true,
			Details: "recovery and primary are reachable",
		},
	}

	primarySchemas, err := v.listTablesFn(ctx, v.primaryDBURL)
	if err != nil {
		return nil, fmt.Errorf("listing primary schema: %w", err)
	}
	recoverySchemas, err := v.listTablesFn(ctx, v.recoveryDBURL)
	if err != nil {
		return nil, fmt.Errorf("listing recovery schema: %w", err)
	}

	primaryHash := hashSchemas(primarySchemas)
	recoveryHash := hashSchemas(recoverySchemas)
	if primaryHash == recoveryHash {
		result.SchemaCheck = RestoreCheckResult{
			Name:    "schema_checksum",
			Passed:  true,
			Details: "schema hashes match",
		}
	} else {
		result.SchemaCheck = RestoreCheckResult{
			Name:    "schema_checksum",
			Passed:  false,
			Details: fmt.Sprintf("schema hash mismatch (primary=%s recovery=%s)", primaryHash, recoveryHash),
		}
	}

	tables := make([]string, 0, len(primarySchemas))
	for _, table := range primarySchemas {
		tables = append(tables, tableName(table))
	}
	sort.Strings(tables)

	result.RowCountChecks = make([]RestoreRowCountCheck, 0, len(tables))
	rowChecksPassed := true
	for _, table := range tables {
		primaryCount, pErr := v.countTableFn(ctx, v.primaryDBURL, table)
		recoveredCount, rErr := v.countTableFn(ctx, v.recoveryDBURL, table)
		check := RestoreRowCountCheck{Table: table, PrimaryCount: primaryCount, RecoveredCount: recoveredCount}
		switch {
		case pErr != nil:
			rowChecksPassed = false
			check.Passed = false
			check.Details = fmt.Sprintf("primary count failed: %v", pErr)
		case rErr != nil:
			rowChecksPassed = false
			check.Passed = false
			check.Details = fmt.Sprintf("recovery count failed: %v", rErr)
		case recoveredCount <= primaryCount:
			check.Passed = true
			check.Details = "recovered count is <= primary count"
		default:
			rowChecksPassed = false
			check.Passed = false
			check.Details = "recovered count exceeds primary count"
		}
		result.RowCountChecks = append(result.RowCountChecks, check)
	}

	result.Passed = result.ConnectivityCheck.Passed && result.SchemaCheck.Passed && rowChecksPassed
	return result, nil
}

// pingDB verifies that a database at the given connection URL is reachable and responsive by executing a SELECT 1 query.
func pingDB(ctx context.Context, connURL string) error {
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	var one int
	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("select 1: %w", err)
	}
	if one != 1 {
		return fmt.Errorf("unexpected ping response: %d", one)
	}
	return nil
}

// listTableSchemas retrieves all user-defined tables from the database and returns their schema information including column definitions.
func listTableSchemas(ctx context.Context, connURL string) ([]tableSchema, error) {
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	// Collect all table names first and close the rows cursor before issuing
	// per-table definition queries. pgx v5 reads rows lazily from the wire,
	// so the connection is busy until rows.Close() — a nested conn.Query
	// while the outer rows are open would fail with "conn busy".
	type tableID struct{ schema, table string }
	var tableIDs []tableID

	rows, err := conn.Query(ctx, `
		SELECT schemaname, tablename
		FROM pg_catalog.pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schemaname, tablename`)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	for rows.Next() {
		var t tableID
		if err := rows.Scan(&t.schema, &t.table); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tableIDs = append(tableIDs, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}

	tables := make([]tableSchema, 0, len(tableIDs))
	for _, t := range tableIDs {
		definition, err := tableDefinition(ctx, conn, t.schema, t.table)
		if err != nil {
			return nil, err
		}
		tables = append(tables, tableSchema{Schema: t.schema, Table: t.table, Definition: definition})
	}
	return tables, nil
}

// tableDefinition returns a string representation of a table's column structure, encoding column names, data types, nullability, and default expressions.
func tableDefinition(ctx context.Context, conn *pgx.Conn, schemaName, tableName string) (string, error) {
	rows, err := conn.Query(ctx, `
		SELECT column_name, data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`,
		schemaName, tableName,
	)
	if err != nil {
		return "", fmt.Errorf("querying columns for %s.%s: %w", schemaName, tableName, err)
	}
	defer rows.Close()

	parts := make([]string, 0)
	for rows.Next() {
		var columnName string
		var dataType string
		var isNullable string
		var defaultExpr string
		if err := rows.Scan(&columnName, &dataType, &isNullable, &defaultExpr); err != nil {
			return "", fmt.Errorf("scanning columns for %s.%s: %w", schemaName, tableName, err)
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%s:%s", columnName, dataType, isNullable, defaultExpr))
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating columns for %s.%s: %w", schemaName, tableName, err)
	}
	return strings.Join(parts, ";"), nil
}

// countTableRows executes a COUNT(*) query against the schema-qualified table and returns the row count.
func countTableRows(ctx context.Context, connURL, fullyQualifiedTable string) (int64, error) {
	parts := strings.Split(fullyQualifiedTable, ".")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid table %q", fullyQualifiedTable)
	}
	conn, err := pgx.Connect(ctx, connURL)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	query := fmt.Sprintf("SELECT count(*) FROM %s", pgx.Identifier{parts[0], parts[1]}.Sanitize())
	var count int64
	if err := conn.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting rows for %s: %w", fullyQualifiedTable, err)
	}
	return count, nil
}

func hashSchemas(tables []tableSchema) string {
	if len(tables) == 0 {
		return ""
	}
	lines := make([]string, 0, len(tables))
	for _, table := range tables {
		lines = append(lines, tableName(table)+"="+table.Definition)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func tableName(t tableSchema) string {
	return t.Schema + "." + t.Table
}
