package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// --- test doubles ---

type fakeDumper struct {
	data string
	err  error
}

func (f *fakeDumper) Dump(_ context.Context, _ string, dst io.Writer) error {
	if f.err != nil {
		return f.err
	}
	_, err := io.WriteString(dst, f.data)
	return err
}

type fakeStore struct {
	putErr    error
	putCalled int
	objects   map[string][]byte
	lastType  string
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (f *fakeStore) PutObject(_ context.Context, key string, body io.Reader, _ int64, contentType string) error {
	f.putCalled++
	f.lastType = contentType
	if f.putErr != nil {
		return f.putErr
	}
	data, _ := io.ReadAll(body)
	f.objects[key] = data
	return nil
}

func (f *fakeStore) GetObject(_ context.Context, key string) (io.ReadCloser, int64, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, 0, fmt.Errorf("not found: %s", key)
	}
	return io.NopCloser(strings.NewReader(string(data))), int64(len(data)), nil
}

func (f *fakeStore) HeadObject(_ context.Context, key string) (int64, error) {
	data, ok := f.objects[key]
	if !ok {
		return 0, fmt.Errorf("not found: %s", key)
	}
	return int64(len(data)), nil
}

func (f *fakeStore) ListObjects(_ context.Context, _ string) ([]StoreObject, error) {
	var out []StoreObject
	for k, v := range f.objects {
		out = append(out, StoreObject{Key: k, Size: int64(len(v))})
	}
	return out, nil
}

