package schema

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// excludedSchemas are system schemas that are never introspected.
var excludedSchemas = []string{"information_schema", "pg_catalog", "pg_toast"}

// BuildCache introspects the database and returns a complete SchemaCache.
func BuildCache(ctx context.Context, pool *pgxpool.Pool) (*SchemaCache, error) {
	enums, err := loadEnums(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading enums: %w", err)
	}

	tables, schemas, err := loadTablesAndColumns(ctx, pool, enums)
	if err != nil {
		return nil, fmt.Errorf("loading tables: %w", err)
	}

	if err := loadPrimaryKeys(ctx, pool, tables); err != nil {
		return nil, fmt.Errorf("loading primary keys: %w", err)
	}

	if err := loadForeignKeys(ctx, pool, tables); err != nil {
		return nil, fmt.Errorf("loading foreign keys: %w", err)
	}

	if err := loadIndexes(ctx, pool, tables); err != nil {
		return nil, fmt.Errorf("loading indexes: %w", err)
	}

	buildRelationships(tables)

	functions, err := loadFunctions(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading functions: %w", err)
	}

	return &SchemaCache{
		Tables:    tables,
		Functions: functions,
		Enums:     enums,
		Schemas:   schemas,
		BuiltAt:   time.Now(),
	}, nil
}

// schemaFilter returns SQL clauses and args for excluding system schemas.
// paramOffset is the starting $N parameter number.
func schemaFilter(alias string, paramOffset int) (clause string, args []any) {
	conditions := make([]string, 0, len(excludedSchemas)+1)
	for i, s := range excludedSchemas {
		conditions = append(conditions, fmt.Sprintf("%s.nspname != $%d", alias, paramOffset+i))
		args = append(args, s)
	}
	conditions = append(conditions, fmt.Sprintf("%s.nspname NOT LIKE $%d", alias, paramOffset+len(excludedSchemas)))
	args = append(args, "pg_%")
	return strings.Join(conditions, " AND "), args
}

func loadEnums(ctx context.Context, pool *pgxpool.Pool) (map[uint32]*EnumType, error) {
	filter, args := schemaFilter("n", 1)

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
		var e EnumType
		if err := rows.Scan(&e.Schema, &e.Name, &e.OID, &e.Values); err != nil {
			return nil, fmt.Errorf("scanning enum: %w", err)
		}
		enums[e.OID] = &e
	}
	return enums, rows.Err()
}

