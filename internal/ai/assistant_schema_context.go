package ai

import (
	"fmt"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// CompactSchemaContext renders a deterministic bounded summary of schema metadata.
func CompactSchemaContext(cache *schema.SchemaCache, maxChars int) string {
	if cache == nil {
		return "schema: unavailable"
	}
	if maxChars <= 0 {
		maxChars = assistantSchemaContextMaxChars
	}

	var builder strings.Builder
	capabilities := []string{"postgres"}
	if cache.HasPgVector {
		capabilities = append(capabilities, "pgvector")
	}
	if cache.HasPostGIS {
		capabilities = append(capabilities, "postgis")
	}
	builder.WriteString("capabilities: ")
	builder.WriteString(strings.Join(capabilities, ", "))
	builder.WriteString("\n")

	tableKeys := make([]string, 0, len(cache.Tables))
	for key := range cache.Tables {
		tableKeys = append(tableKeys, key)
	}
	sort.Strings(tableKeys)
	for _, key := range tableKeys {
		table := cache.Tables[key]
		if table == nil {
			continue
		}
		line := fmt.Sprintf("table %s.%s (%s) pk=[%s] RLS=%t\n", table.Schema, table.Name, table.Kind, strings.Join(table.PrimaryKey, ","), table.RLSEnabled)
		if appendWithBudget(&builder, line, maxChars) {
			return trimWithEllipsis(builder.String(), maxChars)
		}

		columnParts := make([]string, 0, len(table.Columns))
		for _, col := range table.Columns {
			if col == nil {
				continue
			}
			columnParts = append(columnParts, fmt.Sprintf("%s:%s", col.Name, col.TypeName))
		}
		if len(columnParts) > 0 {
			if appendWithBudget(&builder, "  columns "+strings.Join(columnParts, ", ")+"\n", maxChars) {
				return trimWithEllipsis(builder.String(), maxChars)
			}
		}

		if len(table.ForeignKeys) > 0 {
			fkParts := make([]string, 0, len(table.ForeignKeys))
			for _, fk := range table.ForeignKeys {
				if fk == nil {
					continue
				}
				fkParts = append(fkParts, fmt.Sprintf("%s->%s.%s(%s)", strings.Join(fk.Columns, ","), fk.ReferencedSchema, fk.ReferencedTable, strings.Join(fk.ReferencedColumns, ",")))
			}
			sort.Strings(fkParts)
			if len(fkParts) > 0 && appendWithBudget(&builder, "  fks "+strings.Join(fkParts, "; ")+"\n", maxChars) {
				return trimWithEllipsis(builder.String(), maxChars)
			}
		}

		if len(table.Indexes) > 0 {
			idxParts := make([]string, 0, len(table.Indexes))
			for _, idx := range table.Indexes {
				if idx == nil {
					continue
				}
				idxParts = append(idxParts, idx.Name+"("+idx.Method+")")
			}
			sort.Strings(idxParts)
			if len(idxParts) > 0 && appendWithBudget(&builder, "  indexes "+strings.Join(idxParts, ", ")+"\n", maxChars) {
				return trimWithEllipsis(builder.String(), maxChars)
			}
		}

		if len(table.RLSPolicies) > 0 {
			policyParts := make([]string, 0, len(table.RLSPolicies))
			for _, policy := range table.RLSPolicies {
				if policy == nil {
					continue
				}
				policyParts = append(policyParts, fmt.Sprintf("%s:%s", policy.Name, policy.Command))
			}
			sort.Strings(policyParts)
			if len(policyParts) > 0 && appendWithBudget(&builder, "  RLS "+strings.Join(policyParts, ", ")+"\n", maxChars) {
				return trimWithEllipsis(builder.String(), maxChars)
			}
		}
	}

	functionKeys := make([]string, 0, len(cache.Functions))
	for key := range cache.Functions {
		functionKeys = append(functionKeys, key)
	}
	sort.Strings(functionKeys)
	for _, key := range functionKeys {
		fn := cache.Functions[key]
		if fn == nil {
			continue
		}
		line := fmt.Sprintf("function %s.%s returns %s\n", fn.Schema, fn.Name, fn.ReturnType)
		if appendWithBudget(&builder, line, maxChars) {
			return trimWithEllipsis(builder.String(), maxChars)
		}
	}

	return trimWithEllipsis(builder.String(), maxChars)
}

func appendWithBudget(builder *strings.Builder, chunk string, maxChars int) bool {
	if builder.Len()+len(chunk) > maxChars {
		return true
	}
	builder.WriteString(chunk)
	return false
}

func trimWithEllipsis(input string, maxChars int) string {
	if maxChars <= 0 || len(input) <= maxChars {
		return strings.TrimSpace(input)
	}
	if maxChars <= 3 {
		return input[:maxChars]
	}
	return strings.TrimSpace(input[:maxChars-3]) + "..."
}
