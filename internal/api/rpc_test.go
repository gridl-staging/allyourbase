package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func testSchemaWithFunctions() *schema.SchemaCache {
	sc := testSchema()
	sc.Functions = map[string]*schema.Function{
		"public.add_numbers": {
			Schema:     "public",
			Name:       "add_numbers",
			ReturnType: "integer",
			Parameters: []*schema.FuncParam{
				{Name: "a", Type: "integer", Position: 1},
				{Name: "b", Type: "integer", Position: 2},
			},
		},
		"public.get_active_users": {
			Schema:     "public",
			Name:       "get_active_users",
			ReturnType: "SETOF record",
			ReturnsSet: true,
			Parameters: []*schema.FuncParam{
				{Name: "min_age", Type: "integer", Position: 1},
			},
		},
		"public.get_profile": {
			Schema:     "public",
			Name:       "get_profile",
			ReturnType: "record",
		},
		"public.cleanup_old_data": {
			Schema:     "public",
			Name:       "cleanup_old_data",
			ReturnType: "void",
			IsVoid:     true,
		},
		"public.no_args": {
			Schema:     "public",
			Name:       "no_args",
			ReturnType: "text",
		},
	}
	return sc
}

func rpcRequest(handler http.Handler, funcName string, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest("POST", "/rpc/"+funcName, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest("POST", "/rpc/"+funcName, nil)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// --- Schema not ready ---

func TestRPCSchemaCacheNotReady(t *testing.T) {
	t.Parallel()
	h := testHandler(nil)
	w := rpcRequest(h, "add_numbers", `{"a": 1, "b": 2}`)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "schema cache not ready")
}

// --- Function not found ---

func TestRPCFunctionNotFound(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchemaWithFunctions())
	w := rpcRequest(h, "nonexistent", `{}`)
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "function not found")
}

// --- Invalid body ---

func TestRPCInvalidJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchemaWithFunctions())
	w := rpcRequest(h, "add_numbers", `{broken`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid JSON body")
}

// --- buildRPCCall ---

func TestBuildRPCCallScalar(t *testing.T) {
	t.Parallel()
	fn := &schema.Function{
		Schema:     "public",
		Name:       "add_numbers",
		ReturnType: "integer",
		Parameters: []*schema.FuncParam{
			{Name: "a", Type: "integer", Position: 1},
			{Name: "b", Type: "integer", Position: 2},
		},
	}
	query, args, err := buildRPCCall(fn, map[string]any{"a": 1, "b": 2})
	testutil.NoError(t, err)
	testutil.Contains(t, query, `SELECT "public"."add_numbers"($1::integer, $2::integer)`)
	testutil.Equal(t, 2, len(args))
	testutil.Equal(t, 1, args[0])
	testutil.Equal(t, 2, args[1])
}

func TestBuildRPCCallSetReturning(t *testing.T) {
	t.Parallel()
	fn := &schema.Function{
		Schema:     "public",
		Name:       "get_users",
		ReturnType: "SETOF record",
		ReturnsSet: true,
		Parameters: []*schema.FuncParam{
			{Name: "min_age", Type: "integer", Position: 1},
		},
	}
	query, args, err := buildRPCCall(fn, map[string]any{"min_age": 18})
	testutil.NoError(t, err)
	testutil.Contains(t, query, `SELECT * FROM "public"."get_users"($1::integer)`)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, 18, args[0])
}

func TestBuildRPCCallNoArgs(t *testing.T) {
	t.Parallel()
	fn := &schema.Function{
		Schema:     "public",
		Name:       "now_utc",
		ReturnType: "timestamptz",
	}
	query, args, err := buildRPCCall(fn, nil)
	testutil.NoError(t, err)
	testutil.Contains(t, query, `SELECT "public"."now_utc"()`)
	testutil.Equal(t, 0, len(args))
}

func TestBuildRPCCallMissingArgPassesNull(t *testing.T) {
	t.Parallel()
	fn := &schema.Function{
		Schema:     "public",
		Name:       "greet",
		ReturnType: "text",
		Parameters: []*schema.FuncParam{
			{Name: "name", Type: "text", Position: 1},
		},
	}
	query, args, err := buildRPCCall(fn, map[string]any{})
	testutil.NoError(t, err)
	testutil.Contains(t, query, `"greet"($1::text)`)
	testutil.Equal(t, 1, len(args))
	testutil.Nil(t, args[0])
}

