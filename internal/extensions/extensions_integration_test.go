//go:build integration

package extensions_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/extensions"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/stdlib"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestListExtensions(t *testing.T) {
	// The extensions service uses database/sql interfaces, so we convert
	// the pgx pool via stdlib.OpenDBFromPool (same as production code).
	sqlDB := stdlib.OpenDBFromPool(sharedPG.Pool)
	defer sqlDB.Close()

	svc := extensions.NewService(sqlDB)
	ctx := context.Background()

	// List should return available extensions without error.
	exts, err := svc.List(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(exts) > 0, "managed Postgres should report at least one available extension")

	// Verify that at least one well-known extension (plpgsql) is listed
	// since it's installed by default in every Postgres database.
	var foundPlpgsql bool
	for _, ext := range exts {
		if ext.Name == "plpgsql" {
			foundPlpgsql = true
			testutil.True(t, ext.Installed, "plpgsql should be installed by default")
			testutil.True(t, ext.Available, "plpgsql should be available")
			break
		}
	}
	testutil.True(t, foundPlpgsql, "plpgsql should be in the extension list")
}

func TestEnableAndDisableExtension(t *testing.T) {
	sqlDB := stdlib.OpenDBFromPool(sharedPG.Pool)
	defer sqlDB.Close()

	svc := extensions.NewService(sqlDB)
	ctx := context.Background()

	// Find an available but not installed extension to test with.
	// "tablefunc" and "fuzzystrmatch" are commonly available in bundled PG.
	exts, err := svc.List(ctx)
	testutil.NoError(t, err)

	var targetExt string
	candidates := []string{"tablefunc", "fuzzystrmatch", "pg_trgm", "unaccent"}
	for _, candidate := range candidates {
		for _, ext := range exts {
			if ext.Name == candidate && ext.Available && !ext.Installed {
				targetExt = candidate
				break
			}
		}
		if targetExt != "" {
			break
		}
	}

	if targetExt == "" {
		t.Skip("no suitable uninstalled extension found for enable/disable test")
	}

	// Enable the extension.
	err = svc.Enable(ctx, targetExt)
	testutil.NoError(t, err)

	// Verify it's now installed.
	exts, err = svc.List(ctx)
	testutil.NoError(t, err)
	var installed bool
	for _, ext := range exts {
		if ext.Name == targetExt {
			installed = ext.Installed
			break
		}
	}
	testutil.True(t, installed, "extension should be installed after Enable")

	// Disable the extension.
	err = svc.Disable(ctx, targetExt)
	testutil.NoError(t, err)

	// Verify it's no longer installed.
	exts, err = svc.List(ctx)
	testutil.NoError(t, err)
	for _, ext := range exts {
		if ext.Name == targetExt {
			testutil.False(t, ext.Installed, "extension should not be installed after Disable")
			break
		}
	}
}

func TestEnableInvalidExtension(t *testing.T) {
	sqlDB := stdlib.OpenDBFromPool(sharedPG.Pool)
	defer sqlDB.Close()

	svc := extensions.NewService(sqlDB)
	ctx := context.Background()

	// Enabling a non-existent extension should fail.
	err := svc.Enable(ctx, "nonexistent_fake_extension_xyz")
	testutil.Error(t, err)
}

func TestValidateExtensionNameBoundary(t *testing.T) {
	// ValidateExtensionName is a pure function; test boundary cases that
	// would be dangerous if they slipped through to SQL execution.
	testutil.Error(t, extensions.ValidateExtensionName(""))
	testutil.Error(t, extensions.ValidateExtensionName("1starts_with_digit"))
	testutil.Error(t, extensions.ValidateExtensionName("has space"))
	testutil.Error(t, extensions.ValidateExtensionName("has;semi"))
	testutil.NoError(t, extensions.ValidateExtensionName("valid_name"))
	testutil.NoError(t, extensions.ValidateExtensionName("pg-trgm"))
}
