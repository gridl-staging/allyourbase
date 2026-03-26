// Package schema This file contains database introspection functions that query PostgreSQL system catalogs to build a complete schema cache of tables, columns, constraints, indexes, functions, and other database objects.
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

	hasPostGIS, postGISVersion, err := loadPostGIS(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading postgis status: %w", err)
	}
	hasPostGISRaster, postGISRasterVersion, err := loadPostGISRaster(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading postgis raster status: %w", err)
	}

	tables, schemas, err := loadTablesAndColumns(ctx, pool, enums, hasPostGIS)
	if err != nil {
		return nil, fmt.Errorf("loading tables: %w", err)
	}
	if hasPostGISRaster {
		if err := enrichRasterColumns(ctx, pool, tables); err != nil {
			return nil, fmt.Errorf("loading raster columns: %w", err)
		}
	}

	postGISExtensions := make([]string, 0)
	if hasPostGIS {
		postGISExtensions, err = loadPostGISExtensions(ctx, pool)
		if err != nil {
			return nil, fmt.Errorf("loading optional postgis extensions: %w", err)
		}
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

	if err := loadCheckConstraints(ctx, pool, tables); err != nil {
		return nil, fmt.Errorf("loading check constraints: %w", err)
	}

	if err := loadRLSPolicies(ctx, pool, tables); err != nil {
		return nil, fmt.Errorf("loading RLS policies: %w", err)
	}

	buildRelationships(tables)

	functions, err := loadFunctions(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading functions: %w", err)
	}

	hasPgVector := false
	for _, table := range tables {
		if table.HasVector() {
			hasPgVector = true
			break
		}
	}

	return &SchemaCache{
		Tables:               tables,
		Functions:            functions,
		Enums:                enums,
		Schemas:              schemas,
		HasPostGIS:           hasPostGIS,
		PostGISVersion:       postGISVersion,
		HasPostGISRaster:     hasPostGISRaster,
		PostGISRasterVersion: postGISRasterVersion,
		PostGISExtensions:    postGISExtensions,
		HasPgVector:          hasPgVector,
		BuiltAt:              time.Now(),
	}, nil
}

// schemaFilter returns SQL clauses and args for excluding system schemas.
// columnExpr is the SQL expression identifying the schema column (e.g. "n.nspname"
// for pg_namespace joins, or "f_table_schema" for PostGIS views).
// paramOffset is the starting $N parameter number.
func schemaFilter(columnExpr string, paramOffset int) (clause string, args []any) {
	conditions := make([]string, 0, len(excludedSchemas)+1)
	for i, schemaName := range excludedSchemas {
		conditions = append(conditions, fmt.Sprintf("%s != $%d", columnExpr, paramOffset+i))
		args = append(args, schemaName)
	}
	conditions = append(conditions, fmt.Sprintf("%s NOT LIKE $%d", columnExpr, paramOffset+len(excludedSchemas)))
	args = append(args, "pg_%")
	return strings.Join(conditions, " AND "), args
}
