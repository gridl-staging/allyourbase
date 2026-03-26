package main

import (
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewTestPGConfigUsesIsolatedState(t *testing.T) {
	root := t.TempDir()

	cfg := newTestPGConfig(root, 15432, testutil.DiscardLogger())

	testutil.Equal(t, uint32(15432), cfg.Port)
	testutil.Equal(t, filepath.Join(root, "data"), cfg.DataDir)
	testutil.Equal(t, filepath.Join(root, "run"), cfg.RuntimeDir)
	testutil.Equal(t, filepath.Join(root, "pg.pid"), cfg.PIDFile)
}

func TestReplaceDatabaseInConnURL(t *testing.T) {
	t.Parallel()

	updatedURL, err := replaceDatabaseInConnURL(
		"postgresql://ayb:ayb@127.0.0.1:15432/ayb?sslmode=disable",
		"postgres",
	)
	testutil.NoError(t, err)
	testutil.Equal(t, "postgresql://ayb:ayb@127.0.0.1:15432/postgres?sslmode=disable", updatedURL)
}