func TestBuildRPCCallUnnamedParamErrors(t *testing.T) {
	t.Parallel()
	fn := &schema.Function{
		Schema:     "public",
		Name:       "bad_func",
		ReturnType: "integer",
		Parameters: []*schema.FuncParam{
			{Name: "", Type: "integer", Position: 1},
		},
	}
	_, _, err := buildRPCCall(fn, map[string]any{})
	testutil.ErrorContains(t, err, "unnamed parameters")
}

// --- coerceRPCArg ---

func TestCoerceRPCArgIntegerArray(t *testing.T) {
	t.Parallel()
	result := coerceRPCArg([]any{1.0, 2.0, 3.0}, "integer[]")
	arr, ok := result.([]int32)
	testutil.True(t, ok, "expected []int32")
	testutil.Equal(t, 3, len(arr))
	testutil.Equal(t, int32(1), arr[0])
	testutil.Equal(t, int32(2), arr[1])
	testutil.Equal(t, int32(3), arr[2])
}

func TestCoerceRPCArgTextArray(t *testing.T) {
	t.Parallel()
	result := coerceRPCArg([]any{"a", "b"}, "text[]")
	arr, ok := result.([]string)
	testutil.True(t, ok, "expected []string")
	testutil.Equal(t, 2, len(arr))
	testutil.Equal(t, "a", arr[0])
	testutil.Equal(t, "b", arr[1])
}

func TestCoerceRPCArgIntegerScalar(t *testing.T) {
	t.Parallel()
	result := coerceRPCArg(float64(42), "integer")
	v, ok := result.(int64)
	testutil.True(t, ok, "expected int64")
	testutil.Equal(t, int64(42), v)
}

func TestCoerceRPCArgNil(t *testing.T) {
	t.Parallel()
	result := coerceRPCArg(nil, "integer")
	testutil.Nil(t, result)
}

func TestCoerceRPCArgStringPassthrough(t *testing.T) {
	t.Parallel()
	result := coerceRPCArg("hello", "text")
	testutil.Equal(t, "hello", result)
}

// --- FunctionByName ---

func TestFunctionByNamePublic(t *testing.T) {
	t.Parallel()
	sc := testSchemaWithFunctions()
	fn := sc.FunctionByName("add_numbers")
	testutil.NotNil(t, fn)
	testutil.Equal(t, "add_numbers", fn.Name)
}

func TestFunctionByNameNotFound(t *testing.T) {
	t.Parallel()
	sc := testSchemaWithFunctions()
	fn := sc.FunctionByName("nonexistent")
	testutil.Nil(t, fn)
}

func TestFunctionByNameNilMap(t *testing.T) {
	t.Parallel()
	sc := &schema.SchemaCache{}
	fn := sc.FunctionByName("anything")
	testutil.Nil(t, fn)
}

// --- Response format ---

func TestRPCErrorResponseIsJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchemaWithFunctions())
	w := rpcRequest(h, "nonexistent", `{}`)
	ct := w.Header().Get("Content-Type")
	testutil.Equal(t, "application/json", ct)

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	testutil.Equal(t, float64(404), resp["code"].(float64))
	testutil.Contains(t, resp["message"].(string), "function")
}

