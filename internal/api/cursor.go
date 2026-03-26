// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/api/cursor.go.
package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

const maxCursorLen = 4096 // max encoded cursor string length
const cursorSortSelectAliasPrefix = "__cursor_sort_"

// SortField is a parsed sort column with direction.
type SortField struct {
	Column       string
	Expr         string
	ResultColumn string
	Desc         bool
}

type cursorSortProjection struct {
	Fields        []SortField
	Selects       []string
	HelperColumns []string
}

// parseSortFields parses the sort parameter into structured SortField values.
// Format is identical to parseSortSQL: "-created,+name" → [{created,true},{name,false}].
// Unknown columns are silently skipped. At most maxSortFields are returned.
func parseSortFields(tbl *schema.Table, sortParam string) []SortField {
	parsed, err := parseStructuredSort(tbl, sortParam, true)
	if err != nil {
		return nil
	}
	return plainSortFieldsFromStructuredSort(parsed)
}

// ensurePKTiebreaker appends any PK columns not already in the sort list.
// This guarantees deterministic ordering required for keyset pagination.
func ensurePKTiebreaker(tbl *schema.Table, fields []SortField) []SortField {
	if len(tbl.PrimaryKey) == 0 {
		return fields
	}

	present := make(map[string]bool, len(fields))
	for _, f := range fields {
		present[f.Column] = true
	}

	result := make([]SortField, len(fields), len(fields)+len(tbl.PrimaryKey))
	copy(result, fields)

	for _, pk := range tbl.PrimaryKey {
		if !present[pk] {
			result = append(result, SortField{Column: pk, Desc: false})
		}
	}

	return result
}

// sortFieldsToSQL converts structured sort fields into an ORDER BY clause string.
// Output matches parseSortSQL for the same input.
func sortFieldsToSQL(fields []SortField) string {
	if len(fields) == 0 {
		return ""
	}

	parts := make([]string, len(fields))
	for i, f := range fields {
		dir := "ASC"
		if f.Desc {
			dir = "DESC"
		}
		parts[i] = sortFieldExpr(f) + " " + dir
	}

	return strings.Join(parts, ", ")
}

func sortFieldExpr(field SortField) string {
	if field.Expr != "" {
		return field.Expr
	}
	return sqlutil.QuoteIdent(field.Column)
}

func sortFieldResultColumn(field SortField) string {
	if field.ResultColumn != "" {
		return field.ResultColumn
	}
	return field.Column
}

func projectedResultColumns(tbl *schema.Table, requestedFields []string) map[string]struct{} {
	result := make(map[string]struct{}, len(requestedFields))
	for _, field := range requestedFields {
		if tbl.ColumnByName(field) == nil {
			continue
		}
		result[field] = struct{}{}
	}
	return result
}

// TODO: Document nextAvailableCursorSortAlias.
func nextAvailableCursorSortAlias(resultColumns map[string]struct{}, sortIndex int) string {
	baseAlias := fmt.Sprintf("%s%d", cursorSortSelectAliasPrefix, sortIndex)
	if _, exists := resultColumns[baseAlias]; !exists {
		resultColumns[baseAlias] = struct{}{}
		return baseAlias
	}

	for suffix := 1; ; suffix++ {
		alias := fmt.Sprintf("%s_%d", baseAlias, suffix)
		if _, exists := resultColumns[alias]; exists {
			continue
		}
		resultColumns[alias] = struct{}{}
		return alias
	}
}

// TODO: Document prepareCursorSortProjection.
func prepareCursorSortProjection(tbl *schema.Table, requestedFields []string, sortFields []SortField) cursorSortProjection {
	if len(requestedFields) == 0 {
		return cursorSortProjection{Fields: sortFields}
	}

	resultColumns := projectedResultColumns(tbl, requestedFields)
	projected := append([]SortField(nil), sortFields...)
	extraSelects := make([]string, 0, len(sortFields))
	helperColumns := make([]string, 0, len(sortFields))

	for i, field := range projected {
		if field.Column == distanceSortOutputColumn {
			continue
		}
		if _, ok := resultColumns[field.Column]; ok {
			continue
		}

		alias := nextAvailableCursorSortAlias(resultColumns, i)
		projected[i].ResultColumn = alias
		extraSelects = append(extraSelects, fmt.Sprintf(`%s AS %s`, sortFieldExpr(field), sqlutil.QuoteIdent(alias)))
		helperColumns = append(helperColumns, alias)
	}

	return cursorSortProjection{
		Fields:        projected,
		Selects:       extraSelects,
		HelperColumns: helperColumns,
	}
}

// cursorPayload is the JSON structure encoded into cursor tokens.
type cursorPayload struct {
	V []any `json:"v"`
}

// encodeCursor encodes sort key values into an opaque cursor string.
// Uses base64url encoding (no padding) for URL safety.
func encodeCursor(values []any) string {
	data, _ := json.Marshal(cursorPayload{V: values})
	return base64.RawURLEncoding.EncodeToString(data)
}

// decodeCursor decodes an opaque cursor string back into sort key values.
// Returns a user-friendly error for any malformed input.
func decodeCursor(encoded string) ([]any, error) {
	if len(encoded) > maxCursorLen {
		return nil, fmt.Errorf("cursor too long")
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}

	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}

	if len(payload.V) == 0 {
		return nil, fmt.Errorf("invalid cursor")
	}

	return payload.V, nil
}

