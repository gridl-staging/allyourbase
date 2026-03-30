package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// buildColumnMap returns a set of valid column names for the table.
func buildColumnMap(tbl *schema.Table) map[string]bool {
	m := make(map[string]bool, len(tbl.Columns))
	for _, col := range tbl.Columns {
		m[col.Name] = true
	}
	return m
}

// schemaColumnNames returns column names in schema order.
func schemaColumnNames(tbl *schema.Table) []string {
	names := make([]string, len(tbl.Columns))
	for i, col := range tbl.Columns {
		names[i] = col.Name
	}
	return names
}

// filterRecordColumns returns a copy of record with only known columns.
func filterRecordColumns(record map[string]any, colMap map[string]bool) map[string]any {
	filtered := make(map[string]any, len(record))
	for k, v := range record {
		if colMap[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// parseCSVHeaders reads the CSV header row, validates columns against the schema,
// and returns the raw headers, the valid column names, and the csv.Reader for continued reading.
// The returned csv.Reader must be used for subsequent row reads (not a new reader on the
// same body) because csv.Reader internally buffers via bufio.Reader.
func parseCSVHeaders(tbl *schema.Table, body io.Reader) ([]string, []string, *csv.Reader, error) {
	csvReader := csv.NewReader(body)
	csvReader.LazyQuotes = true

	headers, err := csvReader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil, nil, fmt.Errorf("empty CSV: no header row found")
		}
		return nil, nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	if len(headers) == 0 {
		return nil, nil, nil, fmt.Errorf("empty CSV: no header columns")
	}

	// Strip UTF-8 BOM from first cell.
	headers[0] = strings.TrimPrefix(headers[0], "\xEF\xBB\xBF")

	// Trim whitespace and check for duplicates.
	seen := make(map[string]bool, len(headers))
	for i, h := range headers {
		headers[i] = strings.TrimSpace(h)
	}
	for _, h := range headers {
		if h == "" {
			continue
		}
		if seen[h] {
			return nil, nil, nil, fmt.Errorf("duplicate CSV header: %s", h)
		}
		seen[h] = true
	}

	// Build valid column list.
	colMap := buildColumnMap(tbl)
	var validCols []string
	for _, h := range headers {
		if colMap[h] {
			validCols = append(validCols, h)
		}
	}

	if len(validCols) == 0 {
		return nil, nil, nil, fmt.Errorf("no recognized columns in CSV headers")
	}

	return headers, validCols, csvReader, nil
}

// streamJSONRows reads a JSON array of objects from r, calling onRow for each valid record.
// Non-object items are recorded as errors in errs. Returns a non-nil error for structural
// problems (not an array, malformed JSON, etc.).
func streamJSONRows(r io.Reader, colMap map[string]bool, maxRows int, onRow func(row int, record map[string]any) error, errs *[]ImportRowError) error {
	dec := json.NewDecoder(r)

	// Expect opening bracket.
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("expected JSON array, got %T", tok)
	}

	rowNum := 0
	for dec.More() {
		rowNum++
		if rowNum > maxRows {
			return errors.New(importRowLimitMessage(maxRows))
		}

		// Try to decode each element as a JSON object.
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("JSON decode error at row %d: %w", rowNum, err)
		}

		var record map[string]any
		if err := json.Unmarshal(raw, &record); err != nil {
			// Not an object — could be a string, number, array, etc.
			*errs = append(*errs, ImportRowError{Row: rowNum, Message: "expected JSON object"})
			continue
		}

		// Filter to known columns.
		filtered := filterRecordColumns(record, colMap)
		if err := onRow(rowNum, filtered); err != nil {
			return err
		}
	}

	// Expect closing bracket.
	_, err = dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}
