// Package schema Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/schema/introspect_tables.go.
package schema

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type extensionStatusQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func loadPostGIS(ctx context.Context, pool *pgxpool.Pool) (bool, string, error) {
	return loadPostGISExtensionStatus(ctx, pool, "postgis")
}

func loadPostGISRaster(ctx context.Context, pool *pgxpool.Pool) (bool, string, error) {
	return loadPostGISExtensionStatus(ctx, pool, "postgis_raster")
}

func loadPostGISExtensionStatus(ctx context.Context, querier extensionStatusQuerier, extensionName string) (bool, string, error) {
	var version string
	err := querier.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname = $1`, extensionName).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("querying %s extension: %w", extensionName, err)
	}
	return true, version, nil
}

// TODO: Document loadPostGISExtensions.
func loadPostGISExtensions(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT extname
		FROM pg_extension
		WHERE extname IN ('postgis_topology', 'postgis_sfcgal', 'address_standardizer')
		ORDER BY extname`)
	if err != nil {
		return nil, fmt.Errorf("querying optional postgis extensions: %w", err)
	}
	defer rows.Close()

	extensions := make([]string, 0, 3)
	for rows.Next() {
		var extName string
		if err := rows.Scan(&extName); err != nil {
			return nil, fmt.Errorf("scanning optional postgis extension: %w", err)
		}
		extensions = append(extensions, extName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return extensions, nil
}

// loadTablesAndColumns retrieves all tables, views, and materialized views with their columns and metadata, populating type information, nullability, defaults, comments, and type classifications (JSON, array, enum, vector).
func loadTablesAndColumns(ctx context.Context, pool *pgxpool.Pool, enums map[uint32]*EnumType, hasPostGIS bool) (map[string]*Table, []string, error) {
	query, args := buildTablesAndColumnsQuery()
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("querying tables and columns: %w", err)
	}
	defer rows.Close()

	tables, schemaSet, err := scanTablesAndColumnsRows(rows, enums)
	if err != nil {
		return nil, nil, err
	}

	if err := enrichSpatialColumnsAfterBaseScan(ctx, pool, tables, hasPostGIS, enrichSpatialColumns); err != nil {
		return nil, nil, err
	}

	return tables, schemaListFromSet(schemaSet), nil
}

