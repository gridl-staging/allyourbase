//go:build integration

package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestUserRunnerBootstrap(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())

	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)

	// Verify table exists.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = '_ayb_user_migrations')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "_ayb_user_migrations table should exist")
}

func TestUserRunnerBootstrapIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())

	err := runner.Bootstrap(ctx)
	testutil.NoError(t, err)
	err = runner.Bootstrap(ctx)
	testutil.NoError(t, err)
}

func TestUserRunnerUp(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	// Create two migration files.
	os.WriteFile(filepath.Join(dir, "20260201_create_posts.sql"), []byte(`
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT
		)
	`), 0o644)
	os.WriteFile(filepath.Join(dir, "20260202_create_comments.sql"), []byte(`
		CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			post_id INT REFERENCES posts(id),
			body TEXT NOT NULL
		)
	`), 0o644)

	applied, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, applied)

	// Verify tables exist.
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'posts')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "posts table should exist")

	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'comments')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "comments table should exist")
}

func TestUserRunnerUpIdempotent(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	os.WriteFile(filepath.Join(dir, "20260201_init.sql"), []byte(`
		CREATE TABLE items (id SERIAL PRIMARY KEY)
	`), 0o644)

	applied1, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, applied1)

	// Second run should apply zero.
	applied2, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, applied2)
}

func TestUserRunnerUpEmptyDir(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	applied, err := runner.Up(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, applied)
}

func TestUserRunnerUpRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	os.WriteFile(filepath.Join(dir, "20260201_good.sql"), []byte(`
		CREATE TABLE good_table (id SERIAL PRIMARY KEY)
	`), 0o644)
	os.WriteFile(filepath.Join(dir, "20260202_bad.sql"), []byte(`
		CREATE TABLE bad_table (id SERIAL PRIMARY KEY);
		INVALID SQL HERE;
	`), 0o644)

	applied, err := runner.Up(ctx)
	testutil.Equal(t, applied, 1) // First migration succeeded.
	testutil.NotNil(t, err)

	// Good table should exist, bad table should not (rolled back).
	var exists bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'good_table')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.True(t, exists, "good_table should exist")

	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'bad_table')").
		Scan(&exists)
	testutil.NoError(t, err)
	testutil.False(t, exists, "bad_table should not exist (rolled back)")
}

func TestUserRunnerStatus(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	dir := t.TempDir()
	runner := migrations.NewUserRunner(sharedPG.Pool, dir, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))

	// Create 3 migration files, apply 2.
	os.WriteFile(filepath.Join(dir, "20260201_a.sql"), []byte("CREATE TABLE a (id INT)"), 0o644)
	os.WriteFile(filepath.Join(dir, "20260202_b.sql"), []byte("CREATE TABLE b (id INT)"), 0o644)

	_, err := runner.Up(ctx)
	testutil.NoError(t, err)

	// Add a third file (not yet applied).
	os.WriteFile(filepath.Join(dir, "20260203_c.sql"), []byte("CREATE TABLE c (id INT)"), 0o644)

	status, err := runner.Status(ctx)
	testutil.NoError(t, err)
	testutil.SliceLen(t, status, 3)

	// First two should be applied.
	testutil.Equal(t, "20260201_a.sql", status[0].Name)
	testutil.NotNil(t, status[0].AppliedAt)

	testutil.Equal(t, "20260202_b.sql", status[1].Name)
	testutil.NotNil(t, status[1].AppliedAt)

	// Third should be pending.
	testutil.Equal(t, "20260203_c.sql", status[2].Name)
	testutil.Nil(t, status[2].AppliedAt)
}
