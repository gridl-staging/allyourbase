package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStorageUpload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("content-type = %s", r.Header.Get("Content-Type"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "obj_1",
			"bucket":      "avatars",
			"name":        "a.jpg",
			"size":        3,
			"contentType": "image/jpeg",
			"createdAt":   "2026-01-01T00:00:00Z",
			"updatedAt":   "2026-01-01T00:00:00Z",
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.Storage.Upload(context.Background(), "avatars", "a.jpg", []byte("abc"), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
}

func TestStorageDownload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	b, err := c.Storage.Download(context.Background(), "avatars", "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatalf("body = %q", string(b))
	}
}

func TestStorageList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{
				"id":          "obj_1",
				"bucket":      "avatars",
				"name":        "a.jpg",
				"size":        3,
				"contentType": "image/jpeg",
				"createdAt":   "2026-01-01T00:00:00Z",
				"updatedAt":   "2026-01-01T00:00:00Z",
			}},
			"totalItems": 1,
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Storage.List(context.Background(), "avatars", StorageListParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items len = %d", len(res.Items))
	}
}
