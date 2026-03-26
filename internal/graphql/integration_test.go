//go:build integration

package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/graphql"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func resetAndSeedDB(t *testing.T, ctx context.Context) {
	t.Helper()

	resetAndSeedDBWithPool(t, ctx, sharedPG.Pool)
}

func resetAndSeedDBWithPool(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			author_id INTEGER REFERENCES authors(id),
			status TEXT DEFAULT 'draft',
			created_at TIMESTAMPTZ DEFAULT now()
		);

		INSERT INTO authors (name) VALUES ('Alice'), ('Bob');
		INSERT INTO posts (title, body, author_id, status) VALUES
			('First Post', 'Hello world', 1, 'published'),
			('Second Post', 'Another post', 1, 'draft'),
			('Bob Post', 'By Bob', 2, 'published');
	`)
	if err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
}

func setupHandler(t *testing.T, ctx context.Context) *graphql.Handler {
	t.Helper()

	resetAndSeedDB(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	return graphql.NewHandler(sharedPG.Pool, ch, logger)
}

type selectCountTracer struct {
	mu         sync.Mutex
	selects    int
	statements []string
}

func (t *selectCountTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	sql := strings.TrimSpace(strings.ToLower(data.SQL))
	if strings.HasPrefix(sql, "select") {
		t.mu.Lock()
		t.selects++
		t.statements = append(t.statements, data.SQL)
		t.mu.Unlock()
	}
	return ctx
}

func (t *selectCountTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (t *selectCountTracer) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.selects
}

func (t *selectCountTracer) statementsSince(start int) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if start < 0 || start >= len(t.statements) {
		return nil
	}
	out := make([]string, len(t.statements[start:]))
	copy(out, t.statements[start:])
	return out
}

type gqlResponse struct {
	Data   map[string]interface{}   `json:"data"`
	Errors []map[string]interface{} `json:"errors"`
}

func doGQL(t *testing.T, h *graphql.Handler, query string, vars map[string]interface{}) (*httptest.ResponseRecorder, gqlResponse) {
	t.Helper()
	body := map[string]interface{}{
		"query": query,
	}
	if vars != nil {
		body["variables"] = vars
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp gqlResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parsing response: %v\nbody: %s", err, rr.Body.String())
	}
	return rr, resp
}

func doGQLWithContext(t *testing.T, h *graphql.Handler, ctx context.Context, query string) (*httptest.ResponseRecorder, gqlResponse) {
	t.Helper()
	body := map[string]interface{}{"query": query}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader(b))
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var resp gqlResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parsing response: %v\nbody: %s", err, rr.Body.String())
	}
	return rr, resp
}

func TestIntegrationBasicSelect(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `{ posts { id title } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts, ok := resp.Data["posts"].([]interface{})
	testutil.True(t, ok, "posts should be a list")
	testutil.Equal(t, 3, len(posts))

	// Verify fields are present
	first := posts[0].(map[string]interface{})
	_, hasID := first["id"]
	_, hasTitle := first["title"]
	testutil.True(t, hasID, "should have id field")
	testutil.True(t, hasTitle, "should have title field")
}

