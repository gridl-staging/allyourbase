package httputil

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// MaxBodySize is the maximum allowed request body size (1MB).
const MaxBodySize = 1 << 20

const baseDocURL = "https://allyourbase.io"

// DecodeJSON reads and decodes a JSON request body with size limiting.
// Writes a 400 error and returns false on failure.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// ExtractBearerToken extracts a Bearer token from the Authorization header.
// Returns the token and true if found, or empty string and false otherwise.
func ExtractBearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" || !strings.HasPrefix(header, "Bearer ") {
		return "", false
	}
	token := header[7:]
	if token == "" {
		return "", false
	}
	return token, true
}

// ErrorResponse is the standard error envelope for all AYB API errors.
type ErrorResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
	DocURL  string         `json:"doc_url,omitempty"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a standard error response.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{
		Code:    status,
		Message: message,
	})
}

// WriteErrorWithDocURL writes an error response with a documentation URL.
func WriteErrorWithDocURL(w http.ResponseWriter, status int, message, docURL string) {
	WriteJSON(w, status, ErrorResponse{
		Code:    status,
		Message: message,
		DocURL:  docURL,
	})
}

// WriteFieldError writes an error response with field-level validation detail.
func WriteFieldError(w http.ResponseWriter, status int, message string, field, fieldCode, fieldMsg string) {
	WriteJSON(w, status, ErrorResponse{
		Code:    status,
		Message: message,
		Data: map[string]any{
			field: map[string]string{
				"code":    fieldCode,
				"message": fieldMsg,
			},
		},
	})
}

// DocURL constructs a documentation URL from a path fragment.
// Example: DocURL("/guide/authentication") -> "https://allyourbase.io/guide/authentication"
func DocURL(path string) string {
	return baseDocURL + path
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsValidUUID returns true if s is a valid UUID string (any version, hex+hyphens).
func IsValidUUID(s string) bool {
	return uuidRe.MatchString(s)
}
