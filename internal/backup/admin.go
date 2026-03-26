package backup

import "context"

// AdminService provides the admin API surface for backup operations.
// It wraps Engine + Repo to satisfy the server.backupAdmin interface.
type AdminService struct {
	engine *Engine
	repo   Repo
}

// NewAdminService creates an AdminService.
func NewAdminService(engine *Engine, repo Repo) *AdminService {
	return &AdminService{engine: engine, repo: repo}
}

// List returns backup records matching the given filter.
func (s *AdminService) List(ctx context.Context, f ListFilter) ([]BackupRecord, int, error) {
	return s.repo.List(ctx, f)
}

// TriggerBackup starts an on-demand backup in a background goroutine and returns
// a pending result immediately. The backup ID is pre-created in the metadata table
// so the caller can track progress.
func (s *AdminService) TriggerBackup(ctx context.Context) (RunResult, error) {
	rec, err := s.repo.Create(ctx, s.engine.dbName, "api")
	if err != nil {
		return RunResult{}, err
	}

	// Run the backup pipeline in the background (detached from HTTP request context).
	go s.engine.RunWithRecord(context.Background(), rec)

	return RunResult{
		BackupID: rec.ID,
		Status:   StatusRunning,
	}, nil
}
