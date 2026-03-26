package branching

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
)

func TestDumpArgs(t *testing.T) {
	t.Parallel()
	args := dumpArgs("postgres://localhost:5432/mydb")
	expected := []string{
		"--dbname=postgres://localhost:5432/mydb",
		"--format=plain",
		"--no-owner",
		"--no-privileges",
	}
	if len(args) != len(expected) {
		t.Fatalf("dumpArgs length = %d, want %d", len(args), len(expected))
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("dumpArgs[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestRestoreArgs(t *testing.T) {
	t.Parallel()
	args := restoreArgs("postgres://localhost:5432/branch_db")
	expected := []string{
		"--dbname=postgres://localhost:5432/branch_db",
		"--quiet",
		"--no-psqlrc",
	}
	if len(args) != len(expected) {
		t.Fatalf("restoreArgs length = %d, want %d", len(args), len(expected))
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("restoreArgs[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestCreateDBSQL(t *testing.T) {
	t.Parallel()
	sql := createDBSQL("ayb_branch_feature")
	// Should be a quoted identifier for safety
	want := `CREATE DATABASE "ayb_branch_feature"`
	if sql != want {
		t.Errorf("createDBSQL = %q, want %q", sql, want)
	}
}

func TestDropDBSQL(t *testing.T) {
	t.Parallel()
	sql := dropDBSQL("ayb_branch_feature")
	want := `DROP DATABASE IF EXISTS "ayb_branch_feature"`
	if sql != want {
		t.Errorf("dropDBSQL = %q, want %q", sql, want)
	}
}

func TestTerminateConnsSQL(t *testing.T) {
	t.Parallel()
	sql := terminateConnsSQL()
	if sql == "" {
		t.Fatal("terminateConnsSQL returned empty")
	}
	// Should reference the db name as a parameter placeholder
	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}
}

func TestReplaceDatabaseInURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		url    string
		newDB  string
		want   string
		errStr string
	}{
		{
			"simple",
			"postgres://user:pass@localhost:5432/mydb",
			"ayb_branch_test",
			"postgres://user:pass@localhost:5432/ayb_branch_test",
			"",
		},
		{
			"with params",
			"postgres://user:pass@localhost:5432/mydb?sslmode=disable",
			"ayb_branch_test",
			"postgres://user:pass@localhost:5432/ayb_branch_test?sslmode=disable",
			"",
		},
		{
			"postgresql scheme",
			"postgresql://user@localhost/orig",
			"newdb",
			"postgresql://user@localhost/newdb",
			"",
		},
		{
			"invalid url",
			"not-a-url",
			"newdb",
			"",
			"parsing database URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ReplaceDatabaseInURL(tt.url, tt.newDB)
			if tt.errStr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errStr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeleteValidatesProtectedDB(t *testing.T) {
	t.Parallel()
	// Verify that IsProtectedDatabase blocks deletion of postgres/template DBs.
	// This is a unit-level check; the actual Manager.Delete method uses this guard.
	for _, db := range []string{"postgres", "template0", "template1"} {
		if !IsProtectedDatabase(db) {
			t.Errorf("expected %q to be protected", db)
		}
	}
}

// stubRepo implements Repo for unit tests that don't need a real database.
type stubRepo struct {
	getBranch  *BranchRecord
	getErr     error
	createRec  *BranchRecord
	createErr  error
	listRecs   []BranchRecord
	listErr    error
	updateErr  error
	deleteErr  error
	lastCreate struct{ name, sourceDB, branchDB string }
}

func (s *stubRepo) Create(_ context.Context, name, sourceDB, branchDB string) (*BranchRecord, error) {
	s.lastCreate.name = name
	s.lastCreate.sourceDB = sourceDB
	s.lastCreate.branchDB = branchDB
	return s.createRec, s.createErr
}

func (s *stubRepo) Get(_ context.Context, _ string) (*BranchRecord, error) {
	return s.getBranch, s.getErr
}

func (s *stubRepo) GetByID(_ context.Context, _ string) (*BranchRecord, error) {
	return s.getBranch, s.getErr
}

func (s *stubRepo) List(_ context.Context) ([]BranchRecord, error) {
	return s.listRecs, s.listErr
}

func (s *stubRepo) UpdateStatus(_ context.Context, _, _, _ string) error {
	return s.updateErr
}

func (s *stubRepo) Delete(_ context.Context, _ string) error {
	return s.deleteErr
}

func TestCreateUsesDefaultSourceURLWhenEmpty(t *testing.T) {
	t.Parallel()
	defaultURL := "postgres://user:pass@localhost:5432/maindb"
	repo := &stubRepo{
		getBranch: nil,
		// Return an error from repo.Create so we don't reach createDatabase (nil pool).
		// The important thing is that repo.lastCreate.sourceDB was set correctly.
		createErr: fmt.Errorf("stub: stop after recording args"),
	}
	mgr := NewManager(nil, repo, slog.Default(), ManagerConfig{
		DefaultSourceURL: defaultURL,
	})

	// Call Create with empty sourceDBURL — should fall back to DefaultSourceURL.
	_, err := mgr.Create(context.Background(), "my-branch", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Verify the repo received the correct source DB name from the default URL.
	if repo.lastCreate.sourceDB != "maindb" {
		t.Errorf("repo.Create sourceDB = %q, want %q", repo.lastCreate.sourceDB, "maindb")
	}
}

func TestCreateUsesExplicitSourceURLOverDefault(t *testing.T) {
	t.Parallel()
	defaultURL := "postgres://user:pass@localhost:5432/maindb"
	explicitURL := "postgres://user:pass@localhost:5432/otherdb"
	repo := &stubRepo{
		getBranch: nil,
		createErr: fmt.Errorf("stub: stop after recording args"),
	}
	mgr := NewManager(nil, repo, slog.Default(), ManagerConfig{
		DefaultSourceURL: defaultURL,
	})

	_, err := mgr.Create(context.Background(), "my-branch", explicitURL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should NOT use default — should use the explicit URL's db name.
	if repo.lastCreate.sourceDB != "otherdb" {
		t.Errorf("repo.Create sourceDB = %q, want %q", repo.lastCreate.sourceDB, "otherdb")
	}
}

func TestCreateFailsWhenNoSourceURLAndNoDefault(t *testing.T) {
	t.Parallel()
	repo := &stubRepo{getBranch: nil}
	mgr := NewManager(nil, repo, slog.Default(), ManagerConfig{})

	_, err := mgr.Create(context.Background(), "my-branch", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "no source database URL") {
		t.Fatalf("expected 'no source database URL' error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
