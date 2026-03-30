package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

// FuzzDecodeRPCArgsJSON exercises the JSON decoding path that converts raw
// request bodies into RPC argument maps. The goal is to catch panics or
// unbounded allocations on adversarial input. We feed the raw string through
// json.Unmarshal (mirroring decodeRPCArgs) and then, if it decodes to a
// valid map, pass it through buildRPCCall and coerceRPCArg to shake out
// type-confusion bugs in the coercion layer.
func FuzzDecodeRPCArgsJSON(f *testing.F) {
	// Seed corpus: representative JSON payloads that exercise different
	// decoding branches (empty, valid objects, arrays, nested, edge types).
	seeds := []string{
		`{}`,
		`{"a": 1}`,
		`{"a": 1, "b": "hello"}`,
		`{"a": [1, 2, 3]}`,
		`{"a": null}`,
		`{"a": true, "b": false}`,
		`{"a": 1.5, "b": -3.14}`,
		`{"a": "text", "b": [1, "two", null]}`,
		`{"a": {"nested": "object"}}`,
		`{"a": 9999999999999999999}`,
		`{"a": 1e308}`,
		`{"a": -1e308}`,
		`{"a": 0.0}`,
		`[1, 2, 3]`,       // array — not a valid map, should be handled gracefully
		`"just a string"`, // scalar — not a map
		`null`,            // null — not a map
		``,                // empty body
		`{`,               // truncated JSON
		`{"a": }`,         // malformed value
		`{"a": "\x00"}`,   // null byte in string
		`{"a": "` + strings.Repeat("x", 1024) + `"}`,                // large value
		strings.Repeat(`{"a":`, 50) + `1` + strings.Repeat(`}`, 50), // deeply nested
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	// Test functions with various parameter types to exercise all
	// coercion paths in coerceRPCArg / coerceArray / coerceNumber.
	testFunctions := []*schema.Function{
		{
			Schema: "public", Name: "scalar_int", ReturnType: "integer",
			Parameters: []*schema.FuncParam{
				{Name: "a", Type: "integer", Position: 1},
			},
		},
		{
			Schema: "public", Name: "multi_type", ReturnType: "text",
			Parameters: []*schema.FuncParam{
				{Name: "a", Type: "integer", Position: 1},
				{Name: "b", Type: "text", Position: 2},
				{Name: "c", Type: "boolean", Position: 3},
			},
		},
		{
			Schema: "public", Name: "array_params", ReturnType: "void", IsVoid: true,
			Parameters: []*schema.FuncParam{
				{Name: "ids", Type: "integer[]", Position: 1},
				{Name: "names", Type: "text[]", Position: 2},
			},
		},
		{
			Schema: "public", Name: "bigint_float", ReturnType: "record",
			Parameters: []*schema.FuncParam{
				{Name: "big", Type: "bigint", Position: 1},
				{Name: "dbl", Type: "double precision", Position: 2},
				{Name: "sm", Type: "smallint", Position: 3},
			},
		},
		{
			// No parameters — exercises the empty-args path.
			Schema: "public", Name: "no_params", ReturnType: "void", IsVoid: true,
		},
		{
			// Variadic function — exercises the VARIADIC cast path.
			Schema: "public", Name: "variadic_fn", ReturnType: "text",
			Parameters: []*schema.FuncParam{
				{Name: "items", Type: "text[]", Position: 1, IsVariadic: true},
			},
		},
		{
			// Set-returning function — exercises SELECT * FROM path.
			Schema: "public", Name: "set_fn", ReturnType: "SETOF record",
			ReturnsSet: true,
			Parameters: []*schema.FuncParam{
				{Name: "filter", Type: "text", Position: 1},
			},
		},
		{
			// OUT params — exercises the HasOutParams path.
			Schema: "public", Name: "out_fn", ReturnType: "record",
			HasOutParams: true,
			Parameters: []*schema.FuncParam{
				{Name: "input", Type: "integer", Position: 1},
			},
		},
		{
			// Bool array — exercises the boolean[] coercion path.
			Schema: "public", Name: "bool_array_fn", ReturnType: "void", IsVoid: true,
			Parameters: []*schema.FuncParam{
				{Name: "flags", Type: "boolean[]", Position: 1},
			},
		},
		{
			// Float arrays — exercises float32/float64 array coercion.
			Schema: "public", Name: "float_arrays", ReturnType: "void", IsVoid: true,
			Parameters: []*schema.FuncParam{
				{Name: "reals", Type: "real[]", Position: 1},
				{Name: "doubles", Type: "float8[]", Position: 2},
			},
		},
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Cap input size to avoid OOM on huge fuzzer-generated strings.
		if len(input) > 8192 {
			t.Skip()
		}

		// Try to decode the input as a JSON map, mirroring what
		// decodeRPCArgs does with json.NewDecoder(r.Body).Decode(&args).
		var args map[string]any
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			// Invalid JSON — that's fine, decodeRPCArgs would return 400.
			return
		}

		// Feed the decoded args through buildRPCCall for each test function.
		// This exercises the coercion layer that converts JSON-decoded values
		// to Go types suitable for pgx.
		for _, fn := range testFunctions {
			_, _, _ = buildRPCCall(fn, args)
		}

		// Also exercise coerceRPCArg directly with various PG types to catch
		// any type assertion panics in the coercion helpers. We iterate all
		// values from the decoded map against every target PG type to maximize
		// coverage of the type-switch branches.
		pgTypes := []string{
			"integer", "int4", "smallint", "int2", "bigint", "int8",
			"real", "float4", "double precision", "float8",
			"text", "varchar", "character varying", "name",
			"boolean", "bool",
			"integer[]", "text[]", "bigint[]", "boolean[]",
			"real[]", "float8[]",
			"jsonb", "uuid", "timestamp",
		}
		for _, val := range args {
			for _, pgType := range pgTypes {
				_ = coerceRPCArg(val, pgType)
			}
		}
	})
}
