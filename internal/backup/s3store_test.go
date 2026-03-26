package backup

import (
	"context"
	"io"
	"strings"
	"testing"
)

// fakeStore is defined in engine_test.go and satisfies Store.
// This file tests the Store interface contract via the fake.

func TestFakeStoreImplementsStore(t *testing.T) {
	var _ Store = newFakeStore()
}

func TestFakeStorePutAndGet(t *testing.T) {
	s := newFakeStore()
	ctx := context.Background()
	body := strings.NewReader("gzip data")

	if err := s.PutObject(ctx, "test/key.sql.gz", body, int64(body.Len()), "application/gzip"); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	rc, size, err := s.GetObject(ctx, "test/key.sql.gz")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer rc.Close()
	if size <= 0 {
		t.Errorf("expected size > 0, got %d", size)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != "gzip data" {
		t.Errorf("got %q; want %q", got, "gzip data")
	}
}

func TestFakeStoreHeadObject(t *testing.T) {
	s := newFakeStore()
	ctx := context.Background()
	s.PutObject(ctx, "k", strings.NewReader("hello"), 5, "application/gzip")

	size, err := s.HeadObject(ctx, "k")
	if err != nil {
		t.Fatalf("HeadObject: %v", err)
	}
	if size != 5 {
		t.Errorf("size = %d; want 5", size)
	}
}

func TestFakeStoreDeleteObject(t *testing.T) {
	s := newFakeStore()
	ctx := context.Background()
	s.PutObject(ctx, "k", strings.NewReader("x"), 1, "application/gzip")

	if err := s.DeleteObject(ctx, "k"); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
	if _, err := s.HeadObject(ctx, "k"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestFakeStoreListObjects(t *testing.T) {
	s := newFakeStore()
	ctx := context.Background()
	s.PutObject(ctx, "a/1.gz", strings.NewReader("x"), 1, "application/gzip")
	s.PutObject(ctx, "a/2.gz", strings.NewReader("y"), 1, "application/gzip")

	objs, err := s.ListObjects(ctx, "a/")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(objs) != 2 {
		t.Errorf("expected 2 objects, got %d", len(objs))
	}
}
