//go:build integration

package sites

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

var (
	sharedPG      *testutil.PGContainer
	sharedCleanup func()
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	sharedCleanup = cleanup
	code := m.Run()
	sharedCleanup()
	os.Exit(code)
}

func setupService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	pool := sharedPG.Pool
	logger := testutil.DiscardLogger()

	runner := migrations.NewRunner(pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Clean slate for each test.
	_, _ = pool.Exec(ctx, "DELETE FROM _ayb_deploys")
	_, _ = pool.Exec(ctx, "DELETE FROM _ayb_sites")
	_, _ = pool.Exec(ctx, "DELETE FROM _ayb_custom_domains")

	return NewService(pool, logger)
}

func seedCustomDomain(t *testing.T, id, hostname string) {
	t.Helper()

	_, err := sharedPG.Pool.Exec(
		context.Background(),
		`INSERT INTO _ayb_custom_domains (id, hostname, environment, status, verification_token)
		 VALUES ($1, $2, 'production', 'active', 'service-test-token')`,
		id,
		hostname,
	)
	testutil.NoError(t, err)
}

func stringPtr(value string) *string {
	return &value
}

func TestCreateSite(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "My Site", "my-site", true, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "My Site", site.Name)
	testutil.Equal(t, "my-site", site.Slug)
	testutil.Equal(t, true, site.SPAMode)
	if site.ID == "" {
		t.Fatal("expected non-empty site ID")
	}
}

func TestCreateSiteDuplicateSlug(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.CreateSite(ctx, "First", "dup-slug", true, nil)
	testutil.NoError(t, err)

	_, err = svc.CreateSite(ctx, "Second", "dup-slug", false, nil)
	if err != ErrSiteSlugTaken {
		t.Fatalf("expected ErrSiteSlugTaken, got %v", err)
	}
}

func TestCreateSiteDuplicateCustomDomain(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	const customDomainID = "00000000-0000-0000-0000-000000000201"

	seedCustomDomain(t, customDomainID, "duplicate-create.example.com")

	_, err := svc.CreateSite(ctx, "First", "first-site", true, stringPtr(customDomainID))
	testutil.NoError(t, err)

	_, err = svc.CreateSite(ctx, "Second", "second-site", true, stringPtr(customDomainID))
	if err != ErrSiteCustomDomainTaken {
		t.Fatalf("expected ErrSiteCustomDomainTaken, got %v", err)
	}
}

func TestGetSiteNotFound(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.GetSite(ctx, "00000000-0000-0000-0000-000000000000")
	if err != ErrSiteNotFound {
		t.Fatalf("expected ErrSiteNotFound, got %v", err)
	}
}

func TestDeleteSiteNotFound(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	err := svc.DeleteSite(ctx, "00000000-0000-0000-0000-000000000000")
	if err != ErrSiteNotFound {
		t.Fatalf("expected ErrSiteNotFound, got %v", err)
	}
}

func TestPromoteDeployNoExistingLive(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Promote Test", "promote-test", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusUploading, deploy.Status)

	promoted, err := svc.PromoteDeploy(ctx, site.ID, deploy.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusLive, promoted.Status)
}

func TestPromoteDeploySupersedsPreviousLive(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Supersede Test", "supersede-test", true, nil)
	testutil.NoError(t, err)

	// First deploy goes live.
	d1, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	_, err = svc.PromoteDeploy(ctx, site.ID, d1.ID)
	testutil.NoError(t, err)

	// Second deploy: promoting it should supersede d1.
	d2, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	promoted, err := svc.PromoteDeploy(ctx, site.ID, d2.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusLive, promoted.Status)

	// Verify d1 is now superseded.
	d1After, err := svc.GetDeploy(ctx, site.ID, d1.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusSuperseded, d1After.Status)
}

func TestPromoteDeployInvalidTransition(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Invalid Transition", "invalid-transition", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)

	// Fail the deploy, then try to promote.
	_, err = svc.FailDeploy(ctx, site.ID, deploy.ID, "test error")
	testutil.NoError(t, err)

	_, err = svc.PromoteDeploy(ctx, site.ID, deploy.ID)
	if err == nil {
		t.Fatal("expected error promoting a failed deploy")
	}
}

func TestEnsureDeployUploadingRejectsPostPromoteConflict(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Upload Transition", "upload-transition", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)

	err = svc.EnsureDeployUploading(ctx, site.ID, deploy.ID)
	testutil.NoError(t, err)

	_, err = svc.PromoteDeploy(ctx, site.ID, deploy.ID)
	testutil.NoError(t, err)

	err = svc.EnsureDeployUploading(ctx, site.ID, deploy.ID)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition after promote, got %v", err)
	}
}

func TestRecordDeployFileUploadAccumulatesCounters(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Upload Counters", "upload-counters", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)

	updated, err := svc.RecordDeployFileUpload(ctx, site.ID, deploy.ID, 5)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, updated.FileCount)
	testutil.Equal(t, int64(5), updated.TotalBytes)

	updated, err = svc.RecordDeployFileUpload(ctx, site.ID, deploy.ID, 7)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, updated.FileCount)
	testutil.Equal(t, int64(12), updated.TotalBytes)
}

