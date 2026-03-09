//go:build integration

package realtime

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupVisibilityIntegrationDB(t *testing.T) (*testutil.PGContainer, context.Context) {
	t.Helper()
	ctx := context.Background()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("integration test requires TEST_DATABASE_URL")
	}

	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	t.Cleanup(cleanup)

	_, err := pg.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(pg.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)

	return pg, ctx
}

func setupJoinPolicyFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		);

		CREATE TABLE project_memberships (
			user_id TEXT NOT NULL,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			PRIMARY KEY (user_id, project_id)
		);

		CREATE TABLE secure_docs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL
		);

		ALTER TABLE secure_docs ENABLE ROW LEVEL SECURITY;
		ALTER TABLE secure_docs FORCE ROW LEVEL SECURITY;

		CREATE POLICY secure_docs_membership_select
			ON secure_docs
			FOR SELECT
			USING (
				EXISTS (
					SELECT 1
					FROM project_memberships pm
					WHERE pm.project_id = secure_docs.project_id
					  AND pm.user_id = current_setting('ayb.user_id', true)
				)
			);

		INSERT INTO projects (id, name) VALUES ('project-1', 'Project One');
		INSERT INTO secure_docs (id, project_id, title) VALUES ('doc-1', 'project-1', 'Top Secret');
	`)
	testutil.NoError(t, err)
}

func newVisibilityIntegrationHandler(pool *pgxpool.Pool) *Handler {
	ch := schema.NewCacheHolder(nil, testutil.DiscardLogger())
	ch.SetForTesting(&schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.secure_docs": {
				Schema:     "public",
				Name:       "secure_docs",
				PrimaryKey: []string{"id"},
			},
		},
	})
	return &Handler{
		pool:        pool,
		schemaCache: ch,
		logger:      testutil.DiscardLogger(),
	}
}

func testClaims(userID string) *auth.Claims {
	claims := &auth.Claims{
		Email: userID + "@example.com",
	}
	claims.Subject = userID
	return claims
}

func TestCanSeeRecordJoinPolicyMembershipAccess(t *testing.T) {
	pg, ctx := setupVisibilityIntegrationDB(t)
	setupJoinPolicyFixture(t, ctx, pg.Pool)

	_, err := pg.Pool.Exec(ctx,
		`INSERT INTO project_memberships (user_id, project_id) VALUES ($1, 'project-1')`,
		"user-member",
	)
	testutil.NoError(t, err)

	h := newVisibilityIntegrationHandler(pg.Pool)
	event := &Event{
		Action: "update",
		Table:  "secure_docs",
		Record: map[string]any{"id": "doc-1"},
	}

	testutil.True(t, h.canSeeRecord(ctx, testClaims("user-member"), event), "member should pass joined-table RLS")
	testutil.False(t, h.canSeeRecord(ctx, testClaims("user-outsider"), event), "non-member should fail joined-table RLS")
}

func TestCanSeeRecordJoinPolicyMembershipTransitions(t *testing.T) {
	pg, ctx := setupVisibilityIntegrationDB(t)
	setupJoinPolicyFixture(t, ctx, pg.Pool)

	h := newVisibilityIntegrationHandler(pg.Pool)
	event := &Event{
		Action: "create",
		Table:  "secure_docs",
		Record: map[string]any{"id": "doc-1"},
	}
	claims := testClaims("user-transitions")

	testutil.False(t, h.canSeeRecord(ctx, claims, event), "without membership the event should be filtered")

	_, err := pg.Pool.Exec(ctx,
		`INSERT INTO project_memberships (user_id, project_id) VALUES ($1, 'project-1')`,
		"user-transitions",
	)
	testutil.NoError(t, err)
	testutil.True(t, h.canSeeRecord(ctx, claims, event), "after membership grant the event should pass")

	_, err = pg.Pool.Exec(ctx,
		`DELETE FROM project_memberships WHERE user_id = $1 AND project_id = 'project-1'`,
		"user-transitions",
	)
	testutil.NoError(t, err)
	testutil.False(t, h.canSeeRecord(ctx, claims, event), "after membership revoke the event should be filtered again")
}

func TestCanSeeRecordDeletePassThroughWithJoinPolicy(t *testing.T) {
	pg, ctx := setupVisibilityIntegrationDB(t)
	setupJoinPolicyFixture(t, ctx, pg.Pool)

	h := newVisibilityIntegrationHandler(pg.Pool)
	event := &Event{
		Action: "delete",
		Table:  "secure_docs",
		Record: map[string]any{"id": "doc-1"},
	}

	testutil.True(t, h.canSeeRecord(ctx, testClaims("user-outsider"), event),
		"delete events should pass through even when user is not currently a member")
}
