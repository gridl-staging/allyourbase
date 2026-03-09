//go:build integration

package emailtemplates_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	testPool = pg.Pool

	// Run migrations to create the email templates table.
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

func TestStoreUpsert_Insert(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	})

	tpl, err := store.Upsert(ctx, "test.insert", "Subject {{.Name}}", "<p>Hello {{.Name}}</p>")
	testutil.NoError(t, err)
	testutil.True(t, tpl.ID != "", "should have a UUID ID")
	testutil.Equal(t, "test.insert", tpl.TemplateKey)
	testutil.Equal(t, "Subject {{.Name}}", tpl.SubjectTemplate)
	testutil.Equal(t, "<p>Hello {{.Name}}</p>", tpl.HTMLTemplate)
	testutil.True(t, tpl.Enabled, "should be enabled by default")
	testutil.True(t, !tpl.CreatedAt.IsZero(), "should have a CreatedAt")
	testutil.True(t, !tpl.UpdatedAt.IsZero(), "should have an UpdatedAt")
}

func TestStoreUpsert_Update(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	})

	// Insert first.
	_, err := store.Upsert(ctx, "test.update", "Original", "<p>Original</p>")
	testutil.NoError(t, err)

	// Update with same key.
	tpl, err := store.Upsert(ctx, "test.update", "Updated", "<p>Updated</p>")
	testutil.NoError(t, err)
	testutil.Equal(t, "Updated", tpl.SubjectTemplate)
	testutil.Equal(t, "<p>Updated</p>", tpl.HTMLTemplate)
}

func TestStoreUpsert_InvalidKey(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	_, err := store.Upsert(ctx, "INVALID", "Subject", "<p>Body</p>")
	testutil.True(t, err != nil, "invalid key should be rejected")
}

func TestStoreUpsert_ParseError(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	_, err := store.Upsert(ctx, "test.bad_syntax", "OK", "<p>{{.Name</p>")
	testutil.True(t, err != nil, "invalid HTML template syntax should be rejected")
}

func TestStoreGet(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	})

	_, err := store.Upsert(ctx, "test.get", "Subject", "<p>Body</p>")
	testutil.NoError(t, err)

	tpl, err := store.Get(ctx, "test.get")
	testutil.NoError(t, err)
	testutil.Equal(t, "test.get", tpl.TemplateKey)
	testutil.Equal(t, "Subject", tpl.SubjectTemplate)
}

func TestStoreGet_NotFound(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	_, err := store.Get(ctx, "test.nonexistent")
	testutil.True(t, err != nil, "should error for nonexistent key")
}

func TestStoreList(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	// Clean slate.
	testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	})

	_, err := store.Upsert(ctx, "test.list_a", "A", "<p>A</p>")
	testutil.NoError(t, err)
	_, err = store.Upsert(ctx, "test.list_b", "B", "<p>B</p>")
	testutil.NoError(t, err)

	templates, err := store.List(ctx)
	testutil.NoError(t, err)
	testutil.True(t, len(templates) >= 2, "should list at least 2 templates, got %d", len(templates))

	// Verify they're returned in key order.
	keys := make(map[string]bool, len(templates))
	for _, tpl := range templates {
		keys[tpl.TemplateKey] = true
	}
	testutil.True(t, keys["test.list_a"], "should contain test.list_a")
	testutil.True(t, keys["test.list_b"], "should contain test.list_b")
}

func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	})

	_, err := store.Upsert(ctx, "test.delete", "Subject", "<p>Body</p>")
	testutil.NoError(t, err)

	err = store.Delete(ctx, "test.delete")
	testutil.NoError(t, err)

	_, err = store.Get(ctx, "test.delete")
	testutil.True(t, err != nil, "should not find deleted template")
}

func TestStoreDelete_NotFound(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	err := store.Delete(ctx, "test.nonexistent")
	testutil.True(t, err != nil, "delete nonexistent should error")
}

func TestStoreSetEnabled(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	t.Cleanup(func() {
		testPool.Exec(ctx, "DELETE FROM _ayb_email_templates WHERE template_key LIKE 'test.%'")
	})

	_, err := store.Upsert(ctx, "test.toggle", "Subject", "<p>Body</p>")
	testutil.NoError(t, err)

	// Disable.
	err = store.SetEnabled(ctx, "test.toggle", false)
	testutil.NoError(t, err)

	tpl, err := store.Get(ctx, "test.toggle")
	testutil.NoError(t, err)
	testutil.True(t, !tpl.Enabled, "should be disabled")

	// Re-enable.
	err = store.SetEnabled(ctx, "test.toggle", true)
	testutil.NoError(t, err)

	tpl, err = store.Get(ctx, "test.toggle")
	testutil.NoError(t, err)
	testutil.True(t, tpl.Enabled, "should be re-enabled")
}

func TestStoreSetEnabled_NotFound(t *testing.T) {
	ctx := context.Background()
	store := emailtemplates.NewStore(testPool)

	err := store.SetEnabled(ctx, "test.nonexistent", true)
	testutil.True(t, err != nil, "toggle nonexistent should error")
}
