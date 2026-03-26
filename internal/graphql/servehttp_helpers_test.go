package graphql

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeGraphQLRequestRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("{"))
	w := httptest.NewRecorder()

	_, ok := decodeGraphQLRequest(w, req)
	if ok {
		t.Fatalf("expected invalid JSON decode to fail")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