func TestNormalizeRPCNotifyContract(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		headers map[string]string
		want    rpcNotifyContract
	}{
		{
			name: "empty_headers",
			want: rpcNotifyContract{},
		},
		{
			name: "missing_table_with_action_present",
			headers: map[string]string{
				"X-Notify-Action": "create",
			},
			want: rpcNotifyContract{},
		},
		{
			name: "missing_action_with_table_present",
			headers: map[string]string{
				"X-Notify-Table": "users",
			},
			want: rpcNotifyContract{},
		},
		{
			name: "blank_or_whitespace_values_for_both_headers",
			headers: map[string]string{
				"X-Notify-Table":  " \t ",
				"X-Notify-Action": "\n ",
			},
			want: rpcNotifyContract{},
		},
		{
			name: "supported_action_create_plain_table",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "create",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "create",
				table:   "users",
			},
		},
		{
			name: "supported_action_update_plain_table",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "update",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "update",
				table:   "users",
			},
		},
		{
			name: "supported_action_delete_plain_table",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "delete",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "delete",
				table:   "users",
			},
		},
		{
			name: "mixed_case_update_is_normalized",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "UpDaTe",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "update",
				table:   "users",
			},
		},
		{
			name: "mixed_case_create_is_normalized",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "CREATE",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "create",
				table:   "users",
			},
		},
		{
			name: "unsupported_action_upsert_disables_notify",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "upsert",
			},
			want: rpcNotifyContract{},
		},
		{
			name: "unsupported_action_drop_disables_notify",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "drop",
			},
			want: rpcNotifyContract{},
		},
		{
			name: "unsupported_action_select_disables_notify",
			headers: map[string]string{
				"X-Notify-Table":  "users",
				"X-Notify-Action": "SELECT",
			},
			want: rpcNotifyContract{},
		},
		{
			name: "schema_qualified_table_name_is_preserved",
			headers: map[string]string{
				"X-Notify-Table":  "myschema.users",
				"X-Notify-Action": "update",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "update",
				table:   "myschema.users",
			},
		},
		{
			name: "leading_and_trailing_whitespace_is_trimmed_and_normalized",
			headers: map[string]string{
				"X-Notify-Table":  "  users\t",
				"X-Notify-Action": "  UpDaTe ",
			},
			want: rpcNotifyContract{
				enabled: true,
				action:  "update",
				table:   "users",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			for k, v := range tc.headers {
				headers.Set(k, v)
			}

			got := normalizeRPCNotifyContract(headers)
			testutil.Equal(t, tc.want.enabled, got.enabled)
			testutil.Equal(t, tc.want.action, got.action)
			testutil.Equal(t, tc.want.table, got.table)
		})
	}
}

type recordingHub struct {
	events []*realtime.Event
}

func (h *recordingHub) Publish(event *realtime.Event) {
	h.events = append(h.events, event)
}

type recordingEventSink struct {
	events []*realtime.Event
}

func (s *recordingEventSink) Enqueue(event *realtime.Event) {
	s.events = append(s.events, event)
}

type rpcExecPlan struct {
	tag pgconn.CommandTag
	err error
}

type rpcQueryPlan struct {
	rows pgx.Rows
	err  error
}

type rpcRequestPathConn struct {
	execPlans    map[string]rpcExecPlan
	queryPlans   map[string]rpcQueryPlan
	execQueries  []string
	queryQueries []string
	beginTx      pgx.Tx
	beginErr     error
}

func newRPCRequestPathConn() *rpcRequestPathConn {
	return &rpcRequestPathConn{
		execPlans: map[string]rpcExecPlan{
			"cleanup_old_data": {tag: pgconn.NewCommandTag("DELETE 1")},
		},
		queryPlans: map[string]rpcQueryPlan{
			"add_numbers": {
				rows: &fakeBatchRows{
					cols: []string{"add_numbers"},
					rows: [][]any{{int64(3)}},
				},
			},
			"get_active_users": {
				rows: &fakeBatchRows{
					cols: []string{"id", "email"},
					rows: [][]any{
						{"u1", "ada@example.com"},
						{"u2", "grace@example.com"},
					},
				},
			},
			"get_profile": {
				rows: &fakeBatchRows{
					cols: []string{"id", "email"},
					rows: [][]any{
						{"u1", "ada@example.com"},
					},
				},
			},
		},
	}
}

func (c *rpcRequestPathConn) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	fn := rpcFunctionFromSQL(query)
	if fn != "" {
		c.queryQueries = append(c.queryQueries, query)
	}
	plan, ok := c.queryPlans[fn]
	if !ok {
		return &fakeBatchRows{}, nil
	}
	if plan.err != nil {
		return nil, plan.err
	}
	if plan.rows == nil {
		return &fakeBatchRows{}, nil
	}
	return plan.rows, nil
}

func (c *rpcRequestPathConn) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeRequestRow{}
}

func (c *rpcRequestPathConn) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	fn := rpcFunctionFromSQL(query)
	if fn != "" {
		c.execQueries = append(c.execQueries, query)
	}
	plan, ok := c.execPlans[fn]
	if !ok {
		return pgconn.NewCommandTag("SELECT 1"), nil
	}
	if plan.err != nil {
		return pgconn.CommandTag{}, plan.err
	}
	return plan.tag, nil
}

func (c *rpcRequestPathConn) Begin(context.Context) (pgx.Tx, error) {
	if c.beginErr != nil {
		return nil, c.beginErr
	}
	if c.beginTx != nil {
		return c.beginTx, nil
	}
	return nil, errors.New("Begin should not be called in RPC request-path harness")
}

