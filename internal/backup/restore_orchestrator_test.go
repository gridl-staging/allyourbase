package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRestorePlanner struct {
	plan *RestorePlan
	err  error
}

func (f *fakeRestorePlanner) ValidateWindow(ctx context.Context, projectID, databaseID string, targetTime time.Time) (*RestorePlan, error) {
	_ = ctx
	_ = projectID
	_ = databaseID
	_ = targetTime
	if f.err != nil {
		return nil, f.err
	}
	return f.plan, nil
}

type fakeRestoreJobRepo struct {
	jobs           map[string]*RestoreJob
	nextID         int
	markFailedLogs []string
	updatePhaseErr map[string]error
}

func newFakeRestoreJobRepo() *fakeRestoreJobRepo {
	return &fakeRestoreJobRepo{jobs: map[string]*RestoreJob{}, nextID: 1}
}

func (f *fakeRestoreJobRepo) Create(ctx context.Context, job RestoreJob) (*RestoreJob, error) {
	_ = ctx
	id := fmt.Sprintf("job-%d", f.nextID)
	f.nextID++
	job.ID = id
	job.StartedAt = time.Now().UTC()
	clone := job
	f.jobs[id] = &clone
	return &clone, nil
}

func (f *fakeRestoreJobRepo) Get(ctx context.Context, id string) (*RestoreJob, error) {
	_ = ctx
	job, ok := f.jobs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	clone := *job
	return &clone, nil
}

func (f *fakeRestoreJobRepo) UpdatePhase(ctx context.Context, id, phase, status string) error {
	_ = ctx
	if err, ok := f.updatePhaseErr[phase]; ok {
		return err
	}
	job, ok := f.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	job.Phase = phase
	job.Status = status
	return nil
}

func (f *fakeRestoreJobRepo) SetBaseBackup(ctx context.Context, id, baseBackupID string, walSegmentsNeeded int) error {
	_ = ctx
	job, ok := f.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	job.BaseBackupID = baseBackupID
	job.WALSegmentsNeeded = walSegmentsNeeded
	return nil
}

func (f *fakeRestoreJobRepo) MarkCompleted(ctx context.Context, id string, verificationResult json.RawMessage) error {
	_ = ctx
	job, ok := f.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	job.Phase = RestorePhaseCompleted
	job.Status = RestoreStatusCompleted
	job.VerificationResult = append([]byte(nil), verificationResult...)
	now := time.Now().UTC()
	job.CompletedAt = &now
	return nil
}

func (f *fakeRestoreJobRepo) MarkFailed(ctx context.Context, id, errorMessage string) error {
	_ = ctx
	job, ok := f.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	job.Phase = RestorePhaseFailed
	job.Status = RestoreStatusFailed
	job.ErrorMessage = errorMessage
	now := time.Now().UTC()
	job.CompletedAt = &now
	f.markFailedLogs = append(f.markFailedLogs, errorMessage)
	return nil
}

func (f *fakeRestoreJobRepo) AppendLog(ctx context.Context, id, line string) error {
	_ = ctx
	job, ok := f.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	job.Logs += line
	return nil
}

func (f *fakeRestoreJobRepo) ListByProject(ctx context.Context, projectID, databaseID string) ([]RestoreJob, error) {
	_ = ctx
	var out []RestoreJob
	for _, job := range f.jobs {
		if job.ProjectID == projectID && job.DatabaseID == databaseID {
			out = append(out, *job)
		}
	}
	return out, nil
}

type fakeRestoreVerifier struct {
	result *RestoreVerification
	err    error
}

