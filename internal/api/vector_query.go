package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/vector"
)

// findVectorColumn resolves the target vector column for a nearest-neighbor query.
// If vectorColName is non-empty it must exist and be a vector column.
// If empty, auto-selects when exactly one vector column exists.
func findVectorColumn(tbl *schema.Table, vectorColName string) (*schema.Column, error) {
	if vectorColName != "" {
		col := tbl.ColumnByName(vectorColName)
		if col == nil {
			return nil, fmt.Errorf("column %q not found in table %q", vectorColName, tbl.Name)
		}
		if !col.IsVector {
			return nil, fmt.Errorf("column %q is not a vector column (type: %s)", vectorColName, col.TypeName)
		}
		return col, nil
	}

	vecCols := tbl.VectorColumns()
	switch len(vecCols) {
	case 0:
		return nil, fmt.Errorf("table %q has no vector columns", tbl.Name)
	case 1:
		return vecCols[0], nil
	default:
		names := make([]string, len(vecCols))
		for i, c := range vecCols {
			names[i] = c.Name
		}
		return nil, fmt.Errorf("table %q has multiple vector columns (%s); specify vector_column explicitly",
			tbl.Name, strings.Join(names, ", "))
	}
}

// parseNearestVector JSON-decodes the nearest query parameter into a float64 slice
// and validates its dimension against the target column.
func parseNearestVector(raw string, col *schema.Column) ([]float64, error) {
	var vec []float64
	if err := json.Unmarshal([]byte(raw), &vec); err != nil {
		return nil, fmt.Errorf("invalid nearest parameter: expected JSON array of numbers")
	}
	if len(vec) == 0 {
		return nil, fmt.Errorf("nearest vector must not be empty")
	}
	if col.VectorDim > 0 && len(vec) != col.VectorDim {
		return nil, fmt.Errorf("dimension mismatch: query vector has %d dimensions, column %q expects %d",
			len(vec), col.Name, col.VectorDim)
	}
	return vec, nil
}

// resolveDistanceMetric validates and normalises the distance metric parameter.
// Returns "cosine" when the input is empty.
func resolveDistanceMetric(metric string) (string, error) {
	if metric == "" {
		return "cosine", nil
	}
	if _, err := vector.DistanceOperator(metric); err != nil {
		return "", err
	}
	return metric, nil
}

// handleNearest executes a vector nearest-neighbor query and writes the response.
// Called from handleList when the "nearest" query param is present.
func (h *Handler) handleNearest(w http.ResponseWriter, r *http.Request, tbl *schema.Table,
	nearestRaw, vectorColName, distanceParam string, limit int,
	filterSQL string, filterArgs []any) {

	// Resolve vector column.
	col, err := findVectorColumn(tbl, vectorColName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Parse and validate query vector.
	queryVec, err := parseNearestVector(nearestRaw, col)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate distance metric.
	metric, err := resolveDistanceMetric(distanceParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.executeNearestQuery(w, r, tbl, col, queryVec, metric, limit, filterSQL, filterArgs)
}

// executeNearestQuery builds and runs the nearest-neighbor SQL, writing the response.
// Shared by handleNearest (raw vector) and handleSemanticQuery (embedding result).
func (h *Handler) executeNearestQuery(w http.ResponseWriter, r *http.Request, tbl *schema.Table,
	col *schema.Column, queryVec []float64, metric string, limit int,
	filterSQL string, filterArgs []any) {
	items, err := h.executeVectorQuery(r, tbl, col, queryVec, metric, limit, filterSQL, filterArgs)
	if err != nil {
		if !mapPGError(w, err) {
			h.logger.Error("nearest query error", "error", err, "table", tbl.Name)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Page:       1,
		PerPage:    limit,
		TotalItems: len(items),
		TotalPages: 1,
		Items:      items,
	})
}
