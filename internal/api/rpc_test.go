package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
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
