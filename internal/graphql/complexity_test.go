package graphql

import (
	"testing"

	gql "github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func mustParseDoc(t *testing.T, query string) *ast.Document {
	t.Helper()
	doc, err := parser.Parse(parser.ParseParams{Source: query})
	testutil.NoError(t, err)
	testutil.NotNil(t, doc)
	return doc
}

func complexityTestSchema(t *testing.T) *gql.Schema {
	t.Helper()
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "title", TypeName: "text"},
			},
			Relationships: []*schema.Relationship{
				{
					Type:      "one-to-many",
					ToSchema:  "public",
					ToTable:   "comments",
					FieldName: "comments",
				},
			},
		},
		{
			Schema: "public",
			Name:   "comments",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "body", TypeName: "text"},
				{Name: "post_id", TypeName: "integer"},
			},
			Relationships: []*schema.Relationship{
				{
					Type:        "many-to-one",
					FromColumns: []string{"post_id"},
					ToSchema:    "public",
					ToTable:     "posts",
					ToColumns:   []string{"id"},
					FieldName:   "post",
				},
			},
		},
	})

	s, err := BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.NotNil(t, s)
	return s
}

func TestCheckDepthAcceptsSimpleQuery(t *testing.T) {
	t.Parallel()
	doc := mustParseDoc(t, `{ posts { id } }`)
	testutil.NoError(t, CheckDepth(doc, 10))
}

func TestCheckDepthAcceptsAtLimit(t *testing.T) {
	t.Parallel()
	doc := mustParseDoc(t, `{ posts { comments { post { id } } } }`)
	testutil.NoError(t, CheckDepth(doc, 4))
}

func TestCheckDepthRejectsOverLimit(t *testing.T) {
	t.Parallel()
	doc := mustParseDoc(t, `{ posts { comments { post { id } } } }`)
	err := CheckDepth(doc, 3)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "query depth 4 exceeds maximum allowed depth of 3")
}

func TestCheckDepthInlineFragmentDoesNotIncreaseDepth(t *testing.T) {
	t.Parallel()
	doc := mustParseDoc(t, `{ posts { ... on posts { id } } }`)
	testutil.NoError(t, CheckDepth(doc, 2))
}

func TestCheckDepthNamedFragment(t *testing.T) {
	t.Parallel()
	doc := mustParseDoc(t, `fragment F on posts { id title } query { posts { ...F } }`)
	testutil.NoError(t, CheckDepth(doc, 2))
}

func TestCheckComplexitySimpleQuery(t *testing.T) {
	t.Parallel()
	s := complexityTestSchema(t)
	doc := mustParseDoc(t, `{ posts(limit: 10) { id title } }`)
	got, err := checkComplexityWithVariables(doc, s, 1000, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 20, got)
}

func TestCheckComplexityNestedQuery(t *testing.T) {
	t.Parallel()
	s := complexityTestSchema(t)
	doc := mustParseDoc(t, `{ posts(limit: 10) { id comments { id } } }`)
	got, err := checkComplexityWithVariables(doc, s, 1000, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 260, got)
}

func TestCheckComplexityOverBudget(t *testing.T) {
	t.Parallel()
	s := complexityTestSchema(t)
	doc := mustParseDoc(t, `{ posts(limit: 50) { id comments { id } } }`)
	_, err := checkComplexityWithVariables(doc, s, 1000, nil)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "query complexity 1300 exceeds maximum allowed complexity of 1000")
}

func TestCheckComplexityIntrospectionFlatCost(t *testing.T) {
	t.Parallel()
	s := complexityTestSchema(t)
	doc := mustParseDoc(t, `{ __schema { types { name } } }`)
	got, err := checkComplexityWithVariables(doc, s, 1000, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, got)
}

func TestCheckDepthDisabledLimit(t *testing.T) {
	t.Parallel()
	doc := mustParseDoc(t, `{ posts { comments { post { comments { id } } } } }`)
	testutil.NoError(t, CheckDepth(doc, 0))
}

func TestCheckComplexityDisabledLimit(t *testing.T) {
	t.Parallel()
	s := complexityTestSchema(t)
	doc := mustParseDoc(t, `{ posts(limit: 200) { id comments { id } } }`)
	_, err := checkComplexityWithVariables(doc, s, 0, nil)
	testutil.NoError(t, err)
}

func TestCheckComplexityWithVariableLimit(t *testing.T) {
	t.Parallel()
	s := complexityTestSchema(t)
	doc := mustParseDoc(t, `query($lim: Int) { posts(limit: $lim) { id title } }`)
	got, err := checkComplexityWithVariables(doc, s, 1000, map[string]interface{}{"lim": 10})
	testutil.NoError(t, err)
	testutil.Equal(t, 20, got)
}
