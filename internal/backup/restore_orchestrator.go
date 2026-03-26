// Package backup RestoreOrchestrator coordinates physical PITR restores from validation through verification to the ready-for-cutover state. It manages base backup extraction, WAL replay, and recovery instance lifecycle.
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type restoreWindowPlanner interface {
	ValidateWindow(ctx context.Context, projectID, databaseID string, targetTime time.Time) (*RestorePlan, error)
}

type restoreVerifier interface {
	Verify(ctx context.Context) (*RestoreVerification, error)
}

type activeRestore struct {
	instance      *RecoveryInstance
	dataDir       string
	walArchiveDir string
}

type restoreExecutionState struct {
	job              *RestoreJob
	recoveryInstance *RecoveryInstance
	tempDir          string
	dataDir          string
	walArchiveDir    string
	cleanupNeeded    bool
}

// RestoreOrchestrator executes physical PITR restore workflows.
// RestoreOrchestrator orchestrates physical point-in-time restore operations, managing the complete workflow from validation through verification to ready-for-cutover state.
type RestoreOrchestrator struct {
	planner       restoreWindowPlanner
	jobRepo       RestoreJobRepo
	store         Store
	notify        Notifier
	cfg           PITRConfig
	primaryDBURL  string
	archivePrefix string
	logger        *slog.Logger

	mu         sync.Mutex
	activeJobs map[string]*activeRestore

	extractBaseBackupFn   func(ctx context.Context, store Store, objectKey string, targetDir string) error
	downloadWALSegmentsFn func(ctx context.Context, store Store, segments []WALSegment, archivePrefix, projectID, databaseID string, walArchiveDir string) error
	writeRecoveryConfigFn func(dataDir string, targetTime time.Time, walArchiveDir string) error
	findFreePortFn        func() (int, error)
	newRecoveryInstanceFn func(dataDir string, port int, logger *slog.Logger) *RecoveryInstance
	newVerifierFn         func(primaryDBURL, recoveryDBURL string) restoreVerifier
	mkTempDirFn           func(dir, pattern string) (string, error)
	removeAllFn           func(path string) error
	nowFn                 func() time.Time
}

