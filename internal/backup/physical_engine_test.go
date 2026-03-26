package backup

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestPhysicalEngineRunSuccess(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStore()
	notify := &captureNotifier{}

	engine := NewPhysicalEngine(
		PITRConfig{ArchivePrefix: "pitr"},
		store,
		repo,
		&BaseBackupRunner{DBURL: "postgres://unused"},
		notify,
		"proj1",
		"db1",
		nil,
	)

	lsns := []string{"0/1000000", "0/2000000"}
	engine.lsnFn = func(context.Context) (string, error) {
		if len(lsns) == 0 {
			return "", fmt.Errorf("no LSN")
		}
		v := lsns[0]
		lsns = lsns[1:]
		return v, nil
	}
	engine.runBackup = func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("tar bytes")), nil
	}

	if err := engine.Run(context.Background(), "schedule"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rec, err := repo.Get(context.Background(), repo.nextID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.BackupType != "physical" {
		t.Fatalf("BackupType = %q; want physical", rec.BackupType)
	}
	if rec.Status != StatusCompleted {
		t.Fatalf("Status = %q; want completed", rec.Status)
	}
	if rec.StartLSN == nil || *rec.StartLSN != "0/1000000" {
		t.Fatalf("StartLSN = %v; want 0/1000000", rec.StartLSN)
	}
	if rec.EndLSN == nil || *rec.EndLSN != "0/2000000" {
		t.Fatalf("EndLSN = %v; want 0/2000000", rec.EndLSN)
	}
	if store.putCalled != 1 {
		t.Fatalf("putCalled = %d; want 1", store.putCalled)
	}
	if store.lastType != "application/zstd" {
		t.Fatalf("content type = %q; want application/zstd", store.lastType)
	}
	if len(notify.events) != 0 {
		t.Fatalf("expected no failure notification, got %+v", notify.events)
	}
}

func TestPhysicalEngineRunFailureUpdatesStatusAndNotifies(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStore()
	notify := &captureNotifier{}

	engine := NewPhysicalEngine(
		PITRConfig{ArchivePrefix: "pitr"},
		store,
		repo,
		&BaseBackupRunner{DBURL: "postgres://unused"},
		notify,
		"proj1",
		"db1",
		nil,
	)

	engine.lsnFn = func(context.Context) (string, error) { return "0/1000000", nil }
	engine.runBackup = func(context.Context) (io.ReadCloser, error) {
		return nil, fmt.Errorf("pg_basebackup failed")
	}

	err := engine.Run(context.Background(), "schedule")
	if err == nil {
		t.Fatal("expected error")
	}

	rec, getErr := repo.Get(context.Background(), repo.nextID)
	if getErr != nil {
		t.Fatalf("Get: %v", getErr)
	}
	if rec.Status != StatusFailed {
		t.Fatalf("Status = %q; want failed", rec.Status)
	}
	if rec.ErrorMessage == "" {
		t.Fatal("expected non-empty error message")
	}
	if len(notify.events) != 1 {
		t.Fatalf("expected 1 failure notification, got %d", len(notify.events))
	}
	if notify.events[0].Stage != "physical_backup" {
		t.Fatalf("notification stage = %q; want physical_backup", notify.events[0].Stage)
	}
}

func TestPhysicalEngineConcurrentRunGuard(t *testing.T) {
	engine := NewPhysicalEngine(
		PITRConfig{},
		newFakeStore(),
		newFakeRepo(),
		&BaseBackupRunner{DBURL: "postgres://unused"},
		NoopNotifier{},
		"proj1",
		"db1",
		nil,
	)

	engine.mu.Lock()
	engine.running = true
	engine.mu.Unlock()

	err := engine.Run(context.Background(), "schedule")
	if err == nil {
		t.Fatal("expected concurrent run guard error")
	}
	if !containsFold(err.Error(), "already in progress") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPhysicalEngineRunWithRecord(t *testing.T) {
	repo := newFakeRepo()
	store := newFakeStore()
	notify := &captureNotifier{}

	rec, err := repo.CreatePhysical(context.Background(), "proj1", "db1", "manual")
	if err != nil {
		t.Fatalf("CreatePhysical: %v", err)
	}

	engine := NewPhysicalEngine(
		PITRConfig{ArchivePrefix: "pitr"},
		store,
		repo,
		&BaseBackupRunner{DBURL: "postgres://unused"},
		notify,
		"proj1",
		"db1",
		nil,
	)
	lsns := []string{"0/3000000", "0/4000000"}
	engine.lsnFn = func(context.Context) (string, error) {
		v := lsns[0]
		lsns = lsns[1:]
		return v, nil
	}
	engine.runBackup = func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("tar bytes")), nil
	}

	if err := engine.RunWithRecord(context.Background(), rec); err != nil {
		t.Fatalf("RunWithRecord: %v", err)
	}

	got, err := repo.Get(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("Status = %q; want completed", got.Status)
	}
}
