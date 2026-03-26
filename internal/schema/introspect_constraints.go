package schema

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// loadPrimaryKeys retrieves primary key constraints and marks corresponding columns, populating each table's PrimaryKey list with column names in constraint order.
func loadPrimaryKeys(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("n.nspname", 1)

	query := fmt.Sprintf(`
		SELECT n.nspname, c.relname, cn.conkey
		FROM pg_constraint cn
		  JOIN pg_class c ON c.oid = cn.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE cn.contype = 'p' AND %s
		ORDER BY n.nspname, c.relname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying primary keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName, tableName string
		var colPositions []int16
		if err := rows.Scan(&schemaName, &tableName, &colPositions); err != nil {
			return fmt.Errorf("scanning primary key: %w", err)
		}

		key := schemaName + "." + tableName
		tbl, ok := tables[key]
		if !ok {
			continue
		}

		// Resolve column positions to names.
		for _, pos := range colPositions {
			for _, col := range tbl.Columns {
				if col.Position == int(pos) {
					tbl.PrimaryKey = append(tbl.PrimaryKey, col.Name)
					col.IsPrimaryKey = true
					break
				}
			}
		}
	}
	return rows.Err()
}

// loadForeignKeys retrieves foreign key constraints with column mappings, referenced tables, and delete/update actions, appending ForeignKey objects to each table.
func loadForeignKeys(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("n.nspname", 1)

	query := fmt.Sprintf(`
		SELECT cn.conname,
		       n.nspname, c.relname,
		       (SELECT array_agg(a.attname ORDER BY ord.n)
		        FROM unnest(cn.conkey) WITH ORDINALITY AS ord(attnum, n)
		        JOIN pg_attribute a ON a.attrelid = cn.conrelid AND a.attnum = ord.attnum
		       ),
		       tn.nspname, tc.relname,
		       (SELECT array_agg(a.attname ORDER BY ord.n)
		        FROM unnest(cn.confkey) WITH ORDINALITY AS ord(attnum, n)
		        JOIN pg_attribute a ON a.attrelid = cn.confrelid AND a.attnum = ord.attnum
		       ),
		       cn.confupdtype::text,
		       cn.confdeltype::text
		FROM pg_constraint cn
		  JOIN pg_class c ON c.oid = cn.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_class tc ON tc.oid = cn.confrelid
		  JOIN pg_namespace tn ON tn.oid = tc.relnamespace
		WHERE cn.contype = 'f' AND %s
		ORDER BY n.nspname, c.relname, cn.conname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying foreign keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			constraintName      string
			schemaName, name    string
			columns             []string
			refSchema, refTable string
			refColumns          []string
			onUpdate, onDelete  string
		)
		if err := rows.Scan(
			&constraintName,
			&schemaName, &name, &columns,
			&refSchema, &refTable, &refColumns,
			&onUpdate, &onDelete,
		); err != nil {
			return fmt.Errorf("scanning foreign key: %w", err)
		}

		key := schemaName + "." + name
		tbl, ok := tables[key]
		if !ok {
			continue
		}

		tbl.ForeignKeys = append(tbl.ForeignKeys, &ForeignKey{
			ConstraintName:    constraintName,
			Columns:           columns,
			ReferencedSchema:  refSchema,
			ReferencedTable:   refTable,
			ReferencedColumns: refColumns,
			OnUpdate:          fkActionToString(onUpdate),
			OnDelete:          fkActionToString(onDelete),
		})
	}
	return rows.Err()
}