func (f *fakeStore) DeleteObject(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

type fakeRepo struct {
	records      map[string]*BackupRecord
	createErr    error
	updateErr    error
	completedErr error
	nextID       string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{records: map[string]*BackupRecord{}, nextID: "test-backup-id"}
}

func (r *fakeRepo) Create(_ context.Context, dbName, triggeredBy string) (*BackupRecord, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	rec := &BackupRecord{
		ID:          r.nextID,
		DBName:      dbName,
		Status:      StatusRunning,
		TriggeredBy: triggeredBy,
		StartedAt:   time.Now(),
		BackupType:  "logical",
	}
	r.records[rec.ID] = rec
	return rec, nil
}

func (r *fakeRepo) CreatePhysical(_ context.Context, projectID, databaseID, triggeredBy string) (*BackupRecord, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	rec := &BackupRecord{
		ID:          r.nextID,
		DBName:      databaseID,
		ProjectID:   projectID,
		DatabaseID:  databaseID,
		Status:      StatusRunning,
		TriggeredBy: triggeredBy,
		StartedAt:   time.Now(),
		BackupType:  "physical",
	}
	r.records[rec.ID] = rec
	return rec, nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id, status, errMsg string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if rec, ok := r.records[id]; ok {
		rec.Status = status
		rec.ErrorMessage = errMsg
	}
	return nil
}

func (r *fakeRepo) MarkCompleted(_ context.Context, id, objectKey string, sizeBytes int64, checksum string, completedAt time.Time) error {
	if r.completedErr != nil {
		return r.completedErr
	}
	if rec, ok := r.records[id]; ok {
		rec.Status = StatusCompleted
		rec.ObjectKey = objectKey
		rec.SizeBytes = sizeBytes
		rec.Checksum = checksum
		rec.CompletedAt = &completedAt
	}
	return nil
}

func (r *fakeRepo) MarkPhysicalCompleted(_ context.Context, id, objectKey string, sizeBytes int64, checksum, startLSN, endLSN string, completedAt time.Time) error {
	if r.completedErr != nil {
		return r.completedErr
	}
	if rec, ok := r.records[id]; ok {
		rec.Status = StatusCompleted
		rec.ObjectKey = objectKey
		rec.SizeBytes = sizeBytes
		rec.Checksum = checksum
		rec.StartLSN = &startLSN
		rec.EndLSN = &endLSN
		rec.CompletedAt = &completedAt
	}
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id string) (*BackupRecord, error) {
	rec, ok := r.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return rec, nil
}

func (r *fakeRepo) List(_ context.Context, _ ListFilter) ([]BackupRecord, int, error) {
	var out []BackupRecord
	for _, rec := range r.records {
		out = append(out, *rec)
	}
	return out, len(out), nil
}

func (r *fakeRepo) CompletedByDB(_ context.Context, dbName string) ([]BackupRecord, error) {
	var out []BackupRecord
	for _, rec := range r.records {
		if rec.DBName == dbName && rec.Status == StatusCompleted {
			out = append(out, *rec)
		}
	}
	return out, nil
}

func (r *fakeRepo) RecordRestore(_ context.Context, dbName, sourceID, triggeredBy string) (string, error) {
	id := "restore-" + sourceID
	rec := &BackupRecord{
		ID:              id,
		DBName:          dbName,
		Status:          StatusRunning,
		TriggeredBy:     triggeredBy,
		RestoreSourceID: sourceID,
		StartedAt:       time.Now(),
	}
	r.records[id] = rec
	return id, nil
}

func (r *fakeRepo) ListPhysicalCompleted(_ context.Context, projectID, databaseID string) ([]BackupRecord, error) {
	var out []BackupRecord
	for _, rec := range r.records {
		if rec.ProjectID == projectID && rec.DatabaseID == databaseID &&
			rec.BackupType == "physical" && rec.Status == StatusCompleted {
			out = append(out, *rec)
		}
	}
	return out, nil
}

type captureNotifier struct {
	events      []FailureEvent
	alertEvents []AlertEvent
}

func (n *captureNotifier) OnFailure(_ context.Context, evt FailureEvent) {
	n.events = append(n.events, evt)
}

func (n *captureNotifier) OnAlert(_ context.Context, evt AlertEvent) {
	n.alertEvents = append(n.alertEvents, evt)
}

func newTestEngine(dumper Dumper, store Store, repo Repo, notify Notifier) *Engine {
	cfg := Config{Prefix: "backups", RetentionCount: 5}
	return NewEngine(cfg, store, repo, dumper, notify, slog.Default(), "mydb", "postgres://localhost/mydb")
}

// --- engine tests ---

func TestEngineRunSuccess(t *testing.T) {
	store := newFakeStore()
	engine := newTestEngine(&fakeDumper{data: "SELECT 1;"}, store, newFakeRepo(), &captureNotifier{})
	result := engine.Run(context.Background(), "test")

	if result.Status != StatusCompleted {
		t.Errorf("status = %q; want completed", result.Status)
	}
	if result.ObjectKey == "" {
		t.Error("expected non-empty object key")
	}
	if store.lastType != "application/gzip" {
		t.Errorf("content type = %q; want %q", store.lastType, "application/gzip")
	}
}

func TestEngineRunPGDumpFailure(t *testing.T) {
	dumper := &fakeDumper{err: fmt.Errorf("connection refused")}
	notify := &captureNotifier{}
	store := newFakeStore()

	engine := newTestEngine(dumper, store, newFakeRepo(), notify)
	result := engine.Run(context.Background(), "test")

	if result.Err == nil {
		t.Fatal("expected error")
	}
	if result.Status != StatusFailed {
		t.Errorf("status = %q; want failed", result.Status)
	}
	if store.putCalled != 0 {
		t.Error("expected no S3 upload on dump failure")
	}
	if len(notify.events) != 1 || notify.events[0].Stage != "backup" {
		t.Errorf("expected 1 backup failure event, got %+v", notify.events)
	}
}

func TestEngineRunS3UploadFailure(t *testing.T) {
	store := newFakeStore()
	store.putErr = fmt.Errorf("access denied")
	notify := &captureNotifier{}

	engine := newTestEngine(&fakeDumper{data: "SELECT 1;"}, store, newFakeRepo(), notify)
	result := engine.Run(context.Background(), "test")

	if result.Err == nil {
		t.Fatal("expected error")
	}
	if result.Status != StatusFailed {
		t.Errorf("status = %q; want failed", result.Status)
	}
	if len(notify.events) != 1 {
		t.Errorf("expected 1 failure event, got %d", len(notify.events))
	}
}

func TestEngineRunCreateRecordFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.createErr = fmt.Errorf("db down")

	engine := newTestEngine(&fakeDumper{data: "x"}, newFakeStore(), repo, NoopNotifier{})
	result := engine.Run(context.Background(), "test")
	if result.Err == nil {
		t.Fatal("expected error on create failure")
	}
}

