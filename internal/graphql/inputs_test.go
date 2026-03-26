package graphql

import (
	"testing"

	gql "github.com/graphql-go/graphql"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildWhereInputFieldsAndOperators(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
	}

	where := buildWhereInput(tbl)
	fields := where.Fields()
	testutil.NotNil(t, fields["id"])
	testutil.NotNil(t, fields["title"])
	testutil.NotNil(t, fields["_and"])
	testutil.NotNil(t, fields["_or"])
	testutil.NotNil(t, fields["_not"])

	idCmp, ok := fields["id"].Type.(*gql.InputObject)
	testutil.True(t, ok, "expected id comparison input object")
	idOps := idCmp.Fields()
	for _, op := range []string{"_eq", "_neq", "_gt", "_gte", "_lt", "_lte", "_in", "_is_null"} {
		testutil.NotNil(t, idOps[op])
	}

	titleCmp, ok := fields["title"].Type.(*gql.InputObject)
	testutil.True(t, ok, "expected title comparison input object")
	titleOps := titleCmp.Fields()
	for _, op := range []string{"_eq", "_neq", "_gt", "_gte", "_lt", "_lte", "_in", "_is_null", "_like", "_ilike"} {
		testutil.NotNil(t, titleOps[op])
	}
}

func TestBuildOrderByInputHasEnumFields(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Schema: "public",
		Name:   "posts",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "title", TypeName: "text"},
		},
	}

	orderBy := buildOrderByInput(tbl)
	fields := orderBy.Fields()
	testutil.NotNil(t, fields["id"])
	testutil.NotNil(t, fields["title"])
	testutil.True(t, fields["id"].Type == OrderByEnum, "id order type should be OrderByEnum")
	testutil.True(t, fields["title"].Type == OrderByEnum, "title order type should be OrderByEnum")
}