func (f *fakeRestoreVerifier) Verify(ctx context.Context) (*RestoreVerification, error) {
	_ = ctx
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func testRestorePlan() *RestorePlan {
	completed := time.Now().UTC().Add(-time.Hour)
	end := "0/2000000"
	return &RestorePlan{
		BaseBackup: &BackupRecord{
			ID:          "base-1",
			ObjectKey:   "projects/proj1/db/db1/base/2026/01/01/20260101T000000Z_0_1000000.tar.zst",
			CompletedAt: &completed,
			EndLSN:      &end,
		},
		WALSegments: []WALSegment{{Timeline: 1, SegmentName: "000000010000000000000002", SizeBytes: 16}},
	}
}

func newTestRecoveryInstance(stopCount *int) *RecoveryInstance {
	inst := NewRecoveryInstance("/tmp/unused", 55432, slog.Default())
	inst.runCommand = func(ctx context.Context, name string, args ...string) error {
		_ = ctx
		_ = name
		if len(args) > 0 && args[0] == "stop" {
			*stopCount = *stopCount + 1
		}
		return nil
	}
	inst.queryRecoveryFn = func(ctx context.Context, connURL string) (bool, error) {
		_ = ctx
		_ = connURL
		return false, nil
	}
	inst.waitTimeout = 100 * time.Millisecond
	inst.pollInterval = 5 * time.Millisecond
	return inst
}

func TestRestoreOrchestratorExecuteHappyPath(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{plan: testRestorePlan()}
	jobs := newFakeRestoreJobRepo()
	notify := &captureNotifier{}
	store := newFakeStore()
	cfg := PITRConfig{ShadowMode: false, EnvironmentClass: "prod", ArchivePrefix: "archive"}

	orch := NewRestoreOrchestrator(planner, jobs, store, notify, cfg, "postgres://primary", "archive", slog.Default())
	orch.extractBaseBackupFn = func(context.Context, Store, string, string) error { return nil }
	orch.downloadWALSegmentsFn = func(context.Context, Store, []WALSegment, string, string, string, string) error { return nil }
	orch.writeRecoveryConfigFn = func(string, time.Time, string) error { return nil }
	orch.findFreePortFn = func() (int, error) { return 55432, nil }

	stopCount := 0
	orch.newRecoveryInstanceFn = func(dataDir string, port int, logger *slog.Logger) *RecoveryInstance {
		_ = logger
		inst := newTestRecoveryInstance(&stopCount)
		inst.dataDir = dataDir
		inst.port = port
		return inst
	}
	orch.newVerifierFn = func(primaryDBURL, recoveryDBURL string) restoreVerifier {
		_ = primaryDBURL
		_ = recoveryDBURL
		return &fakeRestoreVerifier{result: &RestoreVerification{Passed: true}}
	}

	job, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC().Add(-10*time.Minute), "tester")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if job.Phase != RestorePhaseReadyForCutover || job.Status != RestoreStatusRunning {
		t.Fatalf("unexpected job state: phase=%s status=%s", job.Phase, job.Status)
	}
	if _, ok := orch.ActiveInstance(job.ID); !ok {
		t.Fatalf("active instance missing for job %s", job.ID)
	}
	if stopCount != 0 {
		t.Fatalf("unexpected stop calls on happy path: %d", stopCount)
	}
	if len(notify.events) != 0 {
		t.Fatalf("unexpected failure notifications: %+v", notify.events)
	}
}

func TestRestoreOrchestratorExecuteShadowModeRejected(t *testing.T) {
	t.Parallel()

	orch := NewRestoreOrchestrator(
		&fakeRestorePlanner{plan: testRestorePlan()},
		newFakeRestoreJobRepo(),
		newFakeStore(),
		&captureNotifier{},
		PITRConfig{ShadowMode: true},
		"postgres://primary",
		"archive",
		slog.Default(),
	)

	_, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC(), "tester")
	if err == nil || !strings.Contains(err.Error(), "shadow mode") {
		t.Fatalf("expected shadow mode rejection, got %v", err)
	}
}

func TestRestoreOrchestratorExecuteValidationFailure(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{err: errors.New("no completed physical backups")}
	jobs := newFakeRestoreJobRepo()
	notify := &captureNotifier{}
	orch := NewRestoreOrchestrator(planner, jobs, newFakeStore(), notify, PITRConfig{}, "postgres://primary", "archive", slog.Default())

	_, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC(), "tester")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(jobs.markFailedLogs) == 0 {
		t.Fatalf("expected failed job mark")
	}
	if len(notify.events) != 1 || notify.events[0].Stage != "restore" {
		t.Fatalf("expected one restore notification, got %+v", notify.events)
	}
}