func TestIntegrationWhereFilter(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `{ posts(where: { title: { _eq: "First Post" } }) { id title } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts := resp.Data["posts"].([]interface{})
	testutil.Equal(t, 1, len(posts))
	testutil.Equal(t, "First Post", posts[0].(map[string]interface{})["title"])
}

func TestIntegrationOrderBy(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `{ posts(order_by: { title: ASC }) { title } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts := resp.Data["posts"].([]interface{})
	testutil.Equal(t, 3, len(posts))
	testutil.Equal(t, "Bob Post", posts[0].(map[string]interface{})["title"])
	testutil.Equal(t, "First Post", posts[1].(map[string]interface{})["title"])
	testutil.Equal(t, "Second Post", posts[2].(map[string]interface{})["title"])
}

func TestIntegrationLimitOffset(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	// Use order_by for deterministic results
	rr, resp := doGQL(t, h, `{ posts(order_by: { id: ASC }, limit: 1, offset: 1) { id title } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts := resp.Data["posts"].([]interface{})
	testutil.Equal(t, 1, len(posts))
	testutil.Equal(t, "Second Post", posts[0].(map[string]interface{})["title"])
}

func TestIntegrationRLSEnforcement(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	// Set up RLS: enable on posts, create policy for author_id matching ayb.user_id
	_, err := sharedPG.Pool.Exec(ctx, `
		-- Create the authenticated role if it doesn't exist
		DO $$ BEGIN
			CREATE ROLE ayb_authenticated NOLOGIN;
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$;

		-- Grant usage on public schema and tables
		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT SELECT ON ALL TABLES IN SCHEMA public TO ayb_authenticated;

		ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
		CREATE POLICY user_posts ON posts
			FOR SELECT
			TO ayb_authenticated
			USING (author_id::text = current_setting('ayb.user_id', true));
	`)
	if err != nil {
		t.Fatalf("setting up RLS: %v", err)
	}

	// Query with claims for author_id=1 (Alice) — should only see Alice's posts
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "1"},
		Email:            "alice@test.com",
	}
	ctxWithClaims := auth.ContextWithClaims(ctx, claims)

	rr, resp := doGQLWithContext(t, h, ctxWithClaims, `{ posts(order_by: { id: ASC }) { id title } }`)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts := resp.Data["posts"].([]interface{})
	testutil.Equal(t, 2, len(posts)) // Alice has 2 posts
	testutil.Equal(t, "First Post", posts[0].(map[string]interface{})["title"])
	testutil.Equal(t, "Second Post", posts[1].(map[string]interface{})["title"])

	// Query without claims — no RLS, should see all 3 posts
	rr2, resp2 := doGQL(t, h, `{ posts(order_by: { id: ASC }) { id title } }`, nil)
	testutil.Equal(t, http.StatusOK, rr2.Code)
	testutil.Equal(t, 0, len(resp2.Errors))

	allPosts := resp2.Data["posts"].([]interface{})
	testutil.Equal(t, 3, len(allPosts))
}

func TestIntegrationIntrospectionGating(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	// Set admin checker that always returns false
	h.SetAdminChecker(func(r *http.Request) bool { return false })

	rr, _ := doGQL(t, h, `{ __schema { queryType { name } } }`, nil)
	testutil.Equal(t, http.StatusForbidden, rr.Code)

	// Set admin checker that returns true
	h.SetAdminChecker(func(r *http.Request) bool { return true })

	rr2, resp2 := doGQL(t, h, `{ __schema { queryType { name } } }`, nil)
	testutil.Equal(t, http.StatusOK, rr2.Code)
	testutil.Equal(t, 0, len(resp2.Errors))
}

func TestIntegrationMalformedQuery(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `{ invalid syntax !!!`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code) // GraphQL errors return 200 per spec
	testutil.True(t, len(resp.Errors) > 0, "should have GraphQL errors for syntax error")
}

func TestIntegrationInsertMutation(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			insert_posts(objects: [{ title: "Third Post", body: "New", author_id: 1, status: "draft" }]) {
				affected_rows
				returning { id title body author_id status }
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	insertPosts := resp.Data["insert_posts"].(map[string]interface{})
	testutil.Equal(t, float64(1), insertPosts["affected_rows"].(float64))
	returning := insertPosts["returning"].([]interface{})
	testutil.Equal(t, 1, len(returning))
	row := returning[0].(map[string]interface{})
	testutil.Equal(t, "Third Post", row["title"])
	testutil.Equal(t, "New", row["body"])
	testutil.Equal(t, float64(1), row["author_id"].(float64))
}

func TestIntegrationUpdateMutation(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			update_posts(
				where: { title: { _eq: "Second Post" } }
				_set: { body: "Updated body", status: "published" }
			) {
				affected_rows
				returning { id title body status }
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	updatePosts := resp.Data["update_posts"].(map[string]interface{})
	testutil.Equal(t, float64(1), updatePosts["affected_rows"].(float64))
	returning := updatePosts["returning"].([]interface{})
	testutil.Equal(t, 1, len(returning))
	row := returning[0].(map[string]interface{})
	testutil.Equal(t, "Second Post", row["title"])
	testutil.Equal(t, "Updated body", row["body"])
	testutil.Equal(t, "published", row["status"])
}

func TestIntegrationDeleteMutation(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			delete_posts(where: { title: { _eq: "Bob Post" } }) {
				affected_rows
				returning { id title }
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	deletePosts := resp.Data["delete_posts"].(map[string]interface{})
	testutil.Equal(t, float64(1), deletePosts["affected_rows"].(float64))
	returning := deletePosts["returning"].([]interface{})
	testutil.Equal(t, 1, len(returning))
	testutil.Equal(t, "Bob Post", returning[0].(map[string]interface{})["title"])

	_, check := doGQL(t, h, `{ posts(order_by: { id: ASC }) { id } }`, nil)
	testutil.Equal(t, 0, len(check.Errors))
	testutil.Equal(t, 2, len(check.Data["posts"].([]interface{})))
}

func TestIntegrationInsertMutationOnConflict(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			insert_posts(
				objects: [{ id: 1, title: "First Post", body: "Upserted body", author_id: 1, status: "published" }]
				on_conflict: { constraint: posts_pkey, update_columns: ["body"] }
			) {
				affected_rows
				returning { id title body }
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	insertPosts := resp.Data["insert_posts"].(map[string]interface{})
	testutil.Equal(t, float64(1), insertPosts["affected_rows"].(float64))
	returning := insertPosts["returning"].([]interface{})
	testutil.Equal(t, 1, len(returning))
	row := returning[0].(map[string]interface{})
	testutil.Equal(t, float64(1), row["id"].(float64))
	testutil.Equal(t, "Upserted body", row["body"])
}

func TestIntegrationMutationRLS(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `
		DO $$ BEGIN
			CREATE ROLE ayb_authenticated NOLOGIN;
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$;

		GRANT USAGE ON SCHEMA public TO ayb_authenticated;
		GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ayb_authenticated;
		GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ayb_authenticated;

		ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
		CREATE POLICY posts_owner_select ON posts
			FOR SELECT TO ayb_authenticated
			USING (author_id::text = current_setting('ayb.user_id', true));
		CREATE POLICY posts_owner_update ON posts
			FOR UPDATE TO ayb_authenticated
			USING (author_id::text = current_setting('ayb.user_id', true))
			WITH CHECK (author_id::text = current_setting('ayb.user_id', true));
		CREATE POLICY posts_owner_delete ON posts
			FOR DELETE TO ayb_authenticated
			USING (author_id::text = current_setting('ayb.user_id', true));
		CREATE POLICY posts_owner_insert ON posts
			FOR INSERT TO ayb_authenticated
			WITH CHECK (author_id::text = current_setting('ayb.user_id', true));
	`)
	if err != nil {
		t.Fatalf("setting up mutation RLS: %v", err)
	}

	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "1"},
		Email:            "alice@test.com",
	}
	ctxWithClaims := auth.ContextWithClaims(ctx, claims)
	rr, resp := doGQLWithContext(t, h, ctxWithClaims, `
		mutation {
			update_posts(
				where: { author_id: { _eq: 2 } }
				_set: { body: "unauthorized" }
			) {
				affected_rows
				returning { id }
			}
		}
	`)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	updatePosts := resp.Data["update_posts"].(map[string]interface{})
	testutil.Equal(t, float64(0), updatePosts["affected_rows"].(float64))
	testutil.Equal(t, 0, len(updatePosts["returning"].([]interface{})))
}

func TestIntegrationInsertMutationRejectsUnknownUpdateColumn(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			insert_posts(
				objects: [{ id: 1, title: "First Post", body: "Unchanged", author_id: 1, status: "published" }]
				on_conflict: { constraint: posts_pkey, update_columns: ["not_a_column"] }
			) {
				affected_rows
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.True(t, len(resp.Errors) > 0, "should return GraphQL error for unknown update column")
}

func TestIntegrationMultiMutationAtomicSuccess(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			first: insert_posts(objects: [{ id: 9001, title: "Atomic Success", author_id: 1, status: "draft" }]) {
				affected_rows
			}
			second: update_posts(
				where: { id: { _eq: 9001 } }
				_set: { body: "updated-in-same-request" }
			) {
				affected_rows
				returning { id title body }
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	first := resp.Data["first"].(map[string]interface{})
	testutil.Equal(t, float64(1), first["affected_rows"].(float64))

	second := resp.Data["second"].(map[string]interface{})
	testutil.Equal(t, float64(1), second["affected_rows"].(float64))
	returning := second["returning"].([]interface{})
	testutil.Equal(t, 1, len(returning))
	row := returning[0].(map[string]interface{})
	testutil.Equal(t, float64(9001), row["id"].(float64))
	testutil.Equal(t, "Atomic Success", row["title"])
	testutil.Equal(t, "updated-in-same-request", row["body"])
}

func TestIntegrationMultiMutationRollbackOnSecondFailure(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `
		mutation {
			first: insert_posts(objects: [{ id: 9002, title: "Atomic Rollback", author_id: 1, status: "draft" }]) {
				affected_rows
			}
			second: update_posts(
				where: { id: { _eq: 1 } }
				_inc: { title: 1 }
			) {
				affected_rows
			}
		}
	`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.True(t, len(resp.Errors) > 0, "second mutation should fail and rollback request transaction")

	_, check := doGQL(t, h, `{ posts(where: { id: { _eq: 9002 } }) { id title } }`, nil)
	testutil.Equal(t, 0, len(check.Errors))
	testutil.Equal(t, 0, len(check.Data["posts"].([]interface{})))
}

func TestIntegrationRelationshipManyToOne(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `{ posts(order_by: { id: ASC }) { id title author { id name } } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts := resp.Data["posts"].([]interface{})
	testutil.Equal(t, 3, len(posts))

	post1 := posts[0].(map[string]interface{})
	author1 := post1["author"].(map[string]interface{})
	testutil.Equal(t, float64(1), author1["id"].(float64))
	testutil.Equal(t, "Alice", author1["name"])

	post3 := posts[2].(map[string]interface{})
	author3 := post3["author"].(map[string]interface{})
	testutil.Equal(t, float64(2), author3["id"].(float64))
	testutil.Equal(t, "Bob", author3["name"])
}

func TestIntegrationRelationshipOneToMany(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	rr, resp := doGQL(t, h, `{ authors(order_by: { id: ASC }) { id name posts { id title } } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	authors := resp.Data["authors"].([]interface{})
	testutil.Equal(t, 2, len(authors))

	alice := authors[0].(map[string]interface{})
	alicePosts := alice["posts"].([]interface{})
	testutil.Equal(t, 2, len(alicePosts))
	aliceTitles := []string{
		alicePosts[0].(map[string]interface{})["title"].(string),
		alicePosts[1].(map[string]interface{})["title"].(string),
	}
	sort.Strings(aliceTitles)
	testutil.Equal(t, "First Post", aliceTitles[0])
	testutil.Equal(t, "Second Post", aliceTitles[1])

	bob := authors[1].(map[string]interface{})
	bobPosts := bob["posts"].([]interface{})
	testutil.Equal(t, 1, len(bobPosts))
	testutil.Equal(t, "Bob Post", bobPosts[0].(map[string]interface{})["title"].(string))
}

func TestIntegrationRelationshipNullableFKReturnsNull(t *testing.T) {
	ctx := context.Background()
	h := setupHandler(t, ctx)

	_, err := sharedPG.Pool.Exec(ctx, `INSERT INTO posts (title, body, author_id, status) VALUES ('No Author', 'orphan', NULL, 'draft')`)
	if err != nil {
		t.Fatalf("inserting nullable-fk post: %v", err)
	}

	rr, resp := doGQL(t, h, `{ posts(where: { title: { _eq: "No Author" } }) { id title author { id name } } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	posts := resp.Data["posts"].([]interface{})
	testutil.Equal(t, 1, len(posts))
	post := posts[0].(map[string]interface{})
	testutil.Nil(t, post["author"])
}

func TestIntegrationDataloaderAvoidsNPlusOne(t *testing.T) {
	ctx := context.Background()
	logger := testutil.DiscardLogger()

	cfg, err := pgxpool.ParseConfig(sharedPG.ConnString)
	if err != nil {
		t.Fatalf("parsing pool config: %v", err)
	}
	tracer := &selectCountTracer{}
	cfg.ConnConfig.Tracer = tracer

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("creating traced pool: %v", err)
	}
	defer pool.Close()

	resetAndSeedDBWithPool(t, ctx, pool)

	ch := schema.NewCacheHolder(pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	h := graphql.NewHandler(pool, ch, logger)
	baseline := tracer.count()

	rr, resp := doGQL(t, h, `{ posts(order_by: { id: ASC }) { id title author { id name } } }`, nil)
	testutil.Equal(t, http.StatusOK, rr.Code)
	testutil.Equal(t, 0, len(resp.Errors))

	selectsForRequest := tracer.count() - baseline
	if selectsForRequest != 2 {
		t.Fatalf("got %d SELECTs, want 2\nstatements:\n%s", selectsForRequest, strings.Join(tracer.statementsSince(baseline), "\n"))
	}
}
