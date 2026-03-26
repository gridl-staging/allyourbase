//go:build integration

package edgefunc_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	testPool = pg.Pool

	runner := migrations.NewRunner(testPool, testutil.DiscardLogger())
	if err := runner.Bootstrap(ctx); err != nil {
		panic("bootstrap migrations: " + err.Error())
	}
	if _, err := runner.Run(ctx); err != nil {
		panic("run migrations: " + err.Error())
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func cleanupFunctions(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM _ayb_edge_functions WHERE name LIKE 'test-%'")
	})
}

func TestStoreCreate(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-create",
		EntryPoint: "handler",
		Source:     "export default function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
		Timeout:    5 * time.Second,
		EnvVars:    map[string]string{"KEY": "value"},
		Public:     true,
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)
	testutil.True(t, created.ID.String() != "00000000-0000-0000-0000-000000000000", "should have a UUID")
	testutil.Equal(t, "test-create", created.Name)
	testutil.Equal(t, "handler", created.EntryPoint)
	testutil.Equal(t, fn.Source, created.Source)
	testutil.Equal(t, fn.CompiledJS, created.CompiledJS)
	testutil.Equal(t, 5*time.Second, created.Timeout)
	testutil.Equal(t, "value", created.EnvVars["KEY"])
	testutil.True(t, created.Public, "should be public")
	testutil.True(t, !created.CreatedAt.IsZero(), "should have CreatedAt")
	testutil.True(t, !created.UpdatedAt.IsZero(), "should have UpdatedAt")
}

func TestStoreCreate_DuplicateName(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-dup",
		EntryPoint: "handler",
		Source:     "export default function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
	}

	_, err := store.Create(ctx, fn)
	testutil.NoError(t, err)

	_, err = store.Create(ctx, fn)
	testutil.True(t, err != nil, "duplicate name should error")
	testutil.ErrorContains(t, err, "already exists")
}

func TestStoreCreate_NilEnvVars(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-nil-env",
		EntryPoint: "handler",
		Source:     "function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
		EnvVars:    nil,
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)
	testutil.True(t, created.EnvVars != nil, "nil env vars should be normalized to empty map")
}

func TestStoreGet(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-get",
		EntryPoint: "handler",
		Source:     "function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
		Timeout:    3 * time.Second,
		EnvVars:    map[string]string{"A": "1"},
		Public:     false,
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)

	got, err := store.Get(ctx, created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, got.ID)
	testutil.Equal(t, "test-get", got.Name)
	testutil.Equal(t, "handler", got.EntryPoint)
	testutil.Equal(t, fn.Source, got.Source)
	testutil.Equal(t, 3*time.Second, got.Timeout)
	testutil.Equal(t, "1", got.EnvVars["A"])
	testutil.False(t, got.Public)
}

func TestStoreGet_NotFound(t *testing.T) {
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	_, err := store.Get(ctx, [16]byte{}) // zero UUID
	testutil.True(t, err != nil, "should error for nonexistent ID")
	testutil.ErrorContains(t, err, "not found")
}

func TestStoreGetByName(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-getbyname",
		EntryPoint: "handler",
		Source:     "function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)

	got, err := store.GetByName(ctx, "test-getbyname")
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, got.ID)
	testutil.Equal(t, "test-getbyname", got.Name)
}

func TestStoreGetByName_NotFound(t *testing.T) {
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	_, err := store.GetByName(ctx, "nonexistent-function")
	testutil.True(t, err != nil, "should error for nonexistent name")
	testutil.ErrorContains(t, err, "not found")
}

func TestStoreList(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	// Clean slate for predictable counts.
	testPool.Exec(ctx, "DELETE FROM _ayb_edge_functions WHERE name LIKE 'test-%'")

	for _, name := range []string{"test-list-a", "test-list-b", "test-list-c"} {
		_, err := store.Create(ctx, &edgefunc.EdgeFunction{
			Name:       name,
			EntryPoint: "handler",
			Source:     "function handler(req) { return new Response('" + name + "'); }",
			CompiledJS: "function handler(req) { return new Response('" + name + "'); }",
		})
		testutil.NoError(t, err)
	}

	// Page 1 with perPage=2.
	fns, err := store.List(ctx, 1, 2)
	testutil.NoError(t, err)
	testutil.SliceLen(t, fns, 2)

	// Page 2 with perPage=2.
	fns, err = store.List(ctx, 2, 2)
	testutil.NoError(t, err)
	testutil.SliceLen(t, fns, 1)

	// Default pagination (page=0, perPage=0 → all results up to default limit).
	fns, err = store.List(ctx, 0, 0)
	testutil.NoError(t, err)
	testutil.True(t, len(fns) >= 3, "should list at least 3 functions, got %d", len(fns))
}

