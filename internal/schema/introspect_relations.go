package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// loadEnums retrieves all user-defined enum types from non-system schemas, returning them indexed by object identifier (OID).
func loadEnums(ctx context.Context, pool *pgxpool.Pool) (map[uint32]*EnumType, error) {
	filter, args := schemaFilter("n.nspname", 1)

	query := fmt.Sprintf(`
		SELECT n.nspname, t.typname, t.oid,
		       array_agg(e.enumlabel ORDER BY e.enumsortorder)::text[]
		FROM pg_type t
		  JOIN pg_namespace n ON n.oid = t.typnamespace
		  JOIN pg_enum e ON e.enumtypid = t.oid
		WHERE t.typtype = 'e' AND %s
		GROUP BY n.nspname, t.typname, t.oid
		ORDER BY n.nspname, t.typname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying enums: %w", err)
	}
	defer rows.Close()

	enums := make(map[uint32]*EnumType)
	for rows.Next() {
		var enumType EnumType
		if err := rows.Scan(&enumType.Schema, &enumType.Name, &enumType.OID, &enumType.Values); err != nil {
			return nil, fmt.Errorf("scanning enum: %w", err)
		}
		enums[enumType.OID] = &enumType
	}
	return enums, rows.Err()
}

// loadFunctions retrieves user-defined functions (excluding triggers) with their input parameters, return type, and set-returning status, filtering out OUT and TABLE mode parameters.
func loadFunctions(ctx context.Context, pool *pgxpool.Pool) (map[string]*Function, error) {
	filter, args := schemaFilter("n.nspname", 1)

	// Use proallargtypes/proargmodes when available (functions with OUT/VARIADIC params)
	// to correctly identify parameter modes. Fall back to proargtypes for simple IN-only functions.
	query := fmt.Sprintf(`
		SELECT n.nspname                             AS func_schema,
		       p.proname                             AS func_name,
		       COALESCE(obj_description(p.oid, 'pg_proc'), '') AS func_comment,
		       COALESCE(p.proargnames, '{}')         AS arg_names,
		       COALESCE(
		         (SELECT array_agg(format_type(t, NULL) ORDER BY ord)
		          FROM unnest(COALESCE(p.proallargtypes, p.proargtypes::oid[])) WITH ORDINALITY AS x(t, ord)),
		         '{}'
		       )                                     AS all_arg_types,
		       COALESCE(p.proargmodes::text[], '{}') AS arg_modes,
		       format_type(p.prorettype, NULL)        AS return_type,
		       p.proretset                           AS returns_set
		FROM pg_proc p
		  JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.prokind = 'f'
		  AND p.prorettype != 'trigger'::regtype
		  AND %s
		ORDER BY n.nspname, p.proname`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying functions: %w", err)
	}
	defer rows.Close()

	functions := make(map[string]*Function)
	for rows.Next() {
		var (
			funcSchema, funcName, funcComment string
			argNames                          []string
			allArgTypes                       []string
			argModes                          []string
			returnType                        string
			returnsSet                        bool
		)
		if err := rows.Scan(
			&funcSchema, &funcName, &funcComment,
			&argNames, &allArgTypes, &argModes,
			&returnType, &returnsSet,
		); err != nil {
			return nil, fmt.Errorf("scanning function: %w", err)
		}

		// Build input parameters: filter to IN ('i'), INOUT ('b'), and VARIADIC ('v') modes.
		// When argModes is empty, all params are IN (proargmodes is NULL for all-IN functions).
		var params []*FuncParam
		hasOutParams := false
		position := 0
		for i, typeName := range allArgTypes {
			mode := "i" // default to IN when proargmodes is NULL
			if i < len(argModes) {
				mode = argModes[i]
			}

			if mode == "o" || mode == "t" {
				hasOutParams = hasOutParams || mode == "o"
				continue // skip OUT and TABLE params
			}

			name := ""
			if i < len(argNames) {
				name = argNames[i]
			}
			position++
			params = append(params, &FuncParam{
				Name:       name,
				Type:       typeName,
				Position:   position,
				IsVariadic: mode == "v",
			})
		}

		key := funcSchema + "." + funcName
		functions[key] = &Function{
			Schema:       funcSchema,
			Name:         funcName,
			Comment:      funcComment,
			Parameters:   params,
			ReturnType:   returnType,
			ReturnsSet:   returnsSet,
			IsVoid:       returnType == "void",
			HasOutParams: hasOutParams,
		}
	}
	return functions, rows.Err()
}

// buildRelationships derives forward (many-to-one) and reverse (one-to-many)
// relationships from foreign keys.
func buildRelationships(tables map[string]*Table) {
	for _, tbl := range tables {
		for _, fk := range tbl.ForeignKeys {
			refKey := fk.ReferencedSchema + "." + fk.ReferencedTable

			// Forward: many-to-one (this table -> referenced table).
			forward := &Relationship{
				Name:        fk.ConstraintName,
				Type:        "many-to-one",
				FromSchema:  tbl.Schema,
				FromTable:   tbl.Name,
				FromColumns: fk.Columns,
				ToSchema:    fk.ReferencedSchema,
				ToTable:     fk.ReferencedTable,
				ToColumns:   fk.ReferencedColumns,
				FieldName:   deriveFieldName(fk.Columns, fk.ReferencedTable),
			}
			tbl.Relationships = append(tbl.Relationships, forward)

			// Reverse: one-to-many (referenced table -> this table).
			if refTbl, ok := tables[refKey]; ok {
				reverse := &Relationship{
					Name:        fk.ConstraintName,
					Type:        "one-to-many",
					FromSchema:  fk.ReferencedSchema,
					FromTable:   fk.ReferencedTable,
					FromColumns: fk.ReferencedColumns,
					ToSchema:    tbl.Schema,
					ToTable:     tbl.Name,
					ToColumns:   fk.Columns,
					FieldName:   tbl.Name,
				}
				refTbl.Relationships = append(refTbl.Relationships, reverse)
			}
		}
	}
}

// deriveFieldName generates a human-friendly field name from FK columns.
// "author_id" -> "author", "user_id" -> "user".
func deriveFieldName(columns []string, referencedTable string) string {
	if len(columns) == 1 {
		columnName := columns[0]
		if strings.HasSuffix(columnName, "_id") {
			return strings.TrimSuffix(columnName, "_id")
		}
	}
	return referencedTable
}
