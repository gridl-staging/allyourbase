//go:build integration

package ai_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/ai"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
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

// resetAndMigrate drops and recreates the public schema, then runs all
// migrations so that AI tables exist for store tests.
func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	r := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, r.Bootstrap(ctx))
	_, err = r.Run(ctx)
	testutil.NoError(t, err)
}

// ---------------------------------------------------------------------------
// PgAssistantHistoryStore
// ---------------------------------------------------------------------------

func TestAssistantHistoryInsertAndList(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgAssistantHistoryStore(sharedPG.Pool)

	entry := ai.AssistantHistoryEntry{
		Mode:         ai.AssistantModeSQL,
		QueryText:    "Show all users",
		ResponseText: "SELECT * FROM users",
		SQL:          "SELECT * FROM users",
		Explanation:  "Selects all rows from users table",
		Provider:     "openai",
		Model:        "gpt-4",
		Status:       ai.AssistantStatusSuccess,
		DurationMs:   150,
		InputTokens:  50,
		OutputTokens: 30,
	}

	// Insert should populate ID and CreatedAt.
	inserted, err := store.Insert(ctx, entry)
	testutil.NoError(t, err)
	testutil.True(t, inserted.ID != uuid.Nil, "ID should be populated after insert")
	testutil.True(t, !inserted.CreatedAt.IsZero(), "CreatedAt should be populated")
	testutil.Equal(t, ai.AssistantModeSQL, inserted.Mode)
	testutil.Equal(t, "Show all users", inserted.QueryText)

	// List should return the entry.
	entries, total, err := store.List(ctx, ai.AssistantHistoryFilter{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total)
	testutil.Equal(t, 1, len(entries))
	testutil.Equal(t, inserted.ID, entries[0].ID)
}

func TestAssistantHistoryListFilterByMode(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgAssistantHistoryStore(sharedPG.Pool)

	// Insert entries with different modes.
	sqlEntry := ai.AssistantHistoryEntry{
		Mode: ai.AssistantModeSQL, QueryText: "SQL query",
		Provider: "openai", Model: "gpt-4", Status: ai.AssistantStatusSuccess,
	}
	rlsEntry := ai.AssistantHistoryEntry{
		Mode: ai.AssistantModeRLS, QueryText: "RLS query",
		Provider: "openai", Model: "gpt-4", Status: ai.AssistantStatusSuccess,
	}
	_, err := store.Insert(ctx, sqlEntry)
	testutil.NoError(t, err)
	_, err = store.Insert(ctx, rlsEntry)
	testutil.NoError(t, err)

	// Filter by SQL mode should return only 1.
	entries, total, err := store.List(ctx, ai.AssistantHistoryFilter{Mode: ai.AssistantModeSQL, Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total)
	testutil.Equal(t, 1, len(entries))
	testutil.Equal(t, ai.AssistantModeSQL, entries[0].Mode)

	// No filter returns both.
	entries, total, err = store.List(ctx, ai.AssistantHistoryFilter{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, total)
	testutil.Equal(t, 2, len(entries))
}

func TestAssistantHistoryListPagination(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgAssistantHistoryStore(sharedPG.Pool)

	// Insert 5 entries.
	for i := 0; i < 5; i++ {
		_, err := store.Insert(ctx, ai.AssistantHistoryEntry{
			Mode: ai.AssistantModeGeneral, QueryText: "query",
			Provider: "openai", Model: "gpt-4", Status: ai.AssistantStatusSuccess,
		})
		testutil.NoError(t, err)
	}

	// Page 1 of 3 items.
	entries, total, err := store.List(ctx, ai.AssistantHistoryFilter{Page: 1, PerPage: 3})
	testutil.NoError(t, err)
	testutil.Equal(t, 5, total)
	testutil.Equal(t, 3, len(entries))

	// Page 2 should return remaining 2.
	entries, total, err = store.List(ctx, ai.AssistantHistoryFilter{Page: 2, PerPage: 3})
	testutil.NoError(t, err)
	testutil.Equal(t, 5, total)
	testutil.Equal(t, 2, len(entries))
}

// ---------------------------------------------------------------------------
// PgLogStore
// ---------------------------------------------------------------------------

func TestLogStoreInsertAndList(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgLogStore(sharedPG.Pool)

	log := ai.CallLog{
		Provider:     "anthropic",
		Model:        "claude-3",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.003,
		DurationMs:   200,
		Status:       "success",
	}
	err := store.Insert(ctx, log)
	testutil.NoError(t, err)

	// List with no filters should return the inserted log.
	logs, total, err := store.List(ctx, ai.ListFilter{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total)
	testutil.Equal(t, 1, len(logs))
	testutil.Equal(t, "anthropic", logs[0].Provider)
	testutil.Equal(t, "claude-3", logs[0].Model)
	testutil.Equal(t, 100, logs[0].InputTokens)
	testutil.Equal(t, 50, logs[0].OutputTokens)
	testutil.Equal(t, "success", logs[0].Status)
}

func TestLogStoreListFilterByProvider(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgLogStore(sharedPG.Pool)

	// Insert logs from different providers.
	for _, provider := range []string{"anthropic", "openai", "anthropic"} {
		err := store.Insert(ctx, ai.CallLog{
			Provider: provider, Model: "test", InputTokens: 10,
			OutputTokens: 5, CostUSD: 0.001, DurationMs: 100, Status: "success",
		})
		testutil.NoError(t, err)
	}

	// Filter by provider.
	logs, total, err := store.List(ctx, ai.ListFilter{Provider: "anthropic", Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, total)
	testutil.Equal(t, 2, len(logs))
}

func TestLogStoreListFilterByStatus(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgLogStore(sharedPG.Pool)

	// Insert success and error logs.
	err := store.Insert(ctx, ai.CallLog{
		Provider: "openai", Model: "gpt-4", Status: "success",
		InputTokens: 10, OutputTokens: 5, DurationMs: 100,
	})
	testutil.NoError(t, err)
	err = store.Insert(ctx, ai.CallLog{
		Provider: "openai", Model: "gpt-4", Status: "error",
		ErrorMessage: "rate limit", InputTokens: 10, DurationMs: 50,
	})
	testutil.NoError(t, err)

	// Filter errors only.
	logs, total, err := store.List(ctx, ai.ListFilter{Status: "error", Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, total)
	testutil.Equal(t, 1, len(logs))
	testutil.Equal(t, "error", logs[0].Status)
}

func TestLogStoreUsageSummary(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgLogStore(sharedPG.Pool)

	now := time.Now().UTC()

	// Insert logs from two providers.
	for i := 0; i < 3; i++ {
		err := store.Insert(ctx, ai.CallLog{
			Provider: "anthropic", Model: "claude-3",
			InputTokens: 100, OutputTokens: 50, CostUSD: 0.003,
			DurationMs: 200, Status: "success",
		})
		testutil.NoError(t, err)
	}
	err := store.Insert(ctx, ai.CallLog{
		Provider: "openai", Model: "gpt-4",
		InputTokens: 200, OutputTokens: 100, CostUSD: 0.01,
		DurationMs: 300, Status: "error", ErrorMessage: "timeout",
	})
	testutil.NoError(t, err)

	// Usage summary over a wide range covering all logs.
	summary, err := store.UsageSummary(ctx, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	testutil.NoError(t, err)
	testutil.Equal(t, 4, summary.TotalCalls)
	testutil.Equal(t, 500, summary.TotalInputTokens)  // 3*100 + 200
	testutil.Equal(t, 250, summary.TotalOutputTokens) // 3*50 + 100
	testutil.Equal(t, 750, summary.TotalTokens)       // 500 + 250
	testutil.Equal(t, 1, summary.ErrorCount)
	testutil.Equal(t, 2, len(summary.ByProvider))

	anthro := summary.ByProvider["anthropic"]
	testutil.Equal(t, 3, anthro.Calls)
	testutil.Equal(t, 0, anthro.ErrorCount)

	oai := summary.ByProvider["openai"]
	testutil.Equal(t, 1, oai.Calls)
	testutil.Equal(t, 1, oai.ErrorCount)
}

func TestLogStoreAggregateDailyUsage(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgLogStore(sharedPG.Pool)

	// Insert logs for today.
	for i := 0; i < 3; i++ {
		err := store.Insert(ctx, ai.CallLog{
			Provider: "anthropic", Model: "claude-3",
			InputTokens: 100, OutputTokens: 50, CostUSD: 0.003,
			DurationMs: 200, Status: "success",
		})
		testutil.NoError(t, err)
	}

	// Aggregate for today.
	today := time.Now().UTC()
	rowsAffected, err := store.AggregateDailyUsage(ctx, today)
	testutil.NoError(t, err)
	testutil.True(t, rowsAffected > 0, "should aggregate at least one row")

	// Query daily usage — should reflect the aggregated data.
	daily, err := store.DailyUsage(ctx, today.Add(-24*time.Hour), today.Add(24*time.Hour))
	testutil.NoError(t, err)
	testutil.True(t, len(daily) > 0, "should have daily usage rows")
	testutil.Equal(t, "anthropic", daily[0].Provider)
	testutil.Equal(t, "claude-3", daily[0].Model)
	testutil.Equal(t, 3, daily[0].Calls)
	testutil.Equal(t, 300, daily[0].InputTokens)  // 3 * 100
	testutil.Equal(t, 150, daily[0].OutputTokens) // 3 * 50

	// Re-aggregate should be idempotent (upsert).
	rowsAffected2, err := store.AggregateDailyUsage(ctx, today)
	testutil.NoError(t, err)
	testutil.Equal(t, rowsAffected, rowsAffected2)
}

// ---------------------------------------------------------------------------
// PgPromptStore
// ---------------------------------------------------------------------------

func TestPromptStoreCreateAndGet(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	temp := 0.7
	req := ai.CreatePromptRequest{
		Name:     "sql-helper",
		Template: "Generate SQL for: {{query}}",
		Variables: []ai.PromptVariable{
			{Name: "query", Type: "string", Required: true},
		},
		Model:       "gpt-4",
		Provider:    "openai",
		MaxTokens:   1000,
		Temperature: &temp,
	}

	// Create should populate ID, version, timestamps.
	created, err := store.Create(ctx, req)
	testutil.NoError(t, err)
	testutil.True(t, created.ID != uuid.Nil, "ID should be populated")
	testutil.Equal(t, "sql-helper", created.Name)
	testutil.Equal(t, 1, created.Version)
	testutil.Equal(t, "Generate SQL for: {{query}}", created.Template)
	testutil.Equal(t, 1, len(created.Variables))
	testutil.Equal(t, "query", created.Variables[0].Name)
	testutil.Equal(t, "gpt-4", created.Model)
	testutil.Equal(t, 1000, created.MaxTokens)
	testutil.True(t, !created.CreatedAt.IsZero(), "CreatedAt should be set")

	// Get by ID should return the same prompt.
	got, err := store.Get(ctx, created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, got.ID)
	testutil.Equal(t, "sql-helper", got.Name)

	// GetByName should also work.
	gotByName, err := store.GetByName(ctx, "sql-helper")
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, gotByName.ID)
}

func TestPromptStoreGetNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	_, err := store.Get(ctx, uuid.New())
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not found")

	_, err = store.GetByName(ctx, "nonexistent")
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not found")
}

func TestPromptStoreList(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	// Create 3 prompts.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		_, err := store.Create(ctx, ai.CreatePromptRequest{
			Name: name, Template: "template for " + name,
		})
		testutil.NoError(t, err)
	}

	// List page 1 of 2 items.
	prompts, total, err := store.List(ctx, 1, 2)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, total)
	testutil.Equal(t, 2, len(prompts))
	// Ordered by name, so "alpha" and "beta" first.
	testutil.Equal(t, "alpha", prompts[0].Name)
	testutil.Equal(t, "beta", prompts[1].Name)

	// Page 2 returns "gamma".
	prompts, total, err = store.List(ctx, 2, 2)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, total)
	testutil.Equal(t, 1, len(prompts))
	testutil.Equal(t, "gamma", prompts[0].Name)
}

