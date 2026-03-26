package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/backup"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// --- Fake PITR service ---

type fakePITRService struct {
	validatePlan *backup.RestorePlan
	validateErr  error
	restoreJob   *backup.RestoreJob
	restoreErr   error
	getJob       *backup.RestoreJob
	getJobErr    error
	listJobs     []backup.RestoreJob
	listJobsErr  error
	abandonErr   error
	restoreBy    string
}

func (f *fakePITRService) ValidateWindow(_ context.Context, projectID, databaseID string, targetTime time.Time) (*backup.RestorePlan, error) {
	if f.validateErr != nil {
		return nil, f.validateErr
	}
	return f.validatePlan, nil
}

func (f *fakePITRService) Restore(_ context.Context, projectID, databaseID string, targetTime time.Time, requestedBy string) (*backup.RestoreJob, error) {
	f.restoreBy = requestedBy
	if f.restoreErr != nil {
		return nil, f.restoreErr
	}
	return f.restoreJob, nil
}

func (f *fakePITRService) GetJob(_ context.Context, id string) (*backup.RestoreJob, error) {
	if f.getJobErr != nil {
		return nil, f.getJobErr
	}
	return f.getJob, nil
}

func (f *fakePITRService) ListJobs(_ context.Context, projectID, databaseID string) ([]backup.RestoreJob, error) {
	if f.listJobsErr != nil {
		return nil, f.listJobsErr
	}
	return f.listJobs, nil
}

func (f *fakePITRService) AbandonJob(_ context.Context, id string) error {
	return f.abandonErr
}

func pitrTestServer(svc pitrAdmin) *Server {
	s := &Server{pitrService: svc}
	r := chi.NewRouter()
	r.Route("/admin/backups/projects/{projectId}/pitr", func(r chi.Router) {
		r.Post("/validate", s.handlePITRValidate)
		r.Post("/restore", s.handlePITRRestore)
		r.Get("/jobs", s.handlePITRJobList)
	})
	r.Route("/admin/backups/restore-jobs", func(r chi.Router) {
		r.Get("/{jobId}", s.handlePITRJobGet)
		r.Delete("/{jobId}", s.handlePITRJobAbandon)
	})
	s.router = r
	return s
}

// --- Validate tests ---

func TestPITRValidateNilService(t *testing.T) {
	s := pitrTestServer(nil)
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/validate", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestPITRValidateMissingTargetTime(t *testing.T) {
	svc := &fakePITRService{}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]string{"database_id": "mydb"})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

func TestPITRValidateSuccess(t *testing.T) {
	now := time.Now()
	backupRecord := &backup.BackupRecord{
		ID:         "backup-123",
		DatabaseID: "mydb",
		ObjectKey:  "projects/proj1/db/mydb/base/2024/01/01/backup.tar.zst",
		Checksum:   "abc123",
		StartedAt:  now,
	}
	svc := &fakePITRService{
		validatePlan: &backup.RestorePlan{
			BaseBackup:          backupRecord,
			WALSegments:         []backup.WALSegment{{ID: "seg1"}, {ID: "seg2"}},
			EarliestRecoverable: now.Add(-24 * time.Hour),
			LatestRecoverable:   now,
			EstimatedWALBytes:   1024,
		},
	}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]interface{}{
		"target_time": now.Format(time.RFC3339),
		"database_id": "mydb",
	})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["wal_segments_count"] != float64(2) {
		t.Errorf("wal_segments_count = %v; want 2", resp["wal_segments_count"])
	}
}

func TestPITRValidateError(t *testing.T) {
	svc := &fakePITRService{
		validateErr: fmt.Errorf("no completed physical backups found"),
	}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]interface{}{
		"target_time": time.Now().Format(time.RFC3339),
		"database_id": "mydb",
	})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

// --- Restore tests ---

func TestPITRRestoreNilService(t *testing.T) {
	s := pitrTestServer(nil)
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/restore", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestPITRRestoreMissingTargetTime(t *testing.T) {
	svc := &fakePITRService{}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]string{"database_id": "mydb"})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

func TestPITRRestoreSuccess(t *testing.T) {
	svc := &fakePITRService{
		restoreJob: &backup.RestoreJob{
			ID:     "job-123",
			Status: "running",
			Phase:  "restoring",
		},
	}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]interface{}{
		"target_time": time.Now().Format(time.RFC3339),
		"database_id": "mydb",
	})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d; want 202", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["job_id"] != "job-123" {
		t.Errorf("job_id = %v; want job-123", resp["job_id"])
	}
}

