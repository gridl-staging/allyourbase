// Package cli Provides error handling and message extraction for deploy provider API responses.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// DeployAPIError represents an HTTP error response from a deploy provider API.
type DeployAPIError struct {
	Provider   string
	StatusCode int
	Message    string
	Body       string
}

func (e *DeployAPIError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = strings.TrimSpace(e.Body)
	}
	if msg == "" {
		provider := strings.TrimSpace(e.Provider)
		if provider == "" {
			provider = "deploy"
		}
		return fmt.Sprintf("unknown %s api error", provider)
	}
	return fmt.Sprintf("%s API error (%d): %s", e.Provider, e.StatusCode, msg)
}

// IsDeployStatusCode reports whether err wraps a DeployAPIError with the given HTTP status code.
func IsDeployStatusCode(err error, statusCode int) bool {
	var apiErr *DeployAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == statusCode
	}
	return false
}

// buildDeployAPIError parses a provider API error body using the preferred
// message keys in order, then falls back to the raw body or HTTP status text.
func buildDeployAPIError(provider string, statusCode int, body []byte, messageKeys ...string) *DeployAPIError {
	trimmedBody := strings.TrimSpace(string(body))
	message := trimmedBody

	if extracted, found := extractDeployErrorMessage(body, messageKeys...); found {
		message = extracted
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return &DeployAPIError{
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
		Body:       trimmedBody,
	}
}

// extractDeployErrorMessage extracts an error message from a JSON response body by searching for the given message keys in order. It returns the first non-empty message value found and whether one of the preferred keys was found in the JSON.
func extractDeployErrorMessage(body []byte, messageKeys ...string) (string, bool) {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false
	}

	foundPreferredKey := false
	for _, key := range messageKeys {
		value, ok := parsed[key]
		if !ok || value == nil {
			continue
		}
		foundPreferredKey = true
		if message := strings.TrimSpace(fmt.Sprint(value)); message != "" {
			return message, true
		}
	}
	return "", foundPreferredKey
}
