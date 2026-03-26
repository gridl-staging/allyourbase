package matview

import (
	"fmt"
	"regexp"

	"github.com/allyourbase/ayb/internal/sqlutil"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateIdentifier ensures schema/view names are safe SQL identifiers.
func ValidateIdentifier(name string) error {
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidIdentifier, name)
	}
	return nil
}

func validateRefreshMode(mode RefreshMode) error {
	switch mode {
	case RefreshModeStandard, RefreshModeConcurrent:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidRefreshMode, mode)
	}
}

// BuildRefreshSQL builds a safe REFRESH MATERIALIZED VIEW statement.
func BuildRefreshSQL(schemaName, viewName string, mode RefreshMode) (string, error) {
	if err := ValidateIdentifier(schemaName); err != nil {
		return "", err
	}
	if err := ValidateIdentifier(viewName); err != nil {
		return "", err
	}
	if err := validateRefreshMode(mode); err != nil {
		return "", err
	}

	qualified := sqlutil.QuoteQualifiedName(schemaName, viewName)
	if mode == RefreshModeConcurrent {
		return "REFRESH MATERIALIZED VIEW CONCURRENTLY " + qualified, nil
	}
	return "REFRESH MATERIALIZED VIEW " + qualified, nil
}