func TestRestoreOrchestratorExecuteExtractionFailure(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{plan: testRestorePlan()}
	jobs := newFakeRestoreJobRepo()
	notify := &captureNotifier{}
	orch := NewRestoreOrchestrator(planner, jobs, newFakeStore(), notify, PITRConfig{}, "postgres://primary", "archive", slog.Default())
	orch.extractBaseBackupFn = func(context.Context, Store, string, string) error { return errors.New("extract failed") }

	_, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC(), "tester")
	if err == nil || !strings.Contains(err.Error(), "extract") {
		t.Fatalf("expected extraction error, got %v", err)
	}
	if len(notify.events) != 1 {
		t.Fatalf("expected one failure event, got %d", len(notify.events))
	}
}

func TestRestoreOrchestratorExecuteWALDownloadFailure(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{plan: testRestorePlan()}
	jobs := newFakeRestoreJobRepo()
	notify := &captureNotifier{}
	orch := NewRestoreOrchestrator(planner, jobs, newFakeStore(), notify, PITRConfig{}, "postgres://primary", "archive", slog.Default())
	orch.extractBaseBackupFn = func(context.Context, Store, string, string) error { return nil }
	orch.downloadWALSegmentsFn = func(context.Context, Store, []WALSegment, string, string, string, string) error {
		return errors.New("wal download failed")
	}

	_, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC(), "tester")
	if err == nil || !strings.Contains(err.Error(), "wal") {
		t.Fatalf("expected WAL download error, got %v", err)
	}
	if len(notify.events) != 1 {
		t.Fatalf("expected one failure event, got %d", len(notify.events))
	}
}

func TestRestoreOrchestratorExecuteVerificationFailureCleansUp(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{plan: testRestorePlan()}
	jobs := newFakeRestoreJobRepo()
	notify := &captureNotifier{}
	orch := NewRestoreOrchestrator(planner, jobs, newFakeStore(), notify, PITRConfig{}, "postgres://primary", "archive", slog.Default())
	orch.extractBaseBackupFn = func(context.Context, Store, string, string) error { return nil }
	orch.downloadWALSegmentsFn = func(context.Context, Store, []WALSegment, string, string, string, string) error { return nil }
	orch.writeRecoveryConfigFn = func(string, time.Time, string) error { return nil }
	orch.findFreePortFn = func() (int, error) { return 55432, nil }

	stopCount := 0
	orch.newRecoveryInstanceFn = func(dataDir string, port int, logger *slog.Logger) *RecoveryInstance {
		_ = logger
		inst := newTestRecoveryInstance(&stopCount)
		inst.dataDir = dataDir
		inst.port = port
		return inst
	}
	orch.newVerifierFn = func(primaryDBURL, recoveryDBURL string) restoreVerifier {
		_ = primaryDBURL
		_ = recoveryDBURL
		return &fakeRestoreVerifier{result: &RestoreVerification{Passed: false, SchemaCheck: RestoreCheckResult{Name: "schema", Passed: false, Details: "mismatch"}}}
	}

	_, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC(), "tester")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "verification") {
		t.Fatalf("expected verification failure, got %v", err)
	}
	if stopCount == 0 {
		t.Fatalf("expected recovery instance Stop to be called during cleanup")
	}
	if len(orch.activeJobs) != 0 {
		t.Fatalf("expected no active jobs after verification failure")
	}
	if len(notify.events) != 1 {
		t.Fatalf("expected one failure event, got %d", len(notify.events))
	}
}