func loadTablesAndColumns(ctx context.Context, pool *pgxpool.Pool, enums map[uint32]*EnumType) (map[string]*Table, []string, error) {
	filter, args := schemaFilter("n", 1)

	// Also exclude AYB system tables.
	extraFilter := fmt.Sprintf(" AND c.relname NOT LIKE $%d", len(args)+1)
	args = append(args, "\\_ayb\\_%")

	query := fmt.Sprintf(`
		SELECT n.nspname                              AS table_schema,
		       c.relname                              AS table_name,
		       c.relkind::text                        AS table_kind,
		       COALESCE(obj_description(c.oid), '')   AS table_comment,
		       a.attname                              AS column_name,
		       a.attnum                               AS column_position,
		       format_type(a.atttypid, a.atttypmod)   AS column_type,
		       a.atttypid                             AS type_oid,
		       NOT a.attnotnull                       AS is_nullable,
		       COALESCE(pg_get_expr(d.adbin, d.adrelid), '') AS column_default,
		       COALESCE(col_description(c.oid, a.attnum), '') AS column_comment,
		       t.typcategory::text                     AS type_category
		FROM pg_attribute a
		  JOIN pg_class c ON c.oid = a.attrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_type t ON t.oid = a.atttypid
		  LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
		WHERE c.relkind IN ('r', 'v', 'm', 'p')
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		  AND %s%s
		ORDER BY n.nspname, c.relname, a.attnum`, filter, extraFilter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("querying tables and columns: %w", err)
	}
	defer rows.Close()

	tables := make(map[string]*Table)
	schemaSet := make(map[string]bool)

	for rows.Next() {
		var (
			tableSchema, tableName, tableKind, tableComment string
			colName, colType, colDefault, colComment        string
			colPosition                                     int
			typeOID                                         uint32
			isNullable                                      bool
			typeCategory                                    string
		)

		if err := rows.Scan(
			&tableSchema, &tableName, &tableKind, &tableComment,
			&colName, &colPosition, &colType, &typeOID,
			&isNullable, &colDefault, &colComment, &typeCategory,
		); err != nil {
			return nil, nil, fmt.Errorf("scanning column: %w", err)
		}

		key := tableSchema + "." + tableName
		schemaSet[tableSchema] = true

		tbl, ok := tables[key]
		if !ok {
			tbl = &Table{
				Schema:  tableSchema,
				Name:    tableName,
				Kind:    relkindToString(tableKind),
				Comment: tableComment,
			}
			tables[key] = tbl
		}

		isJSON := typeOID == 114 || typeOID == 3802 // json=114, jsonb=3802
		isArray := typeCategory == "A"
		isEnum := typeCategory == "E"

		col := &Column{
			Name:        colName,
			Position:    colPosition,
			TypeName:    colType,
			TypeOID:     typeOID,
			IsNullable:  isNullable,
			DefaultExpr: colDefault,
			Comment:     colComment,
			IsJSON:      isJSON,
			IsEnum:      isEnum,
			IsArray:     isArray,
			JSONType:    pgTypeToJSON(colType, isArray, isEnum, isJSON),
		}

		// Populate enum values if applicable.
		if isEnum {
			if e, ok := enums[typeOID]; ok {
				col.EnumValues = e.Values
			}
		}

		tbl.Columns = append(tbl.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	schemas := make([]string, 0, len(schemaSet))
	for s := range schemaSet {
		schemas = append(schemas, s)
	}

	return tables, schemas, nil
}

func loadPrimaryKeys(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("n", 1)

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
		var schema, name string
		var colPositions []int16
		if err := rows.Scan(&schema, &name, &colPositions); err != nil {
			return fmt.Errorf("scanning primary key: %w", err)
		}

		key := schema + "." + name
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

func loadForeignKeys(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("n", 1)

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
			schema, name        string
			columns             []string
			refSchema, refTable string
			refColumns          []string
			onUpdate, onDelete  string
		)
		if err := rows.Scan(
			&constraintName,
			&schema, &name, &columns,
			&refSchema, &refTable, &refColumns,
			&onUpdate, &onDelete,
		); err != nil {
			return fmt.Errorf("scanning foreign key: %w", err)
		}

		key := schema + "." + name
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

func loadIndexes(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("tn", 1)

	query := fmt.Sprintf(`
		SELECT ic.relname,
		       tn.nspname, tc.relname,
		       i.indisunique, i.indisprimary,
		       am.amname,
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
			indexName, schema, tableName string
			isUnique, isPrimary          bool
			method, definition           string
		)
		if err := rows.Scan(&indexName, &schema, &tableName, &isUnique, &isPrimary, &method, &definition); err != nil {
			return fmt.Errorf("scanning index: %w", err)
		}

		key := schema + "." + tableName
		tbl, ok := tables[key]
		if !ok {
			continue
		}

		tbl.Indexes = append(tbl.Indexes, &Index{
			Name:       indexName,
			IsUnique:   isUnique,
			IsPrimary:  isPrimary,
			Method:     method,
			Definition: definition,
		})
	}
	return rows.Err()
}

func loadFunctions(ctx context.Context, pool *pgxpool.Pool) (map[string]*Function, error) {
	filter, args := schemaFilter("n", 1)

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
		pos := 0
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
			pos++
			params = append(params, &FuncParam{
				Name:       name,
				Type:       typeName,
				Position:   pos,
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
		col := columns[0]
		if strings.HasSuffix(col, "_id") {
			return strings.TrimSuffix(col, "_id")
		}
	}
	return referencedTable
}
