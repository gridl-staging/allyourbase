package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["hello"] != "world" {
		t.Fatalf("expected world, got %q", body["hello"])
	}
}

func TestWriteJSONCustomStatus(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]int{"id": 1})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "not found")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected code 404, got %d", resp.Code)
	}
	if resp.Message != "not found" {
		t.Fatalf("expected 'not found', got %q", resp.Message)
	}
	if resp.Data != nil {
		t.Fatalf("expected nil data, got %v", resp.Data)
	}
}

func TestWriteFieldError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteFieldError(w, http.StatusConflict, "unique violation", "email", "unique", "already exists")

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected code 409, got %d", resp.Code)
	}
	if resp.Data == nil {
		t.Fatal("expected data to be non-nil")
	}
	emailField, ok := resp.Data["email"]
	if !ok {
		t.Fatal("expected 'email' key in data")
	}
	fieldMap, ok := emailField.(map[string]any)
	if !ok {
		t.Fatalf("expected map for field, got %T", emailField)
	}
	if fieldMap["code"] != "unique" {
		t.Fatalf("expected code 'unique', got %q", fieldMap["code"])
	}
	if fieldMap["message"] != "already exists" {
		t.Fatalf("expected message 'already exists', got %q", fieldMap["message"])
	}
}

func TestDecodeJSONValid(t *testing.T) {
	t.Parallel()
	body := `{"email":"test@example.com","name":"Test"}`
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	var data map[string]string
	ok := DecodeJSON(w, r, &data)
	if !ok {
		t.Fatal("expected DecodeJSON to return true")
	}
	if data["email"] != "test@example.com" {
		t.Fatalf("expected test@example.com, got %q", data["email"])
	}
}

func TestDecodeJSONInvalid(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/", strings.NewReader("{broken"))
	w := httptest.NewRecorder()

	var data map[string]string
	ok := DecodeJSON(w, r, &data)
	if ok {
		t.Fatal("expected DecodeJSON to return false for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDecodeJSONEmptyBody(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("POST", "/", strings.NewReader(""))
	w := httptest.NewRecorder()

	var data map[string]string
	ok := DecodeJSON(w, r, &data)
	if ok {
		t.Fatal("expected DecodeJSON to return false for empty body")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestExtractBearerTokenValid(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer mytoken123")
	token, ok := ExtractBearerToken(r)
	if !ok {
		t.Fatal("expected ok to be true")
	}
	if token != "mytoken123" {
		t.Fatalf("expected mytoken123, got %q", token)
	}
}

func TestExtractBearerTokenMissing(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	_, ok := ExtractBearerToken(r)
	if ok {
		t.Fatal("expected ok to be false for missing header")
	}
}

func TestExtractBearerTokenWrongScheme(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic abc123")
	_, ok := ExtractBearerToken(r)
	if ok {
		t.Fatal("expected ok to be false for non-Bearer scheme")
	}
}

func TestExtractBearerTokenEmptyToken(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	_, ok := ExtractBearerToken(r)
	if ok {
		t.Fatal("expected ok to be false for empty token after Bearer")
	}
}

func TestWriteErrorWithDocURL(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteErrorWithDocURL(w, http.StatusUnauthorized, "missing or invalid JWT", "https://allyourbase.io/errors/401")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected code 401, got %d", resp.Code)
	}
	if resp.Message != "missing or invalid JWT" {
		t.Fatalf("expected 'missing or invalid JWT', got %q", resp.Message)
	}
	if resp.DocURL != "https://allyourbase.io/errors/401" {
		t.Fatalf("expected doc_url 'https://allyourbase.io/errors/401', got %q", resp.DocURL)
	}
}

func TestWriteErrorOmitsDocURLWhenEmpty(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, "not found")

	// Verify doc_url is omitted from JSON (not present as empty string).
	var raw map[string]any
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, exists := raw["doc_url"]; exists {
		t.Fatal("expected doc_url to be omitted from JSON when empty")
	}
}

func TestWriteErrorDocURLInRawJSON(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteErrorWithDocURL(w, http.StatusBadRequest, "bad request", "https://allyourbase.io/errors/400")

	// Verify doc_url is present in raw JSON.
	var raw map[string]any
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	docURL, exists := raw["doc_url"]
	if !exists {
		t.Fatal("expected doc_url to be present in JSON")
	}
	if docURL != "https://allyourbase.io/errors/400" {
		t.Fatalf("expected doc_url 'https://allyourbase.io/errors/400', got %q", docURL)
	}
}

func TestIsValidUUID(t *testing.T) {
	t.Parallel()
	valid := []string{
		"00000000-0000-0000-0000-000000000000",
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"F47AC10B-58CC-4372-A567-0E02B2C3D479",
	}
	for _, s := range valid {
		if !IsValidUUID(s) {
			t.Errorf("expected %q to be valid UUID", s)
		}
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"nonexistent-id",
		"550e8400e29b41d4a716446655440000",      // no hyphens
		"550e8400-e29b-41d4-a716-44665544000",   // too short
		"550e8400-e29b-41d4-a716-4466554400000", // too long
		"gggggggg-gggg-gggg-gggg-gggggggggggg",  // not hex
	}
	for _, s := range invalid {
		if IsValidUUID(s) {
			t.Errorf("expected %q to be invalid UUID", s)
		}
	}
}

func TestMaxBodySizeConstant(t *testing.T) {
	t.Parallel()
	if MaxBodySize != 1<<20 {
		t.Fatalf("expected 1MB (1048576), got %d", MaxBodySize)
	}
}
