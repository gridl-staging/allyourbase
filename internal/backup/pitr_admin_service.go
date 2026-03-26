// Package backup PITRAdminService manages point-in-time restore operations and logs all restore and abandonment events to the audit trail.
package backup

import (
	"context"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
)

type PITRAdminService struct {
	planner      *RestorePlanner
	orchestrator *RestoreOrchestrator
	jobRepo      RestoreJobRepo
	auditLogger  *audit.AuditLogger
}

func NewPITRAdminService(
	planner *RestorePlanner,
	orchestrator *RestoreOrchestrator,
	jobRepo RestoreJobRepo,
	auditLogger *audit.AuditLogger,
) *PITRAdminService {
	return &PITRAdminService{
		planner:      planner,
		orchestrator: orchestrator,
		jobRepo:      jobRepo,
		auditLogger:  auditLogger,
	}
}

func (s *PITRAdminService) ValidateWindow(ctx context.Context, projectID, databaseID string, targetTime time.Time) (*RestorePlan, error) {
	return s.planner.ValidateWindow(ctx, projectID, databaseID, targetTime)
}

func (s *PITRAdminService) Restore(ctx context.Context, projectID, databaseID string, targetTime time.Time, requestedBy string) (*RestoreJob, error) {
	job, err := s.orchestrator.Execute(ctx, projectID, databaseID, targetTime, requestedBy)
	if err != nil {
		s.logRestoreAudit(ctx, projectID, databaseID, targetTime, "", "failed")
		return nil, err
	}

	s.logRestoreAudit(ctx, projectID, databaseID, targetTime, job.ID, "success")
	return job, nil
}

func (s *PITRAdminService) GetJob(ctx context.Context, id string) (*RestoreJob, error) {
	return s.jobRepo.Get(ctx, id)
}

func (s *PITRAdminService) ListJobs(ctx context.Context, projectID, databaseID string) ([]RestoreJob, error) {
	return s.jobRepo.ListByProject(ctx, projectID, databaseID)
}

// AbandonJob marks a restore job as abandoned. It retrieves the job, cancels the restore operation via the orchestrator, and logs the state change in the audit trail.
func (s *PITRAdminService) AbandonJob(ctx context.Context, id string) error {
	job, err := s.jobRepo.Get(ctx, id)
	if err != nil {
		return err
	}

	oldPhase := job.Phase

	err = s.orchestrator.Abandon(ctx, id)
	if err != nil {
		return err
	}

	s.logAbandonAudit(ctx, id, oldPhase)
	return nil
}

// logRestoreAudit logs the result of a restore operation to the audit log. It records the project, database, target time, and result, and includes the job ID if provided.
func (s *PITRAdminService) logRestoreAudit(ctx context.Context, projectID, databaseID string, targetTime time.Time, jobID, result string) {
	if s.auditLogger == nil {
		return
	}

	newValues := map[string]string{
		"project_id":  projectID,
		"database_id": databaseID,
		"target_time": targetTime.Format(time.RFC3339),
		"result":      result,
	}
	if jobID != "" {
		newValues["job_id"] = jobID
	}

	entry := audit.AuditEntry{
		TableName: "_ayb_restore_jobs",
		RecordID:  jobID,
		Operation: "INSERT",
		NewValues: newValues,
	}
	_ = s.auditLogger.LogMutation(ctx, entry)
}

// logAbandonAudit logs the abandonment of a restore job to the audit log. It records the phase transition to RestorePhaseFailed with an error message indicating admin abandonment.
func (s *PITRAdminService) logAbandonAudit(ctx context.Context, jobID, oldPhase string) {
	if s.auditLogger == nil {
		return
	}

	entry := audit.AuditEntry{
		TableName: "_ayb_restore_jobs",
		RecordID:  jobID,
		Operation: "UPDATE",
		OldValues: map[string]string{
			"phase": oldPhase,
		},
		NewValues: map[string]string{
			"phase":         RestorePhaseFailed,
			"error_message": "abandoned by admin",
		},
	}
	_ = s.auditLogger.LogMutation(ctx, entry)
}
