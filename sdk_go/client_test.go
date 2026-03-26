package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientNormalizesBaseURL(t *testing.T) {
	c := NewClient("https://api.example.com///")
	if c.baseURL != "https://api.example.com" {
		t.Fatalf("baseURL = %q", c.baseURL)
	}
}

func TestRequestInjectsBearer(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("tok", "ref")

	_, err := c.doJSON(context.Background(), http.MethodGet, "/test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth = %q", gotAuth)
	}
}

func TestRequestNormalizesError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Invalid email or password",
			"code":    "auth/invalid-credentials",
			"data":    map[string]any{"field": "email"},
			"doc_url": "https://docs.allyourbase.dev/errors/auth-invalid-credentials",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.doJSON(context.Background(), http.MethodGet, "/test", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("err type = %T", err)
	}
	if apiErr.Status != 401 || apiErr.Code != "auth/invalid-credentials" {
		t.Fatalf("unexpected err: %+v", apiErr)
	}
}
