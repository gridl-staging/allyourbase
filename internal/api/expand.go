package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
)

const maxExpandDepth = 2

// expandRecords populates the "expand" key on each record for the given expand parameter.
// Supports comma-separated relations and dot-notation for nested expansion (depth limit 2).
// Claims are checked to enforce API key table restrictions on related tables.
func expandRecords(ctx context.Context, pool Querier, sc *schema.SchemaCache, tbl *schema.Table, records []map[string]any, expandParam string, logger *slog.Logger) {
	if len(records) == 0 || expandParam == "" {
		return
	}

	claims := auth.ClaimsFromContext(ctx)

	relations := strings.Split(expandParam, ",")
	count := 0
	for _, rel := range relations {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}

		count++
		if count > maxExpandRelations {
			break
		}

		// Split on dot for nested expansion.
		parts := strings.SplitN(rel, ".", maxExpandDepth+1)
		if len(parts) > maxExpandDepth {
			parts = parts[:maxExpandDepth]
		}

		expandRelation(ctx, pool, sc, tbl, records, parts, 0, claims, logger)
	}
}

// findRelation looks up a relationship by field name or FK column name.
// Returns nil if no match is found.
func findRelation(tbl *schema.Table, name string) *schema.Relationship {
	for _, r := range tbl.Relationships {
		if r.FieldName == name {
			return r
		}
		if r.Type == "many-to-one" && len(r.FromColumns) == 1 && r.FromColumns[0] == name {
			return r
		}
	}
	return nil
}

// expandRelation expands a single relation (possibly nested) on the given records.
// Table scope is checked for each related table to prevent API key scope bypass.
func expandRelation(ctx context.Context, pool Querier, sc *schema.SchemaCache, tbl *schema.Table, records []map[string]any, relPath []string, depth int, claims *auth.Claims, logger *slog.Logger) {
	if depth >= maxExpandDepth || len(relPath) == 0 {
		return
	}

	rel := findRelation(tbl, relPath[0])
	if rel == nil {
		return
	}

	// Find the related table.
	relTableKey := rel.ToSchema + "." + rel.ToTable
	relTable := sc.Tables[relTableKey]
	if relTable == nil {
		return
	}

	// Check API key table restrictions for the related table.
	if err := auth.CheckTableScope(claims, relTable.Name); err != nil {
		return // silently skip — the key is not allowed to see this table
	}

	switch rel.Type {
	case "many-to-one":
		expandManyToOne(ctx, pool, sc, relTable, records, rel, relPath, depth, claims, logger)
	case "one-to-many":
		expandOneToMany(ctx, pool, sc, relTable, records, rel, relPath, depth, claims, logger)
	}
}

// collectUniqueValues collects unique non-nil values for a given column from a set of records.
func collectUniqueValues(records []map[string]any, col string) []any {
	seen := make(map[any]bool)
	var values []any
	for _, rec := range records {
		v, ok := rec[col]
		if !ok || v == nil {
			continue
		}
		if !seen[v] {
			seen[v] = true
			values = append(values, v)
		}
	}
	return values
}

// fetchRelated runs a batch SELECT * FROM relTable WHERE targetCol IN (...values).
// Returns the matching rows, or nil on error (errors are logged, not returned).
func fetchRelated(ctx context.Context, pool Querier, relTable *schema.Table, targetCol string, values []any, logger *slog.Logger, relName string) []map[string]any {
	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE %s IN (%s)",
		tableRef(relTable),
		quoteIdent(targetCol),
		strings.Join(placeholders, ", "),
	)

	rows, err := pool.Query(ctx, query, values...)
	if err != nil {
		logger.Error("expand query error", "error", err, "relation", relName)
		return nil
	}
	defer rows.Close()

	related, err := scanRows(rows)
	if err != nil {
		logger.Error("expand scan error", "error", err, "relation", relName)
		return nil
	}
	return related
}

// expandManyToOne expands a many-to-one relationship (e.g., post.author_id → user).
// Collects unique FK values, does a single batch query, and attaches results.
func expandManyToOne(ctx context.Context, pool Querier, sc *schema.SchemaCache, relTable *schema.Table, records []map[string]any, rel *schema.Relationship, relPath []string, depth int, claims *auth.Claims, logger *slog.Logger) {
	if len(rel.FromColumns) == 0 || len(rel.ToColumns) == 0 {
		return
	}

	fkCol := rel.FromColumns[0]
	targetCol := rel.ToColumns[0]

	fkValues := collectUniqueValues(records, fkCol)
	if len(fkValues) == 0 {
		return
	}

	related := fetchRelated(ctx, pool, relTable, targetCol, fkValues, logger, rel.FieldName)
	if len(related) == 0 {
		return
	}

	// Nested expansion on the related records.
	if len(relPath) > 1 {
		expandRelation(ctx, pool, sc, relTable, related, relPath[1:], depth+1, claims, logger)
	}

	// Index by target column value.
	index := make(map[any]map[string]any, len(related))
	for _, r := range related {
		index[r[targetCol]] = r
	}

	// Attach to each record under "expand" key.
	for _, rec := range records {
		fkVal := rec[fkCol]
		if fkVal == nil {
			continue
		}
		if related, ok := index[fkVal]; ok {
			expand := getOrCreateExpand(rec)
			expand[rel.FieldName] = related
		}
	}
}

// expandOneToMany expands a one-to-many relationship (e.g., user → posts).
func expandOneToMany(ctx context.Context, pool Querier, sc *schema.SchemaCache, relTable *schema.Table, records []map[string]any, rel *schema.Relationship, relPath []string, depth int, claims *auth.Claims, logger *slog.Logger) {
	if len(rel.FromColumns) == 0 || len(rel.ToColumns) == 0 {
		return
	}

	fromCol := rel.FromColumns[0]
	targetCol := rel.ToColumns[0]

	ourValues := collectUniqueValues(records, fromCol)
	if len(ourValues) == 0 {
		return
	}

	related := fetchRelated(ctx, pool, relTable, targetCol, ourValues, logger, rel.FieldName)
	if len(related) == 0 {
		return
	}

	// Nested expansion.
	if len(relPath) > 1 {
		expandRelation(ctx, pool, sc, relTable, related, relPath[1:], depth+1, claims, logger)
	}

	// Group by target column value.
	groups := make(map[any][]map[string]any)
	for _, r := range related {
		groups[r[targetCol]] = append(groups[r[targetCol]], r)
	}

	// Attach to each record.
	for _, rec := range records {
		ourVal := rec[fromCol]
		if ourVal == nil {
			continue
		}
		if group, ok := groups[ourVal]; ok {
			expand := getOrCreateExpand(rec)
			expand[rel.FieldName] = group
		}
	}
}

func getOrCreateExpand(rec map[string]any) map[string]any {
	if existing, ok := rec["expand"]; ok {
		if m, ok := existing.(map[string]any); ok {
			return m
		}
	}
	m := make(map[string]any)
	rec["expand"] = m
	return m
}
