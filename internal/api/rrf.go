// Package api This file implements Reciprocal Rank Fusion (RRF) for merging ranked result lists from full-text search and vector search into a combined sorted list.
package api

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// rrfMerge performs Reciprocal Rank Fusion on two ranked result lists.
// Each input list must be pre-sorted by its respective signal (FTS by ts_rank DESC,
// vector by distance ASC). pkColumns identifies the primary key columns used to
// match rows across lists. k is the RRF constant (typically 60).
//
// Returns a merged list sorted by descending _hybrid_score, with deterministic
// tie-breaking by primary key values ascending.
func rrfMerge(ftsResults, vectorResults []map[string]any, pkColumns []string, k int) []map[string]any {
	if len(ftsResults) == 0 && len(vectorResults) == 0 {
		return nil
	}

	// merged holds the fused rows keyed by composite PK string.
	type entry struct {
		row   map[string]any
		score float64
		pkKey string
	}
	index := make(map[string]*entry)
	var entries []*entry

	// Process FTS results.
	for i, row := range ftsResults {
		pk := compositeKey(row, pkColumns)
		e, exists := index[pk]
		if !exists {
			merged := copyRow(row)
			e = &entry{row: merged, pkKey: pk}
			index[pk] = e
			entries = append(entries, e)
		}
		e.score += 1.0 / float64(k+i+1) // position is 1-based
		e.row["_fts_rank"] = row["_fts_rank"]
	}

	// Process vector results.
	for i, row := range vectorResults {
		pk := compositeKey(row, pkColumns)
		e, exists := index[pk]
		if !exists {
			merged := copyRow(row)
			// Remove raw _distance; we'll add _vector_distance
			delete(merged, "_distance")
			e = &entry{row: merged, pkKey: pk}
			index[pk] = e
			entries = append(entries, e)
		}
		e.score += 1.0 / float64(k+i+1)
		e.row["_vector_distance"] = row["_distance"]
	}

	// Set _hybrid_score on each entry.
	for _, e := range entries {
		e.row["_hybrid_score"] = e.score
	}

	// Sort by descending score, then ascending PK for tie-breaking.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].score != entries[j].score {
			return entries[i].score > entries[j].score
		}
		return compareRowPK(entries[i].row, entries[j].row, pkColumns) < 0
	})

	result := make([]map[string]any, len(entries))
	for i, e := range entries {
		result[i] = e.row
	}
	return result
}

// compareRowPK compares two rows by their primary key columns in order, using numeric comparison for numeric types and string comparison otherwise. It returns -1 if the first row sorts before the second, 0 if equal, and 1 otherwise, with floating-point comparisons using a tolerance of 1e-12.
func compareRowPK(a, b map[string]any, pkColumns []string) int {
	for _, col := range pkColumns {
		av, bv := a[col], b[col]
		an, aok := toFloat64(av)
		bn, bok := toFloat64(bv)
		if aok && bok {
			if math.Abs(an-bn) > 1e-12 {
				if an < bn {
					return -1
				}
				return 1
			}
			continue
		}
		as, bs := fmt.Sprintf("%v", av), fmt.Sprintf("%v", bv)
		if as < bs {
			return -1
		}
		if as > bs {
			return 1
		}
	}
	return 0
}

// toFloat64 attempts to convert v to a float64, returning the value and a boolean indicating success. It handles all signed and unsigned integer types, floating-point types, and numeric string representations.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// compositeKey builds a string key from the primary key column values of a row.
func compositeKey(row map[string]any, pkColumns []string) string {
	if len(pkColumns) == 1 {
		return fmt.Sprintf("%v", row[pkColumns[0]])
	}
	parts := make([]string, len(pkColumns))
	for i, pk := range pkColumns {
		parts[i] = fmt.Sprintf("%v", row[pk])
	}
	return strings.Join(parts, "\x00")
}

// copyRow makes a shallow copy of a row map.
func copyRow(row map[string]any) map[string]any {
	m := make(map[string]any, len(row))
	for k, v := range row {
		m[k] = v
	}
	return m
}