// TODO: Document buildTablesAndColumnsQuery.
func buildTablesAndColumnsQuery() (string, []any) {
	filter, args := schemaFilter("n.nspname", 1)
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
		WHERE c.relkind IN ('r', 'v', 'm', 'p', 'f')
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		  AND %s%s
		ORDER BY n.nspname, c.relname, a.attnum`, filter, extraFilter)
	return query, args
}

// TODO: Document scanTablesAndColumnsRows.
func scanTablesAndColumnsRows(rows pgx.Rows, enums map[uint32]*EnumType) (map[string]*Table, map[string]bool, error) {
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
			JSONType:    pgTypeToJSON(colType, isArray, isEnum, isJSON, false),
		}

		if isVectorTypeName(colType) {
			col.IsVector = true
			col.VectorDim = parseVectorDim(colType)
			col.JSONType = "array"
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

	return tables, schemaSet, nil
}

func enrichSpatialColumnsAfterBaseScan(
	ctx context.Context,
	pool *pgxpool.Pool,
	tables map[string]*Table,
	hasPostGIS bool,
	enrichFn func(context.Context, *pgxpool.Pool, map[string]*Table) error,
) error {
	if !hasPostGIS {
		return nil
	}
	return enrichFn(ctx, pool, tables)
}

func schemaListFromSet(schemaSet map[string]bool) []string {
	schemas := make([]string, 0, len(schemaSet))
	for s := range schemaSet {
		schemas = append(schemas, s)
	}
	return schemas
}

func enrichSpatialColumns(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	if err := loadSpatialColumnsFromView(ctx, pool, tables, "geometry_columns"); err != nil {
		return err
	}
	if err := loadSpatialColumnsFromView(ctx, pool, tables, "geography_columns"); err != nil {
		return err
	}
	return nil
}

// TODO: Document enrichRasterColumns.
func enrichRasterColumns(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table) error {
	filter, args := schemaFilter("r_table_schema", 1)

	query := fmt.Sprintf(`
		SELECT r_table_schema, r_table_name, r_raster_column, srid, num_bands
		FROM raster_columns
		WHERE %s`, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying raster_columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			schemaName string
			tableName  string
			columnName string
			srid       int
			numBands   *int
		)
		if err := rows.Scan(&schemaName, &tableName, &columnName, &srid, &numBands); err != nil {
			return fmt.Errorf("scanning raster_columns row: %w", err)
		}
		_ = numBands

		tbl, ok := tables[schemaName+"."+tableName]
		if !ok {
			continue
		}
		col := tbl.ColumnByName(columnName)
		if col == nil {
			continue
		}

		col.IsRaster = true
		col.SRID = srid
		col.JSONType = "string"
	}

	return rows.Err()
}

// loadSpatialColumnsFromView enriches spatial columns by querying the PostGIS geometry_columns or geography_columns view to populate geometry type and SRID information.
func loadSpatialColumnsFromView(ctx context.Context, pool *pgxpool.Pool, tables map[string]*Table, viewName string) error {
	if viewName != "geometry_columns" && viewName != "geography_columns" {
		return fmt.Errorf("invalid spatial view name: %s", viewName)
	}
	columnNameField := "f_geometry_column"
	isGeographyView := false
	if viewName == "geography_columns" {
		columnNameField = "f_geography_column"
		isGeographyView = true
	}

	filter, args := schemaFilter("f_table_schema", 1)

	query := fmt.Sprintf(`
		SELECT f_table_schema, f_table_name, %s, type, srid, coord_dimension
		FROM %s
		WHERE %s`, columnNameField, viewName, filter)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("querying %s: %w", viewName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			schemaName string
			tableName  string
			columnName string
			typeName   string
			srid       int
			coordDim   int
		)
		if err := rows.Scan(&schemaName, &tableName, &columnName, &typeName, &srid, &coordDim); err != nil {
			return fmt.Errorf("scanning %s row: %w", viewName, err)
		}

		tbl, ok := tables[schemaName+"."+tableName]
		if !ok {
			continue
		}
		col := tbl.ColumnByName(columnName)
		if col == nil {
			continue
		}

		col.IsGeometry = true
		col.IsGeography = isGeographyView
		col.GeometryType = normalizeGeometryType(typeName)
		col.SRID = srid
		col.CoordDimension = coordDim
		col.JSONType = pgTypeToJSON(col.TypeName, col.IsArray, col.IsEnum, col.IsJSON, true)
	}

	return rows.Err()
}

// normalizeGeometryType normalizes geometry type names to canonical form, mapping uppercase names to proper case (POINT to Point) and returning an empty string for generic geometry or geography types.
func normalizeGeometryType(typeName string) string {
	normalized := strings.ToUpper(strings.TrimSpace(typeName))

	switch normalized {
	case "", "GEOMETRY", "GEOGRAPHY":
		return ""
	case "POINT":
		return "Point"
	case "LINESTRING":
		return "LineString"
	case "POLYGON":
		return "Polygon"
	case "MULTIPOINT":
		return "MultiPoint"
	case "MULTILINESTRING":
		return "MultiLineString"
	case "MULTIPOLYGON":
		return "MultiPolygon"
	case "GEOMETRYCOLLECTION":
		return "GeometryCollection"
	default:
		return strings.TrimSpace(typeName)
	}
}

func isVectorTypeName(typeName string) bool {
	base := strings.ToLower(strings.TrimSpace(typeName))
	if idx := strings.Index(base, "("); idx > 0 {
		base = strings.TrimSpace(base[:idx])
	}
	return base == "vector"
}

// parseVectorDim extracts the vector dimension from pgvector type names like "vector(1536)", returning the integer dimension or 0 if the name is invalid or malformed.
func parseVectorDim(typeName string) int {
	lower := strings.ToLower(strings.TrimSpace(typeName))
	if !strings.HasPrefix(lower, "vector(") {
		return 0
	}
	inner := lower[len("vector("):]
	end := strings.IndexByte(inner, ')')
	if end <= 0 {
		return 0
	}
	if strings.TrimSpace(inner[end+1:]) != "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(inner[:end]))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
