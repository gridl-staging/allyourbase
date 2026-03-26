//go:build integration

package backup

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests require:
//   - A running LocalStack instance (S3): LOCALSTACK_ENDPOINT env var
//   - A PostgreSQL database: TEST_DATABASE_URL env var
//
// Run with: go test -tags integration -count=1 ./internal/backup/

func localstackEndpoint(t *testing.T) string {
	t.Helper()
	ep := os.Getenv("LOCALSTACK_ENDPOINT")
	if ep == "" {
		t.Skip("LOCALSTACK_ENDPOINT not set — skipping integration test")
	}
	return ep
}

func testDBURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return url
}

func setupTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _ayb_backups (
			id                TEXT        NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
			db_name           TEXT        NOT NULL,
			object_key        TEXT        NOT NULL DEFAULT '',
			size_bytes        BIGINT      NOT NULL DEFAULT 0,
			checksum          TEXT        NOT NULL DEFAULT '',
			started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			completed_at      TIMESTAMPTZ,
			status            TEXT        NOT NULL DEFAULT 'pending',
			error_message     TEXT        NOT NULL DEFAULT '',
			triggered_by      TEXT        NOT NULL DEFAULT '',
			restore_source_id TEXT        NOT NULL DEFAULT '',
			backup_type       TEXT        NOT NULL DEFAULT 'logical',
			start_lsn         pg_lsn,
			end_lsn           pg_lsn,
			project_id        TEXT        NOT NULL DEFAULT '',
			database_id       TEXT        NOT NULL DEFAULT ''
		)`)
	if err != nil {
		t.Fatalf("creating _ayb_backups table: %v", err)
	}
	_, _ = pool.Exec(ctx, "DELETE FROM _ayb_backups")
}

func newTestS3Store(t *testing.T, endpoint, bucket string) *S3Store {
	t.Helper()
	store, err := NewS3Store(context.Background(), S3Config{
		Endpoint:  endpoint,
		Bucket:    bucket,
		Region:    "us-east-1",
		AccessKey: "test",
		SecretKey: "test",
	})
	if err != nil {
		t.Fatalf("creating S3Store: %v", err)
	}
	return store
}

func ensureBucket(t *testing.T, store *S3Store, bucket string) {
	t.Helper()
	_, err := store.client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Logf("create bucket %q: %v (may already exist)", bucket, err)
	}
}

// --- E2E backup lifecycle ---

func TestIntegrationBackupLifecycle(t *testing.T) {
	endpoint := localstackEndpoint(t)
	dbURL := testDBURL(t)
	ctx := context.Background()
	bucket := "test-backup-lifecycle"

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()
	setupTestDB(t, pool)

	store := newTestS3Store(t, endpoint, bucket)
	ensureBucket(t, store, bucket)

	repo := NewPgRepo(pool)
	dumper := &DumpRunner{}
	logger := slog.Default()
	notify := &captureNotifier{}

	cfg := Config{Prefix: "test", RetentionCount: 5}
	engine := NewEngine(cfg, store, repo, dumper, notify, logger, "testdb", dbURL)

	// 1. Run backup.
	result := engine.Run(ctx, "integration-test")
	if result.Status != StatusCompleted {
		t.Fatalf("backup status = %q; want completed; err = %v", result.Status, result.Err)
	}
	if result.BackupID == "" {
		t.Fatal("expected non-empty backup ID")
	}
	if result.ObjectKey == "" {
		t.Fatal("expected non-empty object key")
	}
	if result.SizeBytes <= 0 {
		t.Errorf("expected size > 0, got %d", result.SizeBytes)
	}

	// 2. Verify S3 object exists.
	size, err := store.HeadObject(ctx, result.ObjectKey)
	if err != nil {
		t.Fatalf("HeadObject(%q): %v", result.ObjectKey, err)
	}
	if size != result.SizeBytes {
		t.Errorf("S3 size = %d; metadata size = %d", size, result.SizeBytes)
	}

	// 3. Verify metadata row.
	rec, err := repo.Get(ctx, result.BackupID)
	if err != nil {
		t.Fatalf("Get(%q): %v", result.BackupID, err)
	}
	if rec.Status != StatusCompleted {
		t.Errorf("record status = %q; want completed", rec.Status)
	}
	if rec.ObjectKey != result.ObjectKey {
		t.Errorf("record object_key = %q; want %q", rec.ObjectKey, result.ObjectKey)
	}

	// 4. List backups.
	records, total, err := repo.List(ctx, ListFilter{Status: StatusCompleted, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total < 1 {
		t.Errorf("expected at least 1 backup in list, got %d", total)
	}
	found := false
	for _, r := range records {
		if r.ID == result.BackupID {
			found = true
		}
	}
	if !found {
		t.Error("backup not found in list results")
	}

	// 5. Download and verify decompressable.
	body, _, err := store.GetObject(ctx, result.ObjectKey)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	gr, err := DecompressReader(body)
	if err != nil {
		body.Close()
		t.Fatalf("DecompressReader: %v", err)
	}
	buf := make([]byte, 1024)
	n, _ := gr.Read(buf)
	gr.Close()
	body.Close()
	if n == 0 {
		t.Error("expected non-empty decompressed content")
	}

	// 6. Retention with count=5 and 1 backup → no deletions.
	retJob := NewRetentionJob(cfg, store, repo, NoopNotifier{}, logger)
	retResult, err := retJob.Run(ctx, "testdb", false)
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if len(retResult.Deleted) != 0 {
		t.Errorf("expected no deletions, got %d", len(retResult.Deleted))
	}
}

// --- S3 unreachable mid-run ---

func TestIntegrationS3UnreachableFailure(t *testing.T) {
	dbURL := testDBURL(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()
	setupTestDB(t, pool)

	// Point at a port that's not listening to simulate unreachable S3.
	store := newTestS3Store(t, "http://127.0.0.1:19999", "nonexistent-bucket")

	repo := NewPgRepo(pool)
	dumper := &DumpRunner{}
	logger := slog.Default()
	notify := &captureNotifier{}

	cfg := Config{Prefix: "test"}
	engine := NewEngine(cfg, store, repo, dumper, notify, logger, "testdb", dbURL)

	result := engine.Run(ctx, "integration-test")
	if result.Status != StatusFailed {
		t.Fatalf("expected failed status, got %q", result.Status)
	}

	// Verify metadata shows failed.
	if result.BackupID != "" {
		rec, err := repo.Get(ctx, result.BackupID)
		if err != nil {
			t.Fatalf("Get(%q): %v", result.BackupID, err)
		}
		if rec.Status != StatusFailed {
			t.Errorf("record status = %q; want failed", rec.Status)
		}
		if rec.ErrorMessage == "" {
			t.Error("expected non-empty error message")
		}
	}

	// Verify notification hook invoked exactly once.
	if len(notify.events) != 1 {
		t.Errorf("expected 1 failure notification, got %d", len(notify.events))
	}
	if len(notify.events) > 0 && notify.events[0].Stage != "backup" {
		t.Errorf("notification stage = %q; want backup", notify.events[0].Stage)
	}
}
