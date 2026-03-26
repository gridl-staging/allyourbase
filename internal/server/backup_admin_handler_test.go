package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/backup"
	"github.com/go-chi/chi/v5"
)

// --- Fake backup service ---

type fakeBackupService struct {
	records    []backup.BackupRecord
	total      int
	listErr    error
	triggerRes backup.RunResult
	triggerErr error
}

func (f *fakeBackupService) List(_ context.Context, filter backup.ListFilter) ([]backup.BackupRecord, int, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.records, f.total, nil
}

func (f *fakeBackupService) TriggerBackup(_ context.Context) (backup.RunResult, error) {
	return f.triggerRes, f.triggerErr
}

func backupTestServer(svc backupAdmin) *Server {
	s := &Server{backupService: svc}
	r := chi.NewRouter()
	r.Route("/admin/backups", func(r chi.Router) {
		r.Get("/", s.handleAdminBackupList)
		r.Post("/", s.handleAdminBackupTrigger)
	})
	s.router = r
	return s
}

// --- List tests ---

func TestAdminBackupListEmpty(t *testing.T) {
	s := backupTestServer(&fakeBackupService{total: 0})
	req := httptest.NewRequest("GET", "/admin/backups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(0) {
		t.Errorf("total = %v; want 0", resp["total"])
	}
}

func TestAdminBackupListWithRecords(t *testing.T) {
	now := time.Now()
	svc := &fakeBackupService{
		records: []backup.BackupRecord{
			{ID: "b1", DBName: "mydb", Status: "completed", StartedAt: now},
			{ID: "b2", DBName: "mydb", Status: "running", StartedAt: now},
		},
		total: 2,
	}
	s := backupTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups?status=completed&limit=10&offset=0", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(2) {
		t.Errorf("total = %v; want 2", resp["total"])
	}
	backups := resp["backups"].([]any)
	if len(backups) != 2 {
		t.Errorf("backups count = %d; want 2", len(backups))
	}
}

func TestAdminBackupListNilService(t *testing.T) {
	s := backupTestServer(nil)
	req := httptest.NewRequest("GET", "/admin/backups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (empty list)", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(0) {
		t.Errorf("total = %v; want 0", resp["total"])
	}
}

func TestAdminBackupListError(t *testing.T) {
	svc := &fakeBackupService{listErr: fmt.Errorf("db unreachable")}
	s := backupTestServer(svc)
	req := httptest.NewRequest("GET", "/admin/backups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500", w.Code)
	}
}

// --- Trigger tests ---

func TestAdminBackupTriggerSuccess(t *testing.T) {
	svc := &fakeBackupService{
		triggerRes: backup.RunResult{BackupID: "new-backup", Status: "running"},
	}
	s := backupTestServer(svc)
	req := httptest.NewRequest("POST", "/admin/backups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d; want 202", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["backup_id"] != "new-backup" {
		t.Errorf("backup_id = %v; want new-backup", resp["backup_id"])
	}
}

func TestAdminBackupTriggerNilService(t *testing.T) {
	s := backupTestServer(nil)
	req := httptest.NewRequest("POST", "/admin/backups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
}

func TestAdminBackupTriggerError(t *testing.T) {
	svc := &fakeBackupService{triggerErr: fmt.Errorf("engine unavailable")}
	s := backupTestServer(svc)
	req := httptest.NewRequest("POST", "/admin/backups", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500", w.Code)
	}
}
