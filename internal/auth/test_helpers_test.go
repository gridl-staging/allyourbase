package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestRequest creates an HTTP request for testing. Body can be a string or nil.
func newTestRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var req *http.Request
	switch v := body.(type) {
	case string:
		req = httptest.NewRequest(method, path, strings.NewReader(v))
		req.Header.Set("Content-Type", "application/json")
	case nil:
		req = httptest.NewRequest(method, path, nil)
	default:
		t.Fatalf("unsupported body type: %T", body)
		return nil
	}
	return req
}

// serveRequest serves a request through a handler and returns the response recorder.
func serveRequest(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
