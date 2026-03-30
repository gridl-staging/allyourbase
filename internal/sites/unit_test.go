package sites

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// --- clampPagination ---

func TestClampPagination_Defaults(t *testing.T) {
	// Zero/negative inputs should clamp to page=1, perPage=defaultPerPage.
	page, perPage := clampPagination(0, 0)
	if page != 1 {
		t.Errorf("page: got %d, want 1", page)
	}
	if perPage != defaultPerPage {
		t.Errorf("perPage: got %d, want %d", perPage, defaultPerPage)
	}
}

func TestClampPagination_NegativeInputs(t *testing.T) {
	page, perPage := clampPagination(-5, -10)
	if page != 1 {
		t.Errorf("page: got %d, want 1", page)
	}
	if perPage != defaultPerPage {
		t.Errorf("perPage: got %d, want %d", perPage, defaultPerPage)
	}
}

func TestClampPagination_OverMaxPerPage(t *testing.T) {
	// perPage > 100 should clamp to defaultPerPage.
	page, perPage := clampPagination(3, 200)
	if page != 3 {
		t.Errorf("page: got %d, want 3", page)
	}
	if perPage != defaultPerPage {
		t.Errorf("perPage: got %d, want %d", perPage, defaultPerPage)
	}
}

func TestClampPagination_ValidInputsUnchanged(t *testing.T) {
	// Valid inputs within bounds should pass through unchanged.
	page, perPage := clampPagination(2, 50)
	if page != 2 {
		t.Errorf("page: got %d, want 2", page)
	}
	if perPage != 50 {
		t.Errorf("perPage: got %d, want 50", perPage)
	}
}

func TestClampPagination_BoundaryPerPage(t *testing.T) {
	// perPage=100 is the max allowed value — should pass through.
	_, perPage := clampPagination(1, 100)
	if perPage != 100 {
		t.Errorf("perPage: got %d, want 100", perPage)
	}

	// perPage=101 exceeds the max — should clamp to default.
	_, perPage = clampPagination(1, 101)
	if perPage != defaultPerPage {
		t.Errorf("perPage: got %d, want %d", perPage, defaultPerPage)
	}
}

// --- mapSiteWriteError ---

func TestMapSiteWriteError_SlugUnique(t *testing.T) {
	// Postgres unique constraint on slug should map to ErrSiteSlugTaken.
	pgErr := &pgconn.PgError{ConstraintName: "_ayb_sites_slug_unique"}
	err := mapSiteWriteError(pgErr)
	if !errors.Is(err, ErrSiteSlugTaken) {
		t.Errorf("got %v, want ErrSiteSlugTaken", err)
	}
}

func TestMapSiteWriteError_CustomDomainUnique(t *testing.T) {
	// Postgres unique constraint on custom_domain_id should map to ErrSiteCustomDomainTaken.
	pgErr := &pgconn.PgError{ConstraintName: "_ayb_sites_custom_domain_unique"}
	err := mapSiteWriteError(pgErr)
	if !errors.Is(err, ErrSiteCustomDomainTaken) {
		t.Errorf("got %v, want ErrSiteCustomDomainTaken", err)
	}
}

func TestMapSiteWriteError_UnknownConstraint(t *testing.T) {
	// An unrecognized Postgres constraint should return the original error.
	pgErr := &pgconn.PgError{ConstraintName: "some_other_constraint"}
	err := mapSiteWriteError(pgErr)
	if err != pgErr {
		t.Errorf("expected original error returned for unknown constraint")
	}
}

func TestMapSiteWriteError_NonPgError(t *testing.T) {
	// Non-pgconn errors should pass through unchanged.
	orig := errors.New("some random error")
	err := mapSiteWriteError(orig)
	if err != orig {
		t.Errorf("expected original error returned for non-PgError")
	}
}

// --- DeployStatus constants ---

func TestDeployStatusValues(t *testing.T) {
	// Verify status string values match what the SQL queries use as literals.
	// If these drift from the DB column CHECK constraint, queries will silently
	// return no rows instead of failing — this test catches that.
	if StatusUploading != "uploading" {
		t.Errorf("StatusUploading = %q, want %q", StatusUploading, "uploading")
	}
	if StatusLive != "live" {
		t.Errorf("StatusLive = %q, want %q", StatusLive, "live")
	}
	if StatusSuperseded != "superseded" {
		t.Errorf("StatusSuperseded = %q, want %q", StatusSuperseded, "superseded")
	}
	if StatusFailed != "failed" {
		t.Errorf("StatusFailed = %q, want %q", StatusFailed, "failed")
	}
}

// --- Sentinel error identity ---

func TestSentinelErrors_AreDistinct(t *testing.T) {
	// Guard against accidental sentinel aliasing — each sentinel must be its own
	// distinct error so callers using errors.Is get correct results.
	sentinels := map[string]error{
		"ErrSiteNotFound":          ErrSiteNotFound,
		"ErrSiteSlugTaken":         ErrSiteSlugTaken,
		"ErrSiteCustomDomainTaken": ErrSiteCustomDomainTaken,
		"ErrDeployNotFound":        ErrDeployNotFound,
		"ErrNoLiveDeploy":          ErrNoLiveDeploy,
		"ErrInvalidTransition":     ErrInvalidTransition,
	}
	seen := make(map[string]string)
	for name, err := range sentinels {
		msg := err.Error()
		if prev, ok := seen[msg]; ok {
			t.Errorf("sentinel %s has same message as %s: %q", name, prev, msg)
		}
		seen[msg] = name
	}
}