func TestStoreList_OrderByName(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	testPool.Exec(ctx, "DELETE FROM _ayb_edge_functions WHERE name LIKE 'test-%'")

	for _, name := range []string{"test-z-last", "test-a-first", "test-m-middle"} {
		_, err := store.Create(ctx, &edgefunc.EdgeFunction{
			Name:       name,
			EntryPoint: "handler",
			Source:     "function handler() {}",
			CompiledJS: "function handler() {}",
		})
		testutil.NoError(t, err)
	}

	fns, err := store.List(ctx, 0, 0)
	testutil.NoError(t, err)
	testutil.True(t, len(fns) >= 3, "should have at least 3 functions")

	// Verify alphabetical order.
	for i := 1; i < len(fns); i++ {
		testutil.True(t, fns[i-1].Name <= fns[i].Name,
			"expected %q <= %q", fns[i-1].Name, fns[i].Name)
	}
}

func TestStoreUpdate(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-update",
		EntryPoint: "handler",
		Source:     "function handler(req) { return new Response('v1'); }",
		CompiledJS: "function handler(req) { return new Response('v1'); }",
		Timeout:    5 * time.Second,
		Public:     false,
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)

	// Mutate fields.
	created.Source = "function handler(req) { return new Response('v2'); }"
	created.CompiledJS = "function handler(req) { return new Response('v2'); }"
	created.Timeout = 10 * time.Second
	created.Public = true
	created.EnvVars = map[string]string{"NEW_KEY": "new_value"}
	created.EntryPoint = "main"

	updated, err := store.Update(ctx, created)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, updated.ID)
	testutil.Equal(t, "test-update", updated.Name)
	testutil.Equal(t, "main", updated.EntryPoint)
	testutil.Equal(t, created.Source, updated.Source)
	testutil.Equal(t, created.CompiledJS, updated.CompiledJS)
	testutil.Equal(t, 10*time.Second, updated.Timeout)
	testutil.True(t, updated.Public, "should be public after update")
	testutil.Equal(t, "new_value", updated.EnvVars["NEW_KEY"])
	testutil.True(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt),
		"updated_at should be >= created_at")
}

func TestStoreUpdate_NotFound(t *testing.T) {
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-update-ghost",
		EntryPoint: "handler",
		Source:     "function handler() {}",
		CompiledJS: "function handler() {}",
	}

	_, err := store.Update(ctx, fn)
	testutil.True(t, err != nil, "update nonexistent should error")
	testutil.ErrorContains(t, err, "not found")
}

func TestStoreDelete(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-delete",
		EntryPoint: "handler",
		Source:     "function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)

	err = store.Delete(ctx, created.ID)
	testutil.NoError(t, err)

	_, err = store.Get(ctx, created.ID)
	testutil.True(t, err != nil, "should not find deleted function")
	testutil.ErrorContains(t, err, "not found")
}

func TestStoreDelete_NotFound(t *testing.T) {
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	err := store.Delete(ctx, [16]byte{})
	testutil.True(t, err != nil, "delete nonexistent should error")
	testutil.ErrorContains(t, err, "not found")
}

func TestStoreCreate_DefaultTimeout(t *testing.T) {
	cleanupFunctions(t)
	ctx := context.Background()
	store := edgefunc.NewPostgresStore(testPool)

	fn := &edgefunc.EdgeFunction{
		Name:       "test-default-timeout",
		EntryPoint: "handler",
		Source:     "function handler(req) { return new Response('ok'); }",
		CompiledJS: "function handler(req) { return new Response('ok'); }",
		Timeout:    0, // should use DB default of 5000ms
	}

	created, err := store.Create(ctx, fn)
	testutil.NoError(t, err)
	testutil.Equal(t, 5*time.Second, created.Timeout)
}