// loadIndexes retrieves all indexes for tables in non-system schemas, including name, uniqueness, primary key status, access method, and SQL definition.
func loadIndexes(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("tn.nspname", 1)

	query := fmt.Sprintf(`
		SELECT ic.relname,
		       tn.nspname, tc.relname,
		       i.indisunique, i.indisprimary,
		       am.amname,
		       COALESCE((
		         SELECT array_agg(a.attname ORDER BY ord.n)
		         FROM unnest(i.indkey) WITH ORDINALITY AS ord(attnum, n)
		         JOIN pg_attribute a
		           ON a.attrelid = i.indrelid
		          AND a.attnum = ord.attnum
		         WHERE ord.n <= i.indnkeyatts
		       ), '{}'),
		       pg_get_indexdef(i.indexrelid, 0, true)
		FROM pg_index i
		  JOIN pg_class ic ON ic.oid = i.indexrelid
		  JOIN pg_class tc ON tc.oid = i.indrelid
		  JOIN pg_namespace tn ON tn.oid = tc.relnamespace
		  JOIN pg_am am ON am.oid = ic.relam
		WHERE %s
		ORDER BY tn.nspname, tc.relname, ic.relname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying indexes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			indexName, schemaName, tableName string
			isUnique, isPrimary              bool
			method, definition               string
			columns                          []string
		)
		if err := rows.Scan(&indexName, &schemaName, &tableName, &isUnique, &isPrimary, &method, &columns, &definition); err != nil {
			return fmt.Errorf("scanning index: %w", err)
		}

		key := schemaName + "." + tableName
		tbl, ok := tables[key]
		if !ok {
			continue
		}

		tbl.Indexes = append(tbl.Indexes, &Index{
			Name:       indexName,
			IsUnique:   isUnique,
			IsPrimary:  isPrimary,
			Method:     method,
			Columns:    columns,
			Definition: definition,
		})
	}
	return rows.Err()
}

// loadCheckConstraints retrieves check constraints for all tables, appending CheckConstraint objects with the constraint name and CHECK expression.
func loadCheckConstraints(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("n.nspname", 1)

	query := fmt.Sprintf(`
		SELECT n.nspname, c.relname, cn.conname,
		       pg_get_constraintdef(cn.oid, true)
		FROM pg_constraint cn
		  JOIN pg_class c ON c.oid = cn.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE cn.contype = 'c' AND %s
		ORDER BY n.nspname, c.relname, cn.conname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying check constraints: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName, tableName, name, definition string
		if err := rows.Scan(&schemaName, &tableName, &name, &definition); err != nil {
			return fmt.Errorf("scanning check constraint: %w", err)
		}

		key := schemaName + "." + tableName
		tbl, ok := tables[key]
		if !ok {
			continue
		}

		// pg_get_constraintdef returns "CHECK (expr)" — strip the outer wrapper.
		expr := definition
		if len(expr) > 7 && expr[:7] == "CHECK (" && expr[len(expr)-1] == ')' {
			expr = expr[7 : len(expr)-1]
		}
		tbl.CheckConstraints = append(tbl.CheckConstraints, &CheckConstraint{
			Name:       name,
			Definition: expr,
		})
	}
	return rows.Err()
}

// loadRLSPolicies retrieves Row Level Security policies and marks RLS-enabled tables, populating policy metadata including command, permissive/restrictive mode, roles, and USING/WITH CHECK expressions.
func loadRLSPolicies(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("n.nspname", 1)

	// Query RLS-enabled status and policies in one pass.
	// relrowsecurity tells us whether RLS is enabled on the table.
	query := fmt.Sprintf(`
		SELECT n.nspname, c.relname, c.relrowsecurity,
		       p.polname,
		       CASE p.polcmd
		         WHEN '*' THEN 'ALL'
		         WHEN 'r' THEN 'SELECT'
		         WHEN 'a' THEN 'INSERT'
		         WHEN 'w' THEN 'UPDATE'
		         WHEN 'd' THEN 'DELETE'
		       END AS command,
		       p.polpermissive,
		       COALESCE(
		         (SELECT array_agg(r.rolname ORDER BY r.rolname)
		          FROM unnest(p.polroles) AS rid(oid)
		          JOIN pg_roles r ON r.oid = rid.oid),
		         '{}'
		       ) AS roles,
		       COALESCE(pg_get_expr(p.polqual, p.polrelid), '') AS using_expr,
		       COALESCE(pg_get_expr(p.polwithcheck, p.polrelid), '') AS with_check_expr
		FROM pg_policy p
		  JOIN pg_class c ON c.oid = p.polrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE %s
		ORDER BY n.nspname, c.relname, p.polname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying RLS policies: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			schemaName, tableName string
			rlsEnabled            bool
			policyName, command   string
			permissive            bool
			roles                 []string
			usingExpr, checkExpr  string
		)
		if err := rows.Scan(
			&schemaName, &tableName, &rlsEnabled,
			&policyName, &command, &permissive,
			&roles, &usingExpr, &checkExpr,
		); err != nil {
			return fmt.Errorf("scanning RLS policy: %w", err)
		}

		key := schemaName + "." + tableName
		tbl, ok := tables[key]
		if !ok {
			continue
		}

		tbl.RLSEnabled = rlsEnabled
		tbl.RLSPolicies = append(tbl.RLSPolicies, &RLSPolicy{
			Name:          policyName,
			Command:       command,
			Permissive:    permissive,
			Roles:         roles,
			UsingExpr:     usingExpr,
			WithCheckExpr: checkExpr,
		})
	}

	// Also pick up tables that have RLS enabled but no policies yet.
	rlsQuery := fmt.Sprintf(`
		SELECT n.nspname, c.relname
		FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relrowsecurity = true AND %s`, filter)

	rlsRows, err := pool.Query(ctx, rlsQuery, args...)
	if err != nil {
		return fmt.Errorf("querying RLS-enabled tables: %w", err)
	}
	defer rlsRows.Close()

	for rlsRows.Next() {
		var schemaName, tableName string
		if err := rlsRows.Scan(&schemaName, &tableName); err != nil {
			return fmt.Errorf("scanning RLS-enabled table: %w", err)
		}
		key := schemaName + "." + tableName
		if tbl, ok := tables[key]; ok {
			tbl.RLSEnabled = true
		}
	}
	return rlsRows.Err()
}