func TestRecordDeployFileUploadRejectsPostPromoteConflict(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Upload Conflict", "upload-conflict", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)

	_, err = svc.PromoteDeploy(ctx, site.ID, deploy.ID)
	testutil.NoError(t, err)

	_, err = svc.RecordDeployFileUpload(ctx, site.ID, deploy.ID, 1)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition after promote, got %v", err)
	}
}

func TestFailDeployRecordsMessage(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Fail Test", "fail-test", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)

	failed, err := svc.FailDeploy(ctx, site.ID, deploy.ID, "upload timed out")
	testutil.NoError(t, err)
	testutil.Equal(t, StatusFailed, failed.Status)
	if failed.ErrorMessage == nil || *failed.ErrorMessage != "upload timed out" {
		t.Fatalf("expected error message 'upload timed out', got %v", failed.ErrorMessage)
	}
}

func TestFailDeployRejectsInvalidTransition(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Already Live", "already-live", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	_, err = svc.PromoteDeploy(ctx, site.ID, deploy.ID)
	testutil.NoError(t, err)

	_, err = svc.FailDeploy(ctx, site.ID, deploy.ID, "should not apply")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestRollbackDeploy(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Rollback Test", "rollback-test", true, nil)
	testutil.NoError(t, err)

	// Create and promote first deploy.
	d1, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	_, err = svc.PromoteDeploy(ctx, site.ID, d1.ID)
	testutil.NoError(t, err)

	// Create and promote second deploy (d1 becomes superseded).
	d2, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	_, err = svc.PromoteDeploy(ctx, site.ID, d2.ID)
	testutil.NoError(t, err)

	// Rollback should re-promote d1.
	rolledBack, err := svc.RollbackDeploy(ctx, site.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, d1.ID, rolledBack.ID)
	testutil.Equal(t, StatusLive, rolledBack.Status)

	// d2 should now be superseded.
	d2After, err := svc.GetDeploy(ctx, site.ID, d2.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusSuperseded, d2After.Status)
}

func TestRollbackNoSupersededDeploy(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "No Rollback", "no-rollback", true, nil)
	testutil.NoError(t, err)

	_, err = svc.RollbackDeploy(ctx, site.ID)
	if err != ErrNoLiveDeploy {
		t.Fatalf("expected ErrNoLiveDeploy, got %v", err)
	}
}

func TestListSites(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.CreateSite(ctx, "Site A", "site-a", true, nil)
	testutil.NoError(t, err)
	_, err = svc.CreateSite(ctx, "Site B", "site-b", false, nil)
	testutil.NoError(t, err)

	result, err := svc.ListSites(ctx, 1, 10)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, result.TotalCount)
	if len(result.Sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(result.Sites))
	}
}

func TestListDeploysMissingSite(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	_, err := svc.ListDeploys(ctx, "00000000-0000-0000-0000-000000000000", 1, 10)
	if err != ErrSiteNotFound {
		t.Fatalf("expected ErrSiteNotFound, got %v", err)
	}
}

func TestUpdateSite(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Original", "update-test", true, nil)
	testutil.NoError(t, err)

	newName := "Updated"
	newSPA := false
	updated, err := svc.UpdateSite(ctx, site.ID, &newName, &newSPA, nil, false)
	testutil.NoError(t, err)
	testutil.Equal(t, "Updated", updated.Name)
	testutil.Equal(t, false, updated.SPAMode)
}

func TestUpdateSiteDuplicateCustomDomain(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	const customDomainID = "00000000-0000-0000-0000-000000000202"

	seedCustomDomain(t, customDomainID, "duplicate-update.example.com")

	_, err := svc.CreateSite(ctx, "First", "first-site", true, stringPtr(customDomainID))
	testutil.NoError(t, err)

	secondSite, err := svc.CreateSite(ctx, "Second", "second-site", true, nil)
	testutil.NoError(t, err)

	_, err = svc.UpdateSite(ctx, secondSite.ID, nil, nil, stringPtr(customDomainID), false)
	if err != ErrSiteCustomDomainTaken {
		t.Fatalf("expected ErrSiteCustomDomainTaken, got %v", err)
	}
}

func TestUpdateSitePreservesLiveDeployID(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	site, err := svc.CreateSite(ctx, "Live Site", "live-site", true, nil)
	testutil.NoError(t, err)

	deploy, err := svc.CreateDeploy(ctx, site.ID)
	testutil.NoError(t, err)

	_, err = svc.PromoteDeploy(ctx, site.ID, deploy.ID)
	testutil.NoError(t, err)

	newName := "Updated Live Site"
	updated, err := svc.UpdateSite(ctx, site.ID, &newName, nil, nil, false)
	testutil.NoError(t, err)
	testutil.Equal(t, "Updated Live Site", updated.Name)
	if updated.LiveDeployID == nil {
		t.Fatal("expected live deploy id to be returned")
	}
	testutil.Equal(t, deploy.ID, *updated.LiveDeployID)
}