func TestRestoreOrchestratorExecuteReadyForCutoverPhaseFailureCleansUp(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{plan: testRestorePlan()}
	jobs := newFakeRestoreJobRepo()
	jobs.updatePhaseErr = map[string]error{
		RestorePhaseReadyForCutover: errors.New("phase update failed"),
	}
	notify := &captureNotifier{}
	orch := NewRestoreOrchestrator(planner, jobs, newFakeStore(), notify, PITRConfig{}, "postgres://primary", "archive", slog.Default())
	orch.extractBaseBackupFn = func(context.Context, Store, string, string) error { return nil }
	orch.downloadWALSegmentsFn = func(context.Context, Store, []WALSegment, string, string, string, string) error { return nil }
	orch.writeRecoveryConfigFn = func(string, time.Time, string) error { return nil }
	orch.findFreePortFn = func() (int, error) { return 55432, nil }

	stopCount := 0
	orch.newRecoveryInstanceFn = func(dataDir string, port int, logger *slog.Logger) *RecoveryInstance {
		_ = logger
		inst := newTestRecoveryInstance(&stopCount)
		inst.dataDir = dataDir
		inst.port = port
		return inst
	}
	orch.newVerifierFn = func(primaryDBURL, recoveryDBURL string) restoreVerifier {
		_ = primaryDBURL
		_ = recoveryDBURL
		return &fakeRestoreVerifier{result: &RestoreVerification{Passed: true}}
	}

	tempRoot := t.TempDir()
	restoreTempDir := filepath.Join(tempRoot, "restore-tmp")
	orch.mkTempDirFn = func(dir, pattern string) (string, error) {
		_ = dir
		_ = pattern
		if err := os.MkdirAll(restoreTempDir, 0o700); err != nil {
			return "", err
		}
		return restoreTempDir, nil
	}

	removedTempDir := ""
	orch.removeAllFn = func(path string) error {
		removedTempDir = path
		return os.RemoveAll(path)
	}

	_, err := orch.Execute(context.Background(), "proj1", "db1", time.Now().UTC(), "tester")
	if err == nil || !strings.Contains(err.Error(), "ready_for_cutover") {
		t.Fatalf("expected ready_for_cutover phase update error, got %v", err)
	}
	if stopCount == 0 {
		t.Fatalf("expected recovery instance Stop to be called during cleanup")
	}
	if removedTempDir != restoreTempDir {
		t.Fatalf("expected temp dir cleanup for %q, got %q", restoreTempDir, removedTempDir)
	}
	if len(orch.activeJobs) != 0 {
		t.Fatalf("expected no active jobs after ready_for_cutover failure")
	}
	if len(jobs.markFailedLogs) == 0 {
		t.Fatalf("expected failed job mark")
	}
	if len(notify.events) != 1 {
		t.Fatalf("expected one failure event, got %d", len(notify.events))
	}
}

func TestRestoreOrchestratorAbandonActiveJob(t *testing.T) {
	t.Parallel()

	planner := &fakeRestorePlanner{plan: testRestorePlan()}
	jobs := newFakeRestoreJobRepo()
	orch := NewRestoreOrchestrator(planner, jobs, newFakeStore(), &captureNotifier{}, PITRConfig{}, "postgres://primary", "archive", slog.Default())

	stopCount := 0
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	walDir := filepath.Join(tempDir, "wal_archive")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.MkdirAll(walDir, 0o700); err != nil {
		t.Fatalf("mkdir wal: %v", err)
	}

	job, err := jobs.Create(context.Background(), RestoreJob{ProjectID: "proj1", DatabaseID: "db1"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	jobs.jobs[job.ID].Phase = RestorePhaseReadyForCutover
	jobs.jobs[job.ID].Status = RestoreStatusRunning

	inst := newTestRecoveryInstance(&stopCount)
	orch.activeJobs[job.ID] = &activeRestore{instance: inst, dataDir: dataDir, walArchiveDir: walDir}

	if err := orch.Abandon(context.Background(), job.ID); err != nil {
		t.Fatalf("Abandon: %v", err)
	}
	if stopCount == 0 {
		t.Fatalf("expected stop count > 0")
	}
	if _, ok := orch.ActiveInstance(job.ID); ok {
		t.Fatalf("active instance still present after abandon")
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("expected temp dir removed, stat err=%v", err)
	}
	if jobs.jobs[job.ID].Status != RestoreStatusFailed || jobs.jobs[job.ID].ErrorMessage != "abandoned" {
		t.Fatalf("job not marked abandoned/failed: %+v", jobs.jobs[job.ID])
	}
}
