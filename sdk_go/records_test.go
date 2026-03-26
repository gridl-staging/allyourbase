package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecordsListQueryParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("perPage") != "10" || q.Get("skipTotal") != "true" {
			t.Fatalf("bad query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":      []map[string]any{{"id": "rec_1"}},
			"page":       1,
			"perPage":    10,
			"totalItems": 1,
			"totalPages": 1,
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.Records.List(context.Background(), "posts", ListParams{PerPage: 10, SkipTotal: true})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordsGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/collections/posts/rec_1" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec_1"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Records.Get(context.Background(), "posts", "rec_1", GetParams{})
	if err != nil {
		t.Fatal(err)
	}
	if res["id"] != "rec_1" {
		t.Fatalf("unexpected record")
	}
}

func TestRecordsCreate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec_1", "title": "ok"})
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	res, err := c.Records.Create(context.Background(), "posts", map[string]any{"title": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if res["id"] != "rec_1" {
		t.Fatalf("unexpected record: %+v", res)
	}
}

func TestRecordsUpdateDeleteBatch(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch step {
		case 0:
			if r.Method != http.MethodPatch {
				t.Fatalf("method=%s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rec_1", "title": "updated"})
		case 1:
			if r.Method != http.MethodDelete {
				t.Fatalf("method=%s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		case 2:
			if r.URL.Path != "/api/collections/posts/batch" {
				t.Fatalf("path=%s", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"index": 0, "status": 201, "body": map[string]any{"id": "rec_2"}}})
		}
		step++
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	updated, err := c.Records.Update(context.Background(), "posts", "rec_1", map[string]any{"title": "updated"})
	if err != nil {
		t.Fatal(err)
	}
	if updated["title"] != "updated" {
		t.Fatalf("bad update")
	}
	if err := c.Records.Delete(context.Background(), "posts", "rec_1"); err != nil {
		t.Fatal(err)
	}
	res, err := c.Records.Batch(context.Background(), "posts", []BatchOperation{{Method: "create", Body: map[string]any{"title": "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Status != 201 {
		t.Fatalf("bad batch result")
	}
}
