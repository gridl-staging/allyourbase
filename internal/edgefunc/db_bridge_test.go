package edgefunc

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// mockQueryExecutor returns canned results for testing the JS→Go DB bridge.
type mockQueryExecutor struct {
	lastQuery Query
	queries   []Query // all queries in execution order
	result    QueryResult
	err       error
}

func (m *mockQueryExecutor) Execute(_ context.Context, q Query) (QueryResult, error) {
	m.lastQuery = q
	m.queries = append(m.queries, q)
	return m.result, m.err
}

func TestGojaRuntime_DB_SelectAll(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{
			Rows: []map[string]interface{}{
				{"id": 1, "name": "alice"},
				{"id": 2, "name": "bob"},
			},
		},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").select("*").execute();
	return {
		statusCode: 200,
		body: JSON.stringify(result),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"name":"alice"`))
	testutil.True(t, strings.Contains(string(resp.Body), `"name":"bob"`))

	// Verify the query the JS side built
	testutil.Equal(t, "users", mock.lastQuery.Table)
	testutil.Equal(t, "select", mock.lastQuery.Action)
	testutil.Equal(t, "*", mock.lastQuery.Columns)
}

func TestGojaRuntime_DB_SelectWithFilter(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{
			Rows: []map[string]interface{}{
				{"id": 1, "name": "alice"},
			},
		},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").select("id, name").eq("id", 1).execute();
	return {
		statusCode: 200,
		body: JSON.stringify(result),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(string(resp.Body), `"name":"alice"`))

	testutil.Equal(t, "users", mock.lastQuery.Table)
	testutil.Equal(t, "id, name", mock.lastQuery.Columns)
	testutil.Equal(t, 1, len(mock.lastQuery.Filters))
	testutil.Equal(t, "id", mock.lastQuery.Filters[0].Column)
	testutil.Equal(t, "eq", mock.lastQuery.Filters[0].Op)
}

func TestGojaRuntime_DB_Insert(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{
			Rows: []map[string]interface{}{
				{"id": 3, "name": "charlie"},
			},
		},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").insert({name: "charlie"}).execute();
	return {
		statusCode: 201,
		body: JSON.stringify(result),
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "POST", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 201, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"name":"charlie"`))

	testutil.Equal(t, "users", mock.lastQuery.Table)
	testutil.Equal(t, "insert", mock.lastQuery.Action)
	testutil.True(t, mock.lastQuery.Data["name"] == "charlie")
}

func TestGojaRuntime_DB_NotRegisteredWithoutExecutor(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime() // no WithQueryExecutor

	code := `
function handler(request) {
	var result = ayb.db.from("users").select("*").execute();
	return { statusCode: 200, body: "should not reach" };
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error when ayb.db not available")
}

func TestGojaRuntime_DB_ChainIsolation(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{
			Rows: []map[string]interface{}{{"id": 1}},
		},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	// Fork a chain: base.eq("id",1) and base.eq("name","bob") must be independent.
	// If the query builder shares a *Query pointer, the second fork inherits the first's filter.
	code := `
function handler(request) {
	var base = ayb.db.from("users").select("*");
	var q1 = base.eq("id", 1);
	var q2 = base.eq("name", "bob");
	q1.execute();
	q2.execute();
	return { statusCode: 200, body: "ok" };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)

	// q2 was the last execute, so mock.lastQuery should reflect q2.
	// It must have exactly 1 filter (name=bob), NOT 2 (id=1 AND name=bob).
	testutil.Equal(t, 1, len(mock.lastQuery.Filters))
	testutil.Equal(t, "name", mock.lastQuery.Filters[0].Column)
}

func TestGojaRuntime_DB_QueryError(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		err: context.DeadlineExceeded,
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").select("*").execute();
	return { statusCode: 200, body: "should not reach" };
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error from failed query")
}

