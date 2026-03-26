package vault

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrInvalidSecretName = errors.New("invalid vault secret name")

var validSecretNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// NormalizeSecretName trims and validates a secret name for safe URL segment usage.
func NormalizeSecretName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidSecretName)
	}
	if strings.Contains(normalized, "..") {
		return "", fmt.Errorf("%w: name cannot contain '..'", ErrInvalidSecretName)
	}
	if !validSecretNamePattern.MatchString(normalized) {
		return "", fmt.Errorf("%w: name may only contain letters, numbers, underscore, hyphen, and dot", ErrInvalidSecretName)
	}
	return normalized, nil
}