func TestPromptStoreUpdate(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	created, err := store.Create(ctx, ai.CreatePromptRequest{
		Name: "updatable", Template: "v1 template", Model: "gpt-3.5",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, created.Version)

	// Partial update: only change template.
	newTemplate := "v2 template"
	updated, err := store.Update(ctx, created.ID, ai.UpdatePromptRequest{
		Template: &newTemplate,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, updated.Version)
	testutil.Equal(t, "v2 template", updated.Template)
	testutil.Equal(t, "gpt-3.5", updated.Model) // unchanged

	// Another update increments version again.
	newModel := "gpt-4"
	updated2, err := store.Update(ctx, created.ID, ai.UpdatePromptRequest{
		Model: &newModel,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 3, updated2.Version)
	testutil.Equal(t, "gpt-4", updated2.Model)
	testutil.Equal(t, "v2 template", updated2.Template) // unchanged from last update
}

func TestPromptStoreUpdateCreatesVersionHistory(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	created, err := store.Create(ctx, ai.CreatePromptRequest{
		Name: "versioned", Template: "v1",
	})
	testutil.NoError(t, err)

	// Update twice to create version history.
	v2Template := "v2"
	_, err = store.Update(ctx, created.ID, ai.UpdatePromptRequest{Template: &v2Template})
	testutil.NoError(t, err)
	v3Template := "v3"
	_, err = store.Update(ctx, created.ID, ai.UpdatePromptRequest{Template: &v3Template})
	testutil.NoError(t, err)

	// ListVersions should return version history from the trigger.
	versions, err := store.ListVersions(ctx, created.ID)
	testutil.NoError(t, err)
	// The trigger fires on UPDATE, so versions are created for v1→v2 and v2→v3.
	testutil.True(t, len(versions) >= 2, "should have at least 2 version history entries")
	// Versions are ordered DESC, so newest first.
	testutil.True(t, versions[0].Version >= versions[len(versions)-1].Version, "versions should be ordered newest-first")
}

func TestPromptStoreDelete(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	created, err := store.Create(ctx, ai.CreatePromptRequest{
		Name: "deletable", Template: "will be deleted",
	})
	testutil.NoError(t, err)

	// Delete should succeed.
	err = store.Delete(ctx, created.ID)
	testutil.NoError(t, err)

	// Get should fail after deletion.
	_, err = store.Get(ctx, created.ID)
	testutil.Error(t, err)

	// Delete nonexistent should return error.
	err = store.Delete(ctx, uuid.New())
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not found")
}

func TestPromptStoreDuplicateNameFails(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	store := ai.NewPgPromptStore(sharedPG.Pool)

	_, err := store.Create(ctx, ai.CreatePromptRequest{
		Name: "unique-name", Template: "first",
	})
	testutil.NoError(t, err)

	// Creating another prompt with the same name should fail.
	_, err = store.Create(ctx, ai.CreatePromptRequest{
		Name: "unique-name", Template: "second",
	})
	testutil.Error(t, err)
}
