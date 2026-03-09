package api

import (
	"context"
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGetOrCreateExpand(t *testing.T) {
	t.Parallel()
	rec := map[string]any{"id": 1, "name": "test"}

	// First call creates the expand map.
	expand := getOrCreateExpand(rec)
	testutil.NotNil(t, expand)

	expand["author"] = map[string]any{"id": 10, "name": "Alice"}

	// Second call returns the same map.
	expand2 := getOrCreateExpand(rec)
	testutil.Equal(t, "Alice", expand2["author"].(map[string]any)["name"])
}

func TestGetOrCreateExpandExisting(t *testing.T) {
	t.Parallel()
	existing := map[string]any{"tags": []string{"go"}}
	rec := map[string]any{"id": 1, "expand": existing}

	// Should return the existing expand map.
	expand := getOrCreateExpand(rec)
	testutil.NotNil(t, expand)

	// The existing map should be the same object.
	expand["author"] = "test"
	testutil.Equal(t, "test", existing["author"])
}

func TestFindRelationByFieldName(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "authors",
			},
		},
	}

	found := findRelation(tbl, "author")
	testutil.NotNil(t, found)
	testutil.Equal(t, "many-to-one", found.Type)
}

func TestFindRelationByColumnName(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "authors",
			},
		},
	}

	found := findRelation(tbl, "author_id")
	testutil.NotNil(t, found)
	testutil.Equal(t, "author", found.FieldName)
}

func TestFindRelationNotFound(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "authors",
			},
		},
	}

	found := findRelation(tbl, "nonexistent")
	testutil.Nil(t, found)
}

func TestFindRelationColumnNameOnlyMatchesManyToOne(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Name:   "users",
		Schema: "public",
		Relationships: []*schema.Relationship{
			{
				FieldName:   "posts",
				Type:        "one-to-many",
				FromColumns: []string{"id"},
				ToColumns:   []string{"author_id"},
				ToSchema:    "public",
				ToTable:     "posts",
			},
		},
	}

	// Column name fallback should NOT match one-to-many relationships.
	found := findRelation(tbl, "id")
	testutil.Nil(t, found)

	// But field name still matches.
	found = findRelation(tbl, "posts")
	testutil.NotNil(t, found)
	testutil.Equal(t, "one-to-many", found.Type)
}

func TestCountKnownColumns(t *testing.T) {
	t.Parallel()
	tbl := &schema.Table{
		Columns: []*schema.Column{
			{Name: "id"},
			{Name: "name"},
			{Name: "email"},
		},
	}

	tests := []struct {
		name string
		data map[string]any
		want int
	}{
		{"all known", map[string]any{"name": "a", "email": "b"}, 2},
		{"some known", map[string]any{"name": "a", "fake": "b"}, 1},
		{"none known", map[string]any{"fake1": "a", "fake2": "b"}, 0},
		{"empty", map[string]any{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := countKnownColumns(tbl, tt.data)
			testutil.Equal(t, tt.want, got)
		})
	}
}

// TestExpandRelationSkipsRestrictedTable verifies that expandRelation does not
// attach expand data when claims restrict access to the related table.
// This prevents API key scope bypass via expand parameters.
func TestExpandRelationSkipsRestrictedTable(t *testing.T) {
	t.Parallel()
	postsTable := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Columns: []*schema.Column{
			{Name: "id"},
			{Name: "author_id"},
		},
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "users",
			},
		},
	}
	usersTable := &schema.Table{
		Name:   "users",
		Schema: "public",
		Columns: []*schema.Column{
			{Name: "id"},
			{Name: "email"},
		},
	}

	sc := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.posts": postsTable,
			"public.users": usersTable,
		},
	}

	logger := slog.New(slog.NewTextHandler(nil, nil))

	// Claims that allow access to "posts" but NOT "users".
	claims := &auth.Claims{AllowedTables: []string{"posts"}}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	records := []map[string]any{
		{"id": 1, "author_id": 10},
	}

	// expandRelation should return early due to table scope check,
	// without attempting any query (pool is nil — would panic if queried).
	expandRelation(ctx, nil, sc, postsTable, records, []string{"author"}, 0, claims, logger)

	// No "expand" key should be attached since the claims forbid access to "users".
	_, hasExpand := records[0]["expand"]
	testutil.False(t, hasExpand, "expand should not be attached when claims restrict the related table")
}

// TestExpandRelationAllowsUnrestrictedTable verifies that expandRelation
// proceeds past the table scope check when claims are unrestricted.
// It uses the same nil-pool trick as TestExpandRelationSkipsRestrictedTable:
// if the scope check passes, expandRelation will attempt a query on the nil
// pool and panic. We recover from the panic to prove it got past the guard.
func TestExpandRelationAllowsUnrestrictedTable(t *testing.T) {
	t.Parallel()
	postsTable := &schema.Table{
		Name:   "posts",
		Schema: "public",
		Columns: []*schema.Column{
			{Name: "id"},
			{Name: "author_id"},
		},
		Relationships: []*schema.Relationship{
			{
				FieldName:   "author",
				Type:        "many-to-one",
				FromColumns: []string{"author_id"},
				ToColumns:   []string{"id"},
				ToSchema:    "public",
				ToTable:     "users",
			},
		},
	}
	usersTable := &schema.Table{
		Name:   "users",
		Schema: "public",
		Columns: []*schema.Column{
			{Name: "id"},
			{Name: "email"},
		},
	}

	sc := &schema.SchemaCache{
		Tables: map[string]*schema.Table{
			"public.posts": postsTable,
			"public.users": usersTable,
		},
	}

	logger := slog.New(slog.NewTextHandler(nil, nil))

	records := []map[string]any{
		{"id": 1, "author_id": 10},
	}

	// Nil claims (unauthenticated) should pass the scope check and proceed
	// to the query step, where the nil pool causes a panic.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		expandRelation(context.Background(), nil, sc, postsTable, records, []string{"author"}, 0, nil, logger)
	}()
	testutil.True(t, panicked, "nil claims: expected panic from nil pool query, meaning scope check passed")

	// Full-access API key should also pass scope check and reach the query.
	panicked = false
	records2 := []map[string]any{
		{"id": 2, "author_id": 20},
	}
	claims := &auth.Claims{APIKeyScope: "*", AllowedTables: nil}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		expandRelation(ctx, nil, sc, postsTable, records2, []string{"author"}, 0, claims, logger)
	}()
	testutil.True(t, panicked, "full-access claims: expected panic from nil pool query, meaning scope check passed")
}
