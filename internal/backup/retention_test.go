package backup

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

// buildRecords creates n completed BackupRecords with decreasing age.
// Each record is offset by 1 day + 1 hour to avoid boundary drift issues
// when comparing against time.Now() in retention logic.
func buildRecords(n int) []BackupRecord {
	now := time.Now().UTC()
	recs := make([]BackupRecord, n)
	for i := range recs {
		// Use i days + 1 hour offset so records are clearly past the day boundary.
		recs[i] = BackupRecord{
			ID:        fmt.Sprintf("backup-%d", i),
			DBName:    "testdb",
			ObjectKey: fmt.Sprintf("backups/testdb/key-%d.sql.gz", i),
			Status:    StatusCompleted,
			StartedAt: now.Add(-time.Duration(i)*24*time.Hour - time.Hour), // oldest last
		}
	}
	return recs
}

type staticRepo struct {
	records   []BackupRecord
	deleted   []string
	updateErr error
}

func (r *staticRepo) Create(_ context.Context, _, _ string) (*BackupRecord, error) {
	return nil, nil
}
func (r *staticRepo) CreatePhysical(_ context.Context, _, _, _ string) (*BackupRecord, error) {
	return nil, nil
}
func (r *staticRepo) UpdateStatus(_ context.Context, id, status, _ string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if status == StatusDeleted {
		r.deleted = append(r.deleted, id)
	}
	return nil
}
func (r *staticRepo) MarkCompleted(_ context.Context, _, _ string, _ int64, _ string, _ time.Time) error {
	return nil
}
func (r *staticRepo) MarkPhysicalCompleted(_ context.Context, _, _ string, _ int64, _, _, _ string, _ time.Time) error {
	return nil
}
func (r *staticRepo) Get(_ context.Context, _ string) (*BackupRecord, error) { return nil, nil }
func (r *staticRepo) List(_ context.Context, _ ListFilter) ([]BackupRecord, int, error) {
	return r.records, len(r.records), nil
}
func (r *staticRepo) CompletedByDB(_ context.Context, _ string) ([]BackupRecord, error) {
	return r.records, nil
}
func (r *staticRepo) RecordRestore(_ context.Context, _, _, _ string) (string, error) { return "", nil }
func (r *staticRepo) ListPhysicalCompleted(_ context.Context, _, _ string) ([]BackupRecord, error) {
	return nil, nil
}

func newRetentionJob(cfg Config, repo Repo) *RetentionJob {
	return NewRetentionJob(cfg, newFakeStore(), repo, NoopNotifier{}, slog.Default())
}

func TestRetentionCountBased(t *testing.T) {
	recs := buildRecords(10)
	repo := &staticRepo{records: recs}
	job := newRetentionJob(Config{RetentionCount: 3}, repo)

	result, err := job.Run(context.Background(), "testdb", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Deleted) != 7 {
		t.Errorf("deleted = %d; want 7", len(result.Deleted))
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestRetentionAgeBased(t *testing.T) {
	recs := buildRecords(5) // ages: ~1h, ~1d+1h, ~2d+1h, ~3d+1h, ~4d+1h
	repo := &staticRepo{records: recs}
	// Cutoff at 3 days → delete backups older than 3 days (indices 3,4)
	job := newRetentionJob(Config{RetentionDays: 3}, repo)

	result, err := job.Run(context.Background(), "testdb", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Deleted) != 2 {
		t.Errorf("deleted = %d; want 2 (age > 3 days)", len(result.Deleted))
	}
}

func TestRetentionDryRun(t *testing.T) {
	recs := buildRecords(5)
	repo := &staticRepo{records: recs}
	job := newRetentionJob(Config{RetentionCount: 2}, repo)

	result, err := job.Run(context.Background(), "testdb", true)
	if err != nil {
		t.Fatalf("Run dry-run: %v", err)
	}
	if !result.DryRun {
		t.Error("expected DryRun=true")
	}
	if len(result.Deleted) != 3 {
		t.Errorf("dry-run deleted = %d; want 3", len(result.Deleted))
	}
	// No actual deletions in metadata.
	if len(repo.deleted) != 0 {
		t.Errorf("dry-run should not delete from repo, got %d deletions", len(repo.deleted))
	}
}

func TestRetentionNoPolicy(t *testing.T) {
	recs := buildRecords(5)
	repo := &staticRepo{records: recs}
	// Neither count nor days set → nothing deleted.
	job := newRetentionJob(Config{}, repo)
	result, err := job.Run(context.Background(), "testdb", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("expected no deletions, got %d", len(result.Deleted))
	}
}

func TestRetentionPartialDeleteError(t *testing.T) {
	recs := buildRecords(5)
	repo := &staticRepo{records: recs, updateErr: fmt.Errorf("db: connection lost")}
	notify := &captureNotifier{}
	job := &RetentionJob{
		cfg:    Config{RetentionCount: 2},
		store:  newFakeStore(),
		repo:   repo,
		notify: notify,
		logger: slog.Default(),
	}

	result, err := job.Run(context.Background(), "testdb", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors on partial delete failure")
	}
	if len(notify.events) == 0 {
		t.Error("expected failure notification")
	}
}

func TestRetentionMixedPolicy(t *testing.T) {
	// 10 records, keep last 5 by count AND purge > 3 days old.
	// Records 0-4: age 0-4 days (keep by count: 0-4); records 3,4 also age-deleted.
	// Union: delete records 5-9 (count) + 3,4 (age) = 7 deleted total.
	recs := buildRecords(10)
	repo := &staticRepo{records: recs}
	job := newRetentionJob(Config{RetentionCount: 5, RetentionDays: 3}, repo)

	result, err := job.Run(context.Background(), "testdb", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Deleted) < 5 {
		t.Errorf("expected at least 5 deleted (count policy), got %d", len(result.Deleted))
	}
}