type rpcRequestPathTx struct {
	conn      *rpcRequestPathConn
	commitErr error
}

func (tx *rpcRequestPathTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *rpcRequestPathTx) Commit(context.Context) error          { return tx.commitErr }
func (tx *rpcRequestPathTx) Rollback(context.Context) error        { return nil }
func (tx *rpcRequestPathTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (tx *rpcRequestPathTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (tx *rpcRequestPathTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (tx *rpcRequestPathTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (tx *rpcRequestPathTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return tx.conn.Exec(ctx, query, args...)
}
func (tx *rpcRequestPathTx) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return tx.conn.Query(ctx, query, args...)
}
func (tx *rpcRequestPathTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeRequestRow{}
}
func (tx *rpcRequestPathTx) Conn() *pgx.Conn { return nil }

func rpcFunctionFromSQL(query string) string {
	switch {
	case strings.Contains(query, `"cleanup_old_data"`):
		return "cleanup_old_data"
	case strings.Contains(query, `"add_numbers"`):
		return "add_numbers"
	case strings.Contains(query, `"get_active_users"`):
		return "get_active_users"
	case strings.Contains(query, `"get_profile"`):
		return "get_profile"
	default:
		return ""
	}
}

type rpcRequestPathHarness struct {
	router     http.Handler
	conn       *rpcRequestPathConn
	hub        *recordingHub
	dispatcher *recordingEventSink
}

func newRPCRequestPathHarness() *rpcRequestPathHarness {
	conn := newRPCRequestPathConn()
	hub := &recordingHub{}
	dispatcher := &recordingEventSink{}
	h := NewHandler(nil, testCacheHolder(testSchemaWithFunctions()), slog.Default(), hub, dispatcher, nil)
	return &rpcRequestPathHarness{
		router:     h.Routes(),
		conn:       conn,
		hub:        hub,
		dispatcher: dispatcher,
	}
}

func (h *rpcRequestPathHarness) doRPC(function, body string, headers map[string]string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, "/rpc/"+function, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, "/rpc/"+function, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req = req.WithContext(tenant.ContextWithRequestConn(req.Context(), h.conn))
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func (h *rpcRequestPathHarness) doRPCWithClaims(function, body string, headers map[string]string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, "/rpc/"+function, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, "/rpc/"+function, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ctx := tenant.ContextWithRequestConn(req.Context(), h.conn)
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user_1",
		},
		Email: "user_1@example.com",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func (h *rpcRequestPathHarness) assertNoPublish(t *testing.T) {
	t.Helper()
	testutil.Equal(t, 0, len(h.hub.events))
	testutil.Equal(t, 0, len(h.dispatcher.events))
}

func (h *rpcRequestPathHarness) assertPublishedEvents(t *testing.T, action, table string, records []map[string]any) {
	t.Helper()
	testutil.Equal(t, len(records), len(h.hub.events))
	testutil.Equal(t, len(records), len(h.dispatcher.events))
	for i := range records {
		hubEvent := h.hub.events[i]
		dispatcherEvent := h.dispatcher.events[i]
		testutil.NotNil(t, hubEvent)
		testutil.NotNil(t, dispatcherEvent)
		testutil.Equal(t, action, hubEvent.Action)
		testutil.Equal(t, action, dispatcherEvent.Action)
		testutil.Equal(t, table, hubEvent.Table)
		testutil.Equal(t, table, dispatcherEvent.Table)
		assertEventRecordEquals(t, records[i], hubEvent.Record)
		assertEventRecordEquals(t, records[i], dispatcherEvent.Record)
		testutil.Nil(t, hubEvent.OldRecord)
		testutil.Nil(t, dispatcherEvent.OldRecord)
		testutil.True(t, reflect.DeepEqual(hubEvent.Record, dispatcherEvent.Record))
		testutil.True(t, reflect.DeepEqual(hubEvent.OldRecord, dispatcherEvent.OldRecord))
	}
}

func assertEventRecordEquals(t *testing.T, want, got map[string]any) {
	t.Helper()
	if want == nil {
		testutil.Nil(t, got)
		return
	}
	testutil.NotNil(t, got)
	testutil.Equal(t, len(want), len(got))
	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Fatalf("record missing key %q", key)
		}
		if !reflect.DeepEqual(wantValue, gotValue) {
			t.Fatalf("record value mismatch for key %q: got %v want %v", key, gotValue, wantValue)
		}
	}
}

func (h *rpcRequestPathHarness) assertExecCalledOnce(t *testing.T, function string) {
	t.Helper()
	testutil.Equal(t, 1, len(h.conn.execQueries))
	testutil.Equal(t, 0, len(h.conn.queryQueries))
	if len(h.conn.execQueries) == 0 {
		return
	}
	testutil.Contains(t, h.conn.execQueries[0], `"`+function+`"`)
}

func (h *rpcRequestPathHarness) assertQueryCalledOnce(t *testing.T, function string) {
	t.Helper()
	testutil.Equal(t, 0, len(h.conn.execQueries))
	testutil.Equal(t, 1, len(h.conn.queryQueries))
	if len(h.conn.queryQueries) == 0 {
		return
	}
	testutil.Contains(t, h.conn.queryQueries[0], `"`+function+`"`)
}

func (h *rpcRequestPathHarness) assertNoExecution(t *testing.T) {
	t.Helper()
	testutil.Equal(t, 0, len(h.conn.execQueries))
	testutil.Equal(t, 0, len(h.conn.queryQueries))
}

type rpcNotifyHeaderCase struct {
	name    string
	headers map[string]string
}

func TestRPCHandleIgnoresMalformedNotifyHeadersAndKeepsResponses(t *testing.T) {
	t.Parallel()

	headerCases := []rpcNotifyHeaderCase{
		{name: "no_headers"},
		{name: "missing_table", headers: map[string]string{"X-Notify-Action": "update"}},
		{name: "missing_action", headers: map[string]string{"X-Notify-Table": "users"}},
		{name: "blank_values", headers: map[string]string{"X-Notify-Table": "   ", "X-Notify-Action": "\t"}},
		{name: "unsupported_action", headers: map[string]string{"X-Notify-Table": "users", "X-Notify-Action": "upsert"}},
	}

	rpcCases := []struct {
		name       string
		function   string
		body       string
		usesExec   bool
		assertBody func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:     "void_cleanup_old_data",
			function: "cleanup_old_data",
			assertBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				testutil.Equal(t, http.StatusNoContent, w.Code)
				testutil.Equal(t, "", w.Body.String())
			},
			usesExec: true,
		},
		{
			name:     "scalar_add_numbers",
			function: "add_numbers",
			body:     `{"a": 1, "b": 2}`,
			assertBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				testutil.Equal(t, http.StatusOK, w.Code)
				var value float64
				testutil.NoError(t, json.NewDecoder(w.Body).Decode(&value))
				testutil.Equal(t, float64(3), value)
			},
		},
		{
			name:     "set_get_active_users",
			function: "get_active_users",
			body:     `{"min_age": 18}`,
			assertBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				testutil.Equal(t, http.StatusOK, w.Code)
				var rows []map[string]any
				testutil.NoError(t, json.NewDecoder(w.Body).Decode(&rows))
				testutil.Equal(t, 2, len(rows))
				if len(rows) > 0 {
					testutil.Equal(t, "u1", rows[0]["id"])
				}
			},
		},
		{
			name:     "single_row_get_profile",
			function: "get_profile",
			assertBody: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				testutil.Equal(t, http.StatusOK, w.Code)
				var row map[string]any
				testutil.NoError(t, json.NewDecoder(w.Body).Decode(&row))
				testutil.Equal(t, "u1", row["id"])
				testutil.Equal(t, "ada@example.com", row["email"])
			},
		},
	}

	for _, rpcCase := range rpcCases {
		rpcCase := rpcCase
		for _, headerCase := range headerCases {
			headerCase := headerCase
			t.Run(rpcCase.name+"/"+headerCase.name, func(t *testing.T) {
				t.Parallel()
				h := newRPCRequestPathHarness()
				w := h.doRPC(rpcCase.function, rpcCase.body, headerCase.headers)
				rpcCase.assertBody(t, w)
				h.assertNoPublish(t)
				if rpcCase.usesExec {
					h.assertExecCalledOnce(t, rpcCase.function)
					return
				}
				h.assertQueryCalledOnce(t, rpcCase.function)
			})
		}
	}
}