func TestGojaRuntime_DB_FilterOperations(t *testing.T) {
	t.Parallel()

	ops := []struct {
		jsMethod string
		wantOp   string
	}{
		{"neq", "neq"},
		{"gt", "gt"},
		{"lt", "lt"},
		{"gte", "gte"},
		{"lte", "lte"},
	}

	for _, op := range ops {
		t.Run(op.jsMethod, func(t *testing.T) {
			t.Parallel()
			mock := &mockQueryExecutor{
				result: QueryResult{Rows: []map[string]interface{}{{"id": 1}}},
			}
			rt := NewGojaRuntime(WithQueryExecutor(mock))

			code := `
function handler(request) {
	var result = ayb.db.from("users").select("*").` + op.jsMethod + `("age", 30).execute();
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
			resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
			testutil.NoError(t, err)
			testutil.Equal(t, 200, resp.StatusCode)

			testutil.Equal(t, "users", mock.lastQuery.Table)
			testutil.Equal(t, "select", mock.lastQuery.Action)
			testutil.Equal(t, 1, len(mock.lastQuery.Filters))
			testutil.Equal(t, "age", mock.lastQuery.Filters[0].Column)
			testutil.Equal(t, op.wantOp, mock.lastQuery.Filters[0].Op)
			testutil.True(t, mock.lastQuery.Filters[0].Value == int64(30), "expected filter value 30")
		})
	}
}

func TestGojaRuntime_DB_MultipleFilters(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{Rows: []map[string]interface{}{{"id": 2}}},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").select("*").gte("age", 18).lt("age", 65).neq("status", "banned").execute();
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)

	testutil.Equal(t, 3, len(mock.lastQuery.Filters))
	testutil.Equal(t, "gte", mock.lastQuery.Filters[0].Op)
	testutil.Equal(t, "lt", mock.lastQuery.Filters[1].Op)
	testutil.Equal(t, "neq", mock.lastQuery.Filters[2].Op)
}

func TestGojaRuntime_DB_Update(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{Rows: []map[string]interface{}{{"id": 1, "name": "alice_updated"}}},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").update({name: "alice_updated"}).eq("id", 1).execute();
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "PUT", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"name":"alice_updated"`))

	testutil.Equal(t, "users", mock.lastQuery.Table)
	testutil.Equal(t, "update", mock.lastQuery.Action)
	testutil.True(t, mock.lastQuery.Data["name"] == "alice_updated")
	testutil.Equal(t, 1, len(mock.lastQuery.Filters))
	testutil.Equal(t, "eq", mock.lastQuery.Filters[0].Op)
}

func TestGojaRuntime_DB_Delete(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{Rows: []map[string]interface{}{{"id": 1}}},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").delete().eq("id", 1).execute();
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "DELETE", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)

	testutil.Equal(t, "users", mock.lastQuery.Table)
	testutil.Equal(t, "delete", mock.lastQuery.Action)
	testutil.Equal(t, 1, len(mock.lastQuery.Filters))
	testutil.Equal(t, "id", mock.lastQuery.Filters[0].Column)
}

func TestGojaRuntime_DB_UpdateWithMultipleFilters(t *testing.T) {
	t.Parallel()
	mock := &mockQueryExecutor{
		result: QueryResult{Rows: []map[string]interface{}{{"id": 3}}},
	}
	rt := NewGojaRuntime(WithQueryExecutor(mock))

	code := `
function handler(request) {
	var result = ayb.db.from("users").update({status: "active"}).gte("age", 18).neq("role", "admin").execute();
	return { statusCode: 200, body: JSON.stringify(result) };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "PUT", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)

	testutil.Equal(t, "update", mock.lastQuery.Action)
	testutil.True(t, mock.lastQuery.Data["status"] == "active")
	testutil.Equal(t, 2, len(mock.lastQuery.Filters))
	testutil.Equal(t, "gte", mock.lastQuery.Filters[0].Op)
	testutil.Equal(t, "neq", mock.lastQuery.Filters[1].Op)
}

func TestCopyQuery_DataMapIsIndependent(t *testing.T) {
	t.Parallel()
	original := Query{
		Table:  "users",
		Action: "update",
		Data:   map[string]interface{}{"name": "alice", "age": 30},
	}

	cp := copyQuery(original)

	// Mutate the copy's Data — must not affect the original.
	cp.Data["name"] = "bob"
	cp.Data["extra"] = "injected"

	testutil.Equal(t, "alice", original.Data["name"])
	_, hasExtra := original.Data["extra"]
	testutil.False(t, hasExtra, "mutation of copy must not affect original Data map")

	// Mutate original — must not affect copy.
	original.Data["age"] = 99
	testutil.Equal(t, 30, cp.Data["age"])
}
