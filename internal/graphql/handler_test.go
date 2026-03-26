package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/graphql-go/graphql/language/parser"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	cache := testCache([]*schema.Table{{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}})
	return &Handler{
		cache:  cache,
		logger: testutil.DiscardLogger(),
	}
}

func doGraphQLRequest(h *Handler, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// testGQLResponse is a test-friendly response type with typed Errors.
type testGQLResponse struct {
	Data   interface{}              `json:"data"`
	Errors []map[string]interface{} `json:"errors"`
}

func TestHandlerIntrospection(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ __schema { queryType { name } } }`,
	})
	testutil.Equal(t, http.StatusOK, rr.Code)
	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Errors))
	testutil.NotNil(t, resp.Data)
}

func TestHandlerBasicQuery(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	// Without a pool/resolver factory, the query returns nil data (no resolver).
	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ posts { id title } }`,
	})
	testutil.Equal(t, http.StatusOK, rr.Code)
	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Errors))
}

func TestHandlerMalformedJSON(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	testutil.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandlerGetWithoutWebSocketUpgradeRejected(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	body := bytes.NewReader([]byte(`{"query":"{ posts { id } }"}`))
	req := httptest.NewRequest(http.MethodGet, "/api/graphql", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	testutil.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandlerEmptyBody(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	// Empty query should return 200 with GraphQL errors
	testutil.Equal(t, http.StatusOK, rr.Code)
}

func TestHandlerSyntaxError(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ invalid syntax !!!`,
	})
	// GraphQL syntax errors return 200 with errors per spec
	testutil.Equal(t, http.StatusOK, rr.Code)
	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.True(t, len(resp.Errors) > 0, "should have GraphQL errors")
}

func TestHandlerWithVariables(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	body := map[string]interface{}{
		"query":     `query($lim: Int) { posts(limit: $lim) { id } }`,
		"variables": map[string]interface{}{"lim": 5},
	}
	rr := doGraphQLRequest(h, body)
	testutil.Equal(t, http.StatusOK, rr.Code)
	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Errors))
}

func TestHandlerIntrospectionGatedNonAdmin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	// Set an admin checker that always returns false
	h.isAdmin = func(r *http.Request) bool { return false }

	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ __schema { queryType { name } } }`,
	})
	testutil.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandlerIntrospectionAllowedForAdmin(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	h.isAdmin = func(r *http.Request) bool { return true }

	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ __schema { queryType { name } } }`,
	})
	testutil.Equal(t, http.StatusOK, rr.Code)
	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Errors))
}

func TestHandlerIntrospectionAllowedWhenNoAdminChecker(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	// isAdmin is nil — no gating, introspection allowed (backward compat / dev mode)
	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ __schema { queryType { name } } }`,
	})
	testutil.Equal(t, http.StatusOK, rr.Code)
	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Errors))
}

func TestIsMutationDoc(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		query         string
		operationName string
		want          bool
	}{
		{
			name:  "single query",
			query: `query { posts { id } }`,
			want:  false,
		},
		{
			name:  "single mutation",
			query: `mutation { insert_posts(objects: [{title: "x"}]) { affected_rows } }`,
			want:  true,
		},
		{
			name:  "single subscription",
			query: `subscription { posts { id } }`,
			want:  false,
		},
		{
			name:          "named mutation operation selected",
			query:         `query Q { posts { id } } mutation M { delete_posts(where: {id: {_eq: 1}}) { affected_rows } }`,
			operationName: "M",
			want:          true,
		},
		{
			name:          "named query operation selected",
			query:         `query Q { posts { id } } mutation M { delete_posts(where: {id: {_eq: 1}}) { affected_rows } }`,
			operationName: "Q",
			want:          false,
		},
		{
			name:  "multiple operations without operation name",
			query: `query Q { posts { id } } mutation M { delete_posts(where: {id: {_eq: 1}}) { affected_rows } }`,
			want:  false,
		},
		{
			name:  "invalid query",
			query: `{ invalid syntax !!!`,
			want:  false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parser.Parse(parser.ParseParams{Source: tc.query})
			if err != nil {
				testutil.Equal(t, false, tc.want)
				return
			}
			got := isMutationDoc(doc, tc.operationName)
			testutil.Equal(t, tc.want, got)
		})
	}
}