func TestRPCHandleRejectsNotifyHeadersBeforeExecution(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"X-Notify-Table":  "  users\t",
		"X-Notify-Action": "  UpDaTe ",
	}

	t.Run("valid_headers_void_no_event", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		w := h.doRPC("cleanup_old_data", "", headers)
		testutil.Equal(t, http.StatusNoContent, w.Code)
		testutil.Equal(t, "", w.Body.String())
		h.assertExecCalledOnce(t, "cleanup_old_data")
		h.assertNoPublish(t)
	})

	t.Run("valid_headers_scalar_read_publishes_one_event", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		w := h.doRPC("add_numbers", `{"a": 1, "b": 2}`, headers)
		testutil.Equal(t, http.StatusOK, w.Code)
		var value float64
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&value))
		testutil.Equal(t, float64(3), value)
		h.assertQueryCalledOnce(t, "add_numbers")
		h.assertPublishedEvents(t, "update", "users", []map[string]any{{"add_numbers": int64(3)}})
	})

	t.Run("valid_headers_single_row_read_publishes_one_event", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		w := h.doRPC("get_profile", "", headers)
		testutil.Equal(t, http.StatusOK, w.Code)
		var row map[string]any
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&row))
		testutil.Equal(t, "u1", row["id"])
		testutil.Equal(t, "ada@example.com", row["email"])
		h.assertQueryCalledOnce(t, "get_profile")
		h.assertPublishedEvents(t, "update", "users", []map[string]any{
			{"id": "u1", "email": "ada@example.com"},
		})
	})

	t.Run("valid_headers_set_read_publishes_per_row", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		w := h.doRPC("get_active_users", `{"min_age": 18}`, headers)
		testutil.Equal(t, http.StatusOK, w.Code)
		var rows []map[string]any
		testutil.NoError(t, json.NewDecoder(w.Body).Decode(&rows))
		testutil.Equal(t, 2, len(rows))
		h.assertQueryCalledOnce(t, "get_active_users")
		h.assertPublishedEvents(t, "update", "users", []map[string]any{
			{"id": "u1", "email": "ada@example.com"},
			{"id": "u2", "email": "grace@example.com"},
		})
	})
}