func TestEngineSkipsOverlappingRun(t *testing.T) {
	engine := newTestEngine(&fakeDumper{data: "x"}, newFakeStore(), newFakeRepo(), NoopNotifier{})
	engine.mu.Lock()
	engine.running = true
	engine.mu.Unlock()
	defer func() {
		engine.mu.Lock()
		engine.running = false
		engine.mu.Unlock()
	}()

	result := engine.Run(context.Background(), "test")
	if result.Status != "skipped" {
		t.Errorf("expected skipped, got %q", result.Status)
	}
}

func TestEngineRunContextCancellation(t *testing.T) {
	// Create a dumper that blocks until context is cancelled.
	dumper := &blockingDumper{}
	notify := &captureNotifier{}

	engine := newTestEngine(dumper, newFakeStore(), newFakeRepo(), notify)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := engine.Run(ctx, "test")
	if result.Status != StatusFailed {
		t.Errorf("status = %q; want failed", result.Status)
	}
	if result.Err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestEngineRunMetadataWriteFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.completedErr = fmt.Errorf("metadata write timeout")

	// Engine should still succeed (metadata failure is logged, not fatal).
	engine := newTestEngine(&fakeDumper{data: "SELECT 1;"}, newFakeStore(), repo, NoopNotifier{})
	result := engine.Run(context.Background(), "test")

	if result.Status != StatusCompleted {
		t.Errorf("status = %q; want completed (metadata failure is non-fatal)", result.Status)
	}
	if result.ObjectKey == "" {
		t.Error("expected non-empty object key despite metadata failure")
	}
}

func TestEngineRunCompressionFailure(t *testing.T) {
	// A reader that fails mid-stream triggers a compression error.
	dumper := &failingMidStreamDumper{data: "partial data", failAfter: 5}
	notify := &captureNotifier{}

	engine := newTestEngine(dumper, newFakeStore(), newFakeRepo(), notify)
	result := engine.Run(context.Background(), "test")

	if result.Status != StatusFailed {
		t.Errorf("status = %q; want failed", result.Status)
	}
	if len(notify.events) != 1 {
		t.Errorf("expected 1 failure event, got %d", len(notify.events))
	}
}

func TestEngineRunWithRecord(t *testing.T) {
	store := newFakeStore()
	repo := newFakeRepo()
	rec := &BackupRecord{ID: "pre-created-id", DBName: "mydb", Status: StatusRunning}
	repo.records[rec.ID] = rec

	engine := newTestEngine(&fakeDumper{data: "SELECT 1;"}, store, repo, NoopNotifier{})
	result := engine.RunWithRecord(context.Background(), rec)

	if result.Status != StatusCompleted {
		t.Errorf("status = %q; want completed", result.Status)
	}
	if result.BackupID != "pre-created-id" {
		t.Errorf("backup_id = %q; want pre-created-id", result.BackupID)
	}
}

// --- additional test doubles ---

type blockingDumper struct{}

func (d *blockingDumper) Dump(ctx context.Context, _ string, _ io.Writer) error {
	<-ctx.Done()
	return ctx.Err()
}

type failingMidStreamDumper struct {
	data      string
	failAfter int
}

func (d *failingMidStreamDumper) Dump(_ context.Context, _ string, dst io.Writer) error {
	if d.failAfter > 0 && d.failAfter < len(d.data) {
		_, _ = io.WriteString(dst, d.data[:d.failAfter])
	}
	return fmt.Errorf("mid-stream dump failure")
}