func TestHandlerRejectsDepthOverLimit(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	h.SetLimits(1, 0)

	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ posts { id } }`,
	})
	testutil.Equal(t, http.StatusOK, rr.Code)

	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.True(t, len(resp.Errors) > 0, "should include depth analysis error")
	testutil.Contains(t, resp.Errors[0]["message"].(string), "exceeds maximum allowed depth")
}

func TestHandlerRejectsComplexityOverLimit(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	h.SetLimits(0, 1)

	rr := doGraphQLRequest(h, map[string]string{
		"query": `{ posts { id title } }`,
	})
	testutil.Equal(t, http.StatusOK, rr.Code)

	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.True(t, len(resp.Errors) > 0, "should include complexity analysis error")
	testutil.Contains(t, resp.Errors[0]["message"].(string), "exceeds maximum allowed complexity")
}

func TestHandlerComplexityAnalyzesSelectedOperationOnly(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	h.SetLimits(0, 50)

	body := map[string]interface{}{
		"query": `
			query Cheap { posts(limit: 1) { id } }
			query Expensive { posts(limit: 100) { id title } }
		`,
		"operationName": "Cheap",
	}
	rr := doGraphQLRequest(h, body)
	testutil.Equal(t, http.StatusOK, rr.Code)

	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Errors))
}

func TestHandlerComplexityRejectsVariableLimitOverBudget(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	h.SetLimits(0, 1000)

	body := map[string]interface{}{
		"query":     `query($lim: Int) { posts(limit: $lim) { id title } }`,
		"variables": map[string]interface{}{"lim": 1000},
	}
	rr := doGraphQLRequest(h, body)
	testutil.Equal(t, http.StatusOK, rr.Code)

	var resp testGQLResponse
	testutil.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	testutil.True(t, len(resp.Errors) > 0, "should include complexity analysis error")
	testutil.Contains(t, resp.Errors[0]["message"].(string), "exceeds maximum allowed complexity")
}

func TestHandlerPublishesCollectedMutationEventsToHub(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	hub := realtime.NewHub(testutil.DiscardLogger())
	h.SetHub(hub)
	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	ctx := ctxWithMutationEventCollector(context.Background())
	addMutationEvent(ctx, &realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 7, "title": "created"},
	})

	h.publishCollectedMutationEvents(ctx)

	select {
	case event := <-client.Events():
		testutil.Equal(t, "create", event.Action)
		testutil.Equal(t, "posts", event.Table)
		testutil.Equal(t, 7, event.Record["id"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected mutation event to be published")
	}
}

func TestMatchesGraphQLWhereRejectsUnknownOperator(t *testing.T) {
	t.Parallel()

	row := map[string]any{"title": "hello world"}
	where := map[string]any{
		"title": map[string]any{
			"_contains": "hello",
		},
	}

	testutil.False(t, matchesGraphQLWhere(where, row))
}

func TestMatchesGraphQLWhereSupportsLikeAndILike(t *testing.T) {
	t.Parallel()

	row := map[string]any{"title": "Hello World"}

	testutil.True(t, matchesGraphQLWhere(map[string]any{
		"title": map[string]any{
			"_like": "Hello%",
		},
	}, row))
	testutil.False(t, matchesGraphQLWhere(map[string]any{
		"title": map[string]any{
			"_like": "hello%",
		},
	}, row))
	testutil.True(t, matchesGraphQLWhere(map[string]any{
		"title": map[string]any{
			"_ilike": "hello%",
		},
	}, row))
}