func TestPITRRestoreRequestedByFromClaimsSubject(t *testing.T) {
	svc := &fakePITRService{
		restoreJob: &backup.RestoreJob{
			ID:     "job-123",
			Status: "running",
			Phase:  "restoring",
		},
	}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]interface{}{
		"target_time": time.Now().Format(time.RFC3339),
		"database_id": "mydb",
	})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	claims := &auth.Claims{}
	claims.Subject = "user-123"
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d; want 202", w.Code)
	}
	if svc.restoreBy != "user-123" {
		t.Fatalf("requestedBy = %q; want %q", svc.restoreBy, "user-123")
	}
}

func TestPITRRestoreDryRun(t *testing.T) {
	now := time.Now()
	backupRecord := &backup.BackupRecord{
		ID:         "backup-123",
		DatabaseID: "mydb",
		ObjectKey:  "projects/proj1/db/mydb/base/2024/01/01/backup.tar.zst",
		Checksum:   "abc123",
		StartedAt:  now,
	}
	svc := &fakePITRService{
		validatePlan: &backup.RestorePlan{
			BaseBackup:          backupRecord,
			WALSegments:         []backup.WALSegment{{ID: "seg1"}},
			EarliestRecoverable: now.Add(-24 * time.Hour),
			LatestRecoverable:   now,
			EstimatedWALBytes:   1024,
		},
	}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]interface{}{
		"target_time": now.Format(time.RFC3339),
		"database_id": "mydb",
		"dry_run":     true,
	})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["wal_segments_count"] != float64(1) {
		t.Errorf("wal_segments_count = %v; want 1", resp["wal_segments_count"])
	}
}

func TestPITRRestoreShadowMode(t *testing.T) {
	svc := &fakePITRService{
		restoreErr: fmt.Errorf("restore refused: shadow mode is active"),
	}
	s := pitrTestServer(svc)
	body, _ := json.Marshal(map[string]interface{}{
		"target_time": time.Now().Format(time.RFC3339),
		"database_id": "mydb",
	})
	req := httptest.NewRequest("POST", "/admin/backups/projects/proj1/pitr/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d; want 409", w.Code)
	}
}

// --- Job Get tests ---

func TestPITRJobGetNilService(t *testing.T) {
	s := pitrTestServer(nil)
	req := httptest.NewRequest("GET", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestPITRJobGetSuccess(t *testing.T) {
	now := time.Now()
	svc := &fakePITRService{
		getJob: &backup.RestoreJob{
			ID:        "job-123",
			Status:    "completed",
			Phase:     "completed",
			StartedAt: now,
		},
	}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != "job-123" {
		t.Errorf("id = %v; want job-123", resp["id"])
	}
}

func TestPITRJobGetNotFound(t *testing.T) {
	svc := &fakePITRService{
		getJobErr: pgx.ErrNoRows,
	}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

// --- Job List tests ---

func TestPITRJobListNilService(t *testing.T) {
	s := pitrTestServer(nil)
	req := httptest.NewRequest("GET", "/admin/backups/projects/proj1/pitr/jobs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestPITRJobListMissingDatabaseID(t *testing.T) {
	svc := &fakePITRService{}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups/projects/proj1/pitr/jobs", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", w.Code)
	}
}

func TestPITRJobListSuccess(t *testing.T) {
	svc := &fakePITRService{
		listJobs: []backup.RestoreJob{
			{ID: "job-1", Status: "completed"},
			{ID: "job-2", Status: "running"},
		},
	}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups/projects/proj1/pitr/jobs?database_id=mydb", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["count"] != float64(2) {
		t.Errorf("count = %v; want 2", resp["count"])
	}
}

func TestPITRJobListEmpty(t *testing.T) {
	svc := &fakePITRService{
		listJobs: []backup.RestoreJob{},
	}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups/projects/proj1/pitr/jobs?database_id=mydb", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["count"] != float64(0) {
		t.Errorf("count = %v; want 0", resp["count"])
	}
	jobs := resp["jobs"].([]any)
	if len(jobs) != 0 {
		t.Errorf("jobs length = %d; want 0", len(jobs))
	}
}

// --- Abandon tests ---

func TestPITRJobAbandonNilService(t *testing.T) {
	s := pitrTestServer(nil)
	req := httptest.NewRequest("DELETE", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestPITRJobAbandonSuccess(t *testing.T) {
	svc := &fakePITRService{}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("DELETE", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "abandoned" {
		t.Errorf("status = %v; want abandoned", resp["status"])
	}
}

func TestPITRJobAbandonNotFound(t *testing.T) {
	svc := &fakePITRService{
		abandonErr: fmt.Errorf("restore job not found or not active"),
	}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("DELETE", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}

func TestPITRJobAbandonNotFoundErrNoRows(t *testing.T) {
	svc := &fakePITRService{
		abandonErr: pgx.ErrNoRows,
	}
	s := pitrTestServer(svc)
	req := httptest.NewRequest("DELETE", "/admin/backups/restore-jobs/job-123", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", w.Code)
	}
}