// buildCursorWhere generates a keyset WHERE clause for cursor-based pagination.
//
// For uniform direction (all ASC or all DESC), uses PostgreSQL tuple comparison:
//
//	(col1, col2) > ($1, $2)   -- all ASC
//	(col1, col2) < ($1, $2)   -- all DESC
//
// For mixed directions, uses expanded OR/AND chains:
//
//	(a < $1) OR (a = $1 AND b > $2)   -- a DESC, b ASC
func buildCursorWhere(fields []SortField, values []any, argOffset int) (string, []any, error) {
	if len(fields) != len(values) {
		return "", nil, fmt.Errorf("cursor has %d values but sort has %d fields", len(values), len(fields))
	}

	if len(fields) == 0 {
		return "", nil, fmt.Errorf("no sort fields for cursor")
	}

	args := make([]any, len(values))
	copy(args, values)

	// Check if all directions are uniform.
	uniform := true
	firstDesc := fields[0].Desc
	for _, f := range fields[1:] {
		if f.Desc != firstDesc {
			uniform = false
			break
		}
	}

	if uniform {
		return buildTupleCursorWhere(fields, argOffset, firstDesc), args, nil
	}

	return buildExpandedCursorWhere(fields, argOffset), args, nil
}

// buildTupleCursorWhere generates a tuple comparison for uniform sort direction.
func buildTupleCursorWhere(fields []SortField, argOffset int, desc bool) string {
	op := ">"
	if desc {
		op = "<"
	}

	if len(fields) == 1 {
		return fmt.Sprintf("%s %s $%d", sortFieldExpr(fields[0]), op, argOffset)
	}

	cols := make([]string, len(fields))
	params := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = sortFieldExpr(f)
		params[i] = fmt.Sprintf("$%d", argOffset+i)
	}

	return fmt.Sprintf("(%s) %s (%s)", strings.Join(cols, ", "), op, strings.Join(params, ", "))
}

// buildExpandedCursorWhere generates the OR/AND chain form for mixed sort directions.
// For fields (a DESC, b ASC, c ASC):
//
//	(a < $1) OR (a = $1 AND b > $2) OR (a = $1 AND b = $2 AND c > $3)
func buildExpandedCursorWhere(fields []SortField, argOffset int) string {
	var orParts []string

	for i := range fields {
		var andParts []string

		// Equality prefix for all previous fields.
		for j := 0; j < i; j++ {
			andParts = append(andParts, fmt.Sprintf("%s = $%d", sortFieldExpr(fields[j]), argOffset+j))
		}

		// Comparison for the current field.
		op := ">"
		if fields[i].Desc {
			op = "<"
		}
		andParts = append(andParts, fmt.Sprintf("%s %s $%d", sortFieldExpr(fields[i]), op, argOffset+i))

		if len(andParts) == 1 {
			orParts = append(orParts, "("+andParts[0]+")")
		} else {
			orParts = append(orParts, "("+strings.Join(andParts, " AND ")+")")
		}
	}

	return strings.Join(orParts, " OR ")
}

// extractCursorValues plucks sort key values from a result row in sort field order.
func extractCursorValues(fields []SortField, record map[string]any) []any {
	values := make([]any, len(fields))
	for i, f := range fields {
		values[i] = record[sortFieldResultColumn(f)]
	}
	return values
}

// buildListWithCursor builds a SELECT query for cursor-based pagination.
// Uses LIMIT perPage+1 to detect whether more rows exist.
func buildListWithCursor(tbl *schema.Table, opts listOpts, sortFields []SortField, cursorWhere string, cursorArgs []any) (string, []any) {
	cols := buildColumnList(tbl, opts.fields)
	cols = appendSelectExprs(cols, append([]string{opts.distanceSelect}, opts.cursorSelects...))
	ref := sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name)

	basePredicate, baseArgs := combineSQLConditions(
		sqlCondition{clause: opts.filterSQL, args: opts.filterArgs},
		sqlCondition{clause: opts.spatialSQL, args: opts.spatialArgs},
		sqlCondition{clause: opts.searchSQL, args: opts.searchArgs},
	)

	var whereParts []string
	if basePredicate != "" {
		whereParts = append(whereParts, basePredicate)
	}
	if cursorWhere != "" {
		whereParts = append(whereParts, cursorWhere)
	}

	allArgs := make([]any, 0, len(baseArgs)+len(opts.sortArgs)+len(cursorArgs)+1)
	allArgs = append(allArgs, baseArgs...)
	allArgs = append(allArgs, opts.sortArgs...)
	allArgs = append(allArgs, cursorArgs...)

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	orderClause := ""
	if sql := sortFieldsToSQL(sortFields); sql != "" {
		orderClause = " ORDER BY " + sql
	} else if opts.searchRank != "" {
		orderClause = " ORDER BY " + opts.searchRank + " DESC"
	}

	limitArg := opts.perPage + 1
	argIdx := len(allArgs) + 1

	dataQuery := fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT $%d",
		cols, ref, whereClause, orderClause, argIdx)
	allArgs = append(allArgs, limitArg)

	return dataQuery, allArgs
}

// TODO: Document stripCursorHelperFields.
func stripCursorHelperFields(items []map[string]any, helperColumns []string) {
	if len(helperColumns) == 0 {
		return
	}

	helperSet := make(map[string]struct{}, len(helperColumns))
	for _, column := range helperColumns {
		helperSet[column] = struct{}{}
	}

	for _, item := range items {
		for key := range helperSet {
			delete(item, key)
		}
	}
}
