package sqlutil

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestQuoteIdent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple identifier", input: "users", want: `"users"`},
		{name: "underscore identifier", input: "ayb_authenticated", want: `"ayb_authenticated"`},
		{name: "hyphenated identifier", input: "uuid-ossp", want: `"uuid-ossp"`},
		{name: "identifier with space", input: "user name", want: `"user name"`},
		{name: "empty string", input: "", want: `""`},
		{name: "single embedded double quote", input: `say"hello`, want: `"say""hello"`},
		{name: "multiple embedded double quotes", input: `test"foo"bar`, want: `"test""foo""bar"`},
		{name: "SQL injection attempt", input: `table"; DROP TABLE users; --`, want: `"table""; DROP TABLE users; --"`},
		{name: "SQL injection attempt trailing quote", input: `foo"; DROP TABLE users; --"`, want: `"foo""; DROP TABLE users; --"""`},
		{name: "null byte stripped", input: "hello\x00world", want: `"helloworld"`},
		{name: "null byte with quotes", input: "say\x00\"hi", want: `"say""hi"`},
		{name: "only null bytes", input: "\x00\x00", want: `""`},
		{name: "extension name pg_trgm", input: "pg_trgm", want: `"pg_trgm"`},
		{name: "dot in identifier", input: "with.dot", want: `"with.dot"`},
		{name: "dollar in identifier", input: "with$dollar", want: `"with$dollar"`},
		{name: "schema name with quote", input: `schema"name`, want: `"schema""name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := QuoteIdent(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestQuoteIdentList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{name: "empty slice", input: []string{}, want: ""},
		{name: "single element", input: []string{"id"}, want: `"id"`},
		{name: "multiple elements", input: []string{"id", "name", "email"}, want: `"id", "name", "email"`},
		{name: "element with special chars", input: []string{"col one", `col"two`}, want: `"col one", "col""two"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := QuoteIdentList(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestQuoteQualifiedName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		schema     string
		identifier string
		want       string
	}{
		{name: "basic public.users", schema: "public", identifier: "users", want: `"public"."users"`},
		{name: "custom schema", schema: "myschema", identifier: "orders", want: `"myschema"."orders"`},
		{name: "special chars in schema", schema: `my"schema`, identifier: "table", want: `"my""schema"."table"`},
		{name: "special chars in name", schema: "public", identifier: `user"data`, want: `"public"."user""data"`},
		{name: "empty schema passed explicitly", schema: "", identifier: "t", want: `""."t"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := QuoteQualifiedName(tt.schema, tt.identifier)
			testutil.Equal(t, tt.want, got)
		})
	}
}
