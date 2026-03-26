package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestS3StorePutObjectUsesConfiguredKMSKeyID(t *testing.T) {
	headersCh := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		headersCh <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store, err := NewS3Store(context.Background(), S3Config{
		Bucket:     "archive-bucket",
		Region:     "us-east-1",
		Endpoint:   server.URL,
		AccessKey:  "test-access-key",
		SecretKey:  "test-secret-key",
		Encryption: "aws:kms",
		KMSKeyID:   "alias/pitr-key",
	})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}

	body := "wal-segment"
	if err := store.PutObject(context.Background(), "wal/0001", strings.NewReader(body), int64(len(body)), "application/octet-stream"); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	headers := <-headersCh
	if got := headers.Get("X-Amz-Server-Side-Encryption"); got != "aws:kms" {
		t.Fatalf("X-Amz-Server-Side-Encryption = %q; want %q", got, "aws:kms")
	}
	if got := headers.Get("X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id"); got != "alias/pitr-key" {
		t.Fatalf("X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id = %q; want %q", got, "alias/pitr-key")
	}
}
