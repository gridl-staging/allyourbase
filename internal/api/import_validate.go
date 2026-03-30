package api

import (
	"fmt"
	"strings"
)

func parseImportMode(mode string) (string, error) {
	if mode == "" {
		return "full", nil
	}
	if mode != "full" && mode != "partial" {
		return "", fmt.Errorf("invalid mode: must be full or partial")
	}
	return mode, nil
}

func validateImportOnConflict(onConflict string) error {
	if onConflict != "" && onConflict != "skip" && onConflict != "update" {
		return fmt.Errorf("invalid on_conflict: must be skip or update")
	}
	return nil
}

func importContentType(contentType string) (string, error) {
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	if contentType != "text/csv" && contentType != "application/json" {
		return "", fmt.Errorf("unsupported content type: expected text/csv or application/json")
	}
	return contentType, nil
}

func importRowLimitMessage(maxRows int) string {
	return fmt.Sprintf("import row limit exceeded (max %d)", maxRows)
}
