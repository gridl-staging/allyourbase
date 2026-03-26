// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/api/sort.go.
package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

const distanceSortOutputColumn = "_distance"

type StructuredSort struct {
	Terms []StructuredSortTerm
}

type StructuredSortTerm struct {
	Column   *schema.Column
	Desc     bool
	Distance *spatial.DistanceSort
}

type resolvedSort struct {
	Fields         []SortField
	DistanceSelect string
	Args           []any
}

// TODO: Document parseStructuredSort.
func parseStructuredSort(tbl *schema.Table, sortParam string, hasPostGIS bool) (StructuredSort, error) {
	if sortParam == "" {
		return StructuredSort{}, nil
	}

	rawTerms := splitSortTerms(sortParam)
	parsed := StructuredSort{Terms: make([]StructuredSortTerm, 0, len(rawTerms))}
	distanceSeen := false

	nonEmptyTermIndex := 0
	for _, rawTerm := range rawTerms {
		termText := strings.TrimSpace(rawTerm)
		if termText == "" {
			continue
		}

		term, isDistance, err := parseDistanceSortTerm(tbl, termText, hasPostGIS)
		if err != nil {
			return StructuredSort{}, err
		}
		if isDistance {
			if distanceSeen {
				return StructuredSort{}, fmt.Errorf("sort may include at most one distance term")
			}
			if nonEmptyTermIndex > 0 {
				return StructuredSort{}, fmt.Errorf("distance sort term must be the first sort term")
			}
			distanceSeen = true
			parsed.Terms = append(parsed.Terms, term)
			nonEmptyTermIndex++
			if len(parsed.Terms) >= maxSortFields {
				break
			}
			continue
		}

		colName, desc := parsePlainSortToken(termText)
		if colName == "" {
			nonEmptyTermIndex++
			continue
		}

		col := tbl.ColumnByName(colName)
		if col == nil {
			nonEmptyTermIndex++
			continue
		}

		parsed.Terms = append(parsed.Terms, StructuredSortTerm{Column: col, Desc: desc})
		nonEmptyTermIndex++
		if len(parsed.Terms) >= maxSortFields {
			break
		}
	}

	return parsed, nil
}

// TODO: Document parseDistanceSortTerm.
func parseDistanceSortTerm(tbl *schema.Table, token string, hasPostGIS bool) (StructuredSortTerm, bool, error) {
	const distanceMarker = ".distance("

	idx := strings.Index(token, distanceMarker)
	if idx < 0 {
		return StructuredSortTerm{}, false, nil
	}
	if !strings.HasSuffix(token, ")") {
		return StructuredSortTerm{}, true, fmt.Errorf("malformed distance sort %q", token)
	}
	if strings.HasPrefix(token, "+") || strings.HasPrefix(token, "-") {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort does not support +/- prefixes")
	}

	colName := strings.TrimSpace(token[:idx])
	if colName == "" {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort is missing a column name")
	}

	if !hasPostGIS {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort requires PostGIS extension")
	}

	col := tbl.ColumnByName(colName)
	if col == nil {
		return StructuredSortTerm{}, true, fmt.Errorf("column %q not found in table %q", colName, tbl.Name)
	}
	if !col.IsGeometry && !col.IsGeography {
		return StructuredSortTerm{}, true, fmt.Errorf("column %q is not a spatial column", colName)
	}
	if tbl.ColumnByName(distanceSortOutputColumn) != nil {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort output column %q conflicts with a real column in table %q", distanceSortOutputColumn, tbl.Name)
	}

	coordsText := strings.TrimSpace(token[idx+len(distanceMarker) : len(token)-1])
	coordParts := strings.Split(coordsText, ",")
	if len(coordParts) != 2 {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort must use format: column.distance(lng,lat)")
	}

	longitude, err := strconv.ParseFloat(strings.TrimSpace(coordParts[0]), 64)
	if err != nil {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort longitude must be a number: %w", err)
	}
	latitude, err := strconv.ParseFloat(strings.TrimSpace(coordParts[1]), 64)
	if err != nil {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort latitude must be a number: %w", err)
	}
	if err := spatial.ValidateWGS84Point(longitude, latitude); err != nil {
		return StructuredSortTerm{}, true, fmt.Errorf("distance sort requires valid WGS84 coordinates: %w", err)
	}

	distance := &spatial.DistanceSort{
		Column:    col,
		Longitude: longitude,
		Latitude:  latitude,
	}

	return StructuredSortTerm{Column: col, Distance: distance}, true, nil
}

func parsePlainSortToken(token string) (column string, desc bool) {
	column = token
	if strings.HasPrefix(token, "-") {
		return token[1:], true
	}
	if strings.HasPrefix(token, "+") {
		return token[1:], false
	}
	return column, false
}

// TODO: Document splitSortTerms.
func splitSortTerms(sortParam string) []string {
	var (
		terms []string
		depth int
		start int
	)

	for i, ch := range sortParam {
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				terms = append(terms, sortParam[start:i])
				start = i + 1
			}
		}
	}

	terms = append(terms, sortParam[start:])
	return terms
}

// TODO: Document ensureStructuredSortPKTiebreaker.
func ensureStructuredSortPKTiebreaker(tbl *schema.Table, sort StructuredSort) StructuredSort {
	if len(tbl.PrimaryKey) == 0 {
		return sort
	}

	present := make(map[string]bool, len(sort.Terms))
	for _, term := range sort.Terms {
		if term.Distance != nil || term.Column == nil {
			continue
		}
		present[term.Column.Name] = true
	}

	terms := make([]StructuredSortTerm, 0, len(sort.Terms)+len(tbl.PrimaryKey))
	terms = append(terms, sort.Terms...)
	for _, pk := range tbl.PrimaryKey {
		if present[pk] {
			continue
		}
		if col := tbl.ColumnByName(pk); col != nil {
			terms = append(terms, StructuredSortTerm{Column: col})
		}
	}

	return StructuredSort{Terms: terms}
}

// TODO: Document resolveStructuredSort.
func resolveStructuredSort(sort StructuredSort, paramOffset int) (resolvedSort, error) {
	out := resolvedSort{Fields: make([]SortField, 0, len(sort.Terms))}
	for _, term := range sort.Terms {
		if term.Distance != nil {
			expr, args, err := term.Distance.Expr(paramOffset + len(out.Args))
			if err != nil {
				return resolvedSort{}, err
			}
			out.Fields = append(out.Fields, SortField{Column: distanceSortOutputColumn, Expr: expr, Desc: term.Desc})
			out.DistanceSelect = fmt.Sprintf(`%s AS %s`, expr, sqlutil.QuoteIdent(distanceSortOutputColumn))
			out.Args = append(out.Args, args...)
			continue
		}
		if term.Column == nil {
			continue
		}
		expr := sqlutil.QuoteIdent(term.Column.Name)
		out.Fields = append(out.Fields, SortField{Column: term.Column.Name, Expr: expr, Desc: term.Desc})
	}
	return out, nil
}

func plainSortFieldsFromStructuredSort(sort StructuredSort) []SortField {
	fields := make([]SortField, 0, len(sort.Terms))
	for _, term := range sort.Terms {
		if term.Distance != nil || term.Column == nil {
			continue
		}
		fields = append(fields, SortField{Column: term.Column.Name, Desc: term.Desc})
	}
	return fields
}