// NewRestoreOrchestrator returns a RestoreOrchestrator with operation functions initialized to their standard implementations. It provides defaults for the notifier and logger if nil.
func NewRestoreOrchestrator(
	planner restoreWindowPlanner,
	jobRepo RestoreJobRepo,
	store Store,
	notify Notifier,
	cfg PITRConfig,
	primaryDBURL string,
	archivePrefix string,
	logger *slog.Logger,
) *RestoreOrchestrator {
	if notify == nil {
		notify = NoopNotifier{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	orchestrator := &RestoreOrchestrator{
		planner:       planner,
		jobRepo:       jobRepo,
		store:         store,
		notify:        notify,
		cfg:           cfg,
		primaryDBURL:  primaryDBURL,
		archivePrefix: archivePrefix,
		logger:        logger,
		activeJobs:    map[string]*activeRestore{},
	}
	orchestrator.extractBaseBackupFn = ExtractBaseBackup
	orchestrator.downloadWALSegmentsFn = DownloadWALSegments
	orchestrator.writeRecoveryConfigFn = WriteRecoveryConfig
	orchestrator.findFreePortFn = FindFreePort
	orchestrator.newRecoveryInstanceFn = NewRecoveryInstance
	orchestrator.newVerifierFn = func(primaryDBURL, recoveryDBURL string) restoreVerifier {
		return NewRestoreVerifier(primaryDBURL, recoveryDBURL)
	}
	orchestrator.mkTempDirFn = os.MkdirTemp
	orchestrator.removeAllFn = os.RemoveAll
	orchestrator.nowFn = func() time.Time { return time.Now().UTC() }
	return orchestrator
}

// Execute orchestrates a complete restore workflow: validates the restore window, extracts base backup, downloads WAL segments, starts a recovery instance, waits for WAL replay, verifies the restore, and transitions to ready-for-cutover state. On failure at any stage, it performs cleanup and marks the job as failed.
func (o *RestoreOrchestrator) Execute(ctx context.Context, projectID, databaseID string, targetTime time.Time, requestedBy string) (*RestoreJob, error) {
	if o.cfg.ShadowMode {
		return nil, fmt.Errorf("restore refused: shadow mode is active")
	}
	if o.planner == nil {
		return nil, fmt.Errorf("restore planner is not configured")
	}

	job, err := o.jobRepo.Create(ctx, RestoreJob{
		ProjectID:   projectID,
		DatabaseID:  databaseID,
		Environment: o.cfg.EnvironmentClass,
		TargetTime:  targetTime,
		Phase:       RestorePhasePending,
		Status:      RestoreStatusPending,
		RequestedBy: requestedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("creating restore job: %w", err)
	}

	state := &restoreExecutionState{
		job:           job,
		cleanupNeeded: true,
	}
	defer o.cleanupRestoreExecutionState(state)

	plan, err := o.runRestoreValidationPhase(ctx, state.job.ID, projectID, databaseID, targetTime)
	if err != nil {
		return nil, o.failRestoreExecution(ctx, state.job.ID, databaseID, err)
	}

	if err := o.runRestoreExecutionPhase(ctx, state, plan, projectID, databaseID, targetTime); err != nil {
		return nil, o.failRestoreExecution(ctx, state.job.ID, databaseID, err)
	}

	if err := o.runRestoreVerificationPhase(ctx, state.job.ID, state.recoveryInstance); err != nil {
		return nil, o.failRestoreExecution(ctx, state.job.ID, databaseID, err)
	}

	updatedJob, err := o.activateRestoreJob(ctx, state)
	if err != nil {
		o.logger.Warn("failed to re-read restore job after reaching ready_for_cutover", "job_id", state.job.ID, "error", err)
		state.job.Phase = RestorePhaseReadyForCutover
		state.job.Status = RestoreStatusRunning
		return state.job, nil
	}
	return updatedJob, nil
}

func (o *RestoreOrchestrator) cleanupRestoreExecutionState(state *restoreExecutionState) {
	if !state.cleanupNeeded {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if state.recoveryInstance != nil {
		_ = state.recoveryInstance.Stop(cleanupCtx)
	}
	if state.tempDir != "" {
		_ = o.removeAllFn(state.tempDir)
	}
}

func (o *RestoreOrchestrator) appendRestoreLog(ctx context.Context, jobID, line string) {
	_ = o.jobRepo.AppendLog(ctx, jobID, fmt.Sprintf("[%s] %s\n", o.nowFn().Format(time.RFC3339), line))
}

func (o *RestoreOrchestrator) failRestoreExecution(ctx context.Context, jobID, databaseID string, runErr error) error {
	o.appendRestoreLog(ctx, jobID, "failed: "+runErr.Error())
	_ = o.jobRepo.MarkFailed(ctx, jobID, runErr.Error())
	o.notify.OnFailure(ctx, FailureEvent{
		BackupID:  jobID,
		DBName:    databaseID,
		Stage:     "restore",
		Err:       runErr,
		Timestamp: o.nowFn(),
	})
	return runErr
}

// TODO: Document RestoreOrchestrator.runRestoreValidationPhase.
func (o *RestoreOrchestrator) runRestoreValidationPhase(
	ctx context.Context,
	jobID, projectID, databaseID string,
	targetTime time.Time,
) (*RestorePlan, error) {
	if err := o.jobRepo.UpdatePhase(ctx, jobID, RestorePhaseValidating, RestoreStatusRunning); err != nil {
		return nil, fmt.Errorf("updating phase to validating: %w", err)
	}
	o.appendRestoreLog(ctx, jobID, "phase transition: validating/running")

	plan, err := o.planner.ValidateWindow(ctx, projectID, databaseID, targetTime)
	if err != nil {
		return nil, fmt.Errorf("validating restore window: %w", err)
	}

	if err := o.jobRepo.SetBaseBackup(ctx, jobID, plan.BaseBackup.ID, len(plan.WALSegments)); err != nil {
		return nil, fmt.Errorf("setting base backup metadata: %w", err)
	}
	if err := o.jobRepo.UpdatePhase(ctx, jobID, RestorePhaseRestoring, RestoreStatusRunning); err != nil {
		return nil, fmt.Errorf("updating phase to restoring: %w", err)
	}
	o.appendRestoreLog(ctx, jobID, "phase transition: restoring/running")
	return plan, nil
}

// TODO: Document RestoreOrchestrator.runRestoreExecutionPhase.
func (o *RestoreOrchestrator) runRestoreExecutionPhase(
	ctx context.Context,
	state *restoreExecutionState,
	plan *RestorePlan,
	projectID, databaseID string,
	targetTime time.Time,
) error {
	tempDir, err := o.mkTempDirFn("", "ayb-restore-*")
	if err != nil {
		return fmt.Errorf("creating temporary restore directory: %w", err)
	}
	state.tempDir = tempDir
	state.dataDir = filepath.Join(tempDir, "data")
	state.walArchiveDir = filepath.Join(tempDir, "wal_archive")

	if err := os.MkdirAll(state.dataDir, 0o700); err != nil {
		return fmt.Errorf("creating temporary data directory: %w", err)
	}
	if err := os.MkdirAll(state.walArchiveDir, 0o700); err != nil {
		return fmt.Errorf("creating temporary WAL archive directory: %w", err)
	}

	if err := o.extractBaseBackupFn(ctx, o.store, plan.BaseBackup.ObjectKey, state.dataDir); err != nil {
		return fmt.Errorf("extracting base backup: %w", err)
	}
	if err := o.downloadWALSegmentsFn(ctx, o.store, plan.WALSegments, o.archivePrefix, projectID, databaseID, state.walArchiveDir); err != nil {
		return fmt.Errorf("downloading WAL segments: %w", err)
	}
	if err := o.writeRecoveryConfigFn(state.dataDir, targetTime, state.walArchiveDir); err != nil {
		return fmt.Errorf("writing recovery configuration: %w", err)
	}

	port, err := o.findFreePortFn()
	if err != nil {
		return fmt.Errorf("allocating recovery port: %w", err)
	}

	state.recoveryInstance = o.newRecoveryInstanceFn(state.dataDir, port, o.logger)
	if err := state.recoveryInstance.Start(ctx); err != nil {
		return fmt.Errorf("starting recovery instance: %w", err)
	}
	if err := state.recoveryInstance.WaitForRecovery(ctx); err != nil {
		return fmt.Errorf("waiting for WAL replay: %w", err)
	}
	return nil
}

// TODO: Document RestoreOrchestrator.runRestoreVerificationPhase.
func (o *RestoreOrchestrator) runRestoreVerificationPhase(ctx context.Context, jobID string, recoveryInstance *RecoveryInstance) error {
	if err := o.jobRepo.UpdatePhase(ctx, jobID, RestorePhaseVerifying, RestoreStatusRunning); err != nil {
		return fmt.Errorf("updating phase to verifying: %w", err)
	}
	o.appendRestoreLog(ctx, jobID, "phase transition: verifying/running")

	verifier := o.newVerifierFn(o.primaryDBURL, recoveryInstance.ConnURL())
	verification, err := verifier.Verify(ctx)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}
	if verification == nil || !verification.Passed {
		verificationJSON, _ := json.Marshal(verification)
		return fmt.Errorf("verification failed: %s", string(verificationJSON))
	}

	if err := o.jobRepo.UpdatePhase(ctx, jobID, RestorePhaseReadyForCutover, RestoreStatusRunning); err != nil {
		return fmt.Errorf("updating phase to ready_for_cutover: %w", err)
	}
	o.appendRestoreLog(ctx, jobID, "phase transition: ready_for_cutover/running")
	return nil
}

func (o *RestoreOrchestrator) activateRestoreJob(ctx context.Context, state *restoreExecutionState) (*RestoreJob, error) {
	state.cleanupNeeded = false
	o.mu.Lock()
	o.activeJobs[state.job.ID] = &activeRestore{
		instance:      state.recoveryInstance,
		dataDir:       state.dataDir,
		walArchiveDir: state.walArchiveDir,
	}
	o.mu.Unlock()
	return o.jobRepo.Get(ctx, state.job.ID)
}

// Abandon stops the active recovery instance, removes all temporary restore data, and marks the restore job as failed.
func (o *RestoreOrchestrator) Abandon(ctx context.Context, jobID string) error {
	o.mu.Lock()
	active, ok := o.activeJobs[jobID]
	if ok {
		delete(o.activeJobs, jobID)
	}
	o.mu.Unlock()
	if !ok {
		return fmt.Errorf("restore job %s is not active", jobID)
	}

	// Best-effort cleanup: collect errors but always attempt all steps,
	// especially MarkFailed which must run even if Stop or removeAll fails.
	var errs []error

	if active.instance != nil {
		if err := active.instance.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stopping active recovery instance: %w", err))
		}
	}

	cleanupRoot := filepath.Dir(active.dataDir)
	if cleanupRoot != "" {
		if err := o.removeAllFn(cleanupRoot); err != nil {
			errs = append(errs, fmt.Errorf("removing active restore directory: %w", err))
		}
	}

	if err := o.jobRepo.MarkFailed(ctx, jobID, "abandoned"); err != nil {
		errs = append(errs, fmt.Errorf("marking abandoned restore job failed: %w", err))
	}
	_ = o.jobRepo.AppendLog(ctx, jobID, fmt.Sprintf("[%s] abandoned\n", o.nowFn().Format(time.RFC3339)))

	if len(errs) > 0 {
		return fmt.Errorf("abandon job %s: %w", jobID, errors.Join(errs...))
	}
	return nil
}

func (o *RestoreOrchestrator) ActiveInstance(jobID string) (*RecoveryInstance, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	active, ok := o.activeJobs[jobID]
	if !ok {
		return nil, false
	}
	return active.instance, true
}

func (o *RestoreOrchestrator) ActiveJobIDs() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	ids := make([]string, 0, len(o.activeJobs))
	for id := range o.activeJobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
