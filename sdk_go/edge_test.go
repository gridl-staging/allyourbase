package allyourbase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEdgeInvoke(t *testing.T) {
	var gotPath string
	var gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("x-edge-version", "v1")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	resp, err := c.Edge.Invoke(context.Background(), "hello", EdgeInvokeRequest{Method: http.MethodPost, Body: []byte(`{"x":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/functions/v1/hello" || gotMethod != http.MethodPost {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.Headers.Get("x-edge-version") != "v1" {
		t.Fatalf("missing passthrough header")
	}
}