func TestRPCHandleNotifyHeadersShortCircuitExecution(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"X-Notify-Table":  "users",
		"X-Notify-Action": "update",
	}

	t.Run("query_failure_publishes_nothing", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		h.conn.queryPlans["add_numbers"] = rpcQueryPlan{err: errors.New("query failed")}

		w := h.doRPC("add_numbers", `{"a": 1, "b": 2}`, headers)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Contains(t, decodeError(t, w).Message, "internal error")
		h.assertNoPublish(t)
		h.assertQueryCalledOnce(t, "add_numbers")
	})

	t.Run("scan_failure_scalar_publishes_nothing", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		h.conn.queryPlans["add_numbers"] = rpcQueryPlan{
			rows: &fakeBatchRows{
				cols: []string{"add_numbers"},
				err:  errors.New("scan failed"),
			},
		}

		w := h.doRPC("add_numbers", `{"a": 1, "b": 2}`, headers)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Contains(t, decodeError(t, w).Message, "internal error")
		h.assertNoPublish(t)
		h.assertQueryCalledOnce(t, "add_numbers")
	})

	t.Run("scan_failure_set_publishes_nothing", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		h.conn.queryPlans["get_active_users"] = rpcQueryPlan{
			rows: &fakeBatchRows{
				cols: []string{"id", "email"},
				err:  errors.New("scan failed"),
			},
		}

		w := h.doRPC("get_active_users", `{"min_age": 18}`, headers)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Contains(t, decodeError(t, w).Message, "internal error")
		h.assertNoPublish(t)
		h.assertQueryCalledOnce(t, "get_active_users")
	})

	t.Run("commit_failure_publishes_nothing", func(t *testing.T) {
		t.Parallel()
		h := newRPCRequestPathHarness()
		h.conn.beginTx = &rpcRequestPathTx{
			conn:      h.conn,
			commitErr: errors.New("commit failed"),
		}

		w := h.doRPCWithClaims("add_numbers", `{"a": 1, "b": 2}`, headers)
		testutil.Equal(t, http.StatusInternalServerError, w.Code)
		testutil.Contains(t, decodeError(t, w).Message, "internal error")
		h.assertNoPublish(t)
		h.assertQueryCalledOnce(t, "add_numbers")
	})
}
