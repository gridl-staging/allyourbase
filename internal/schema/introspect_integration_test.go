//go:build integration

package schema_test

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

// createTestSchema sets up a known test schema with tables, FKs, indexes, and enums.
func createTestSchema(t *testing.T, ctx context.Context) {
	t.Helper()

	sqls := []string{
		// Enum type.
		`CREATE TYPE mood AS ENUM ('happy', 'sad', 'neutral')`,

		// Users table.
		`CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email VARCHAR(255) UNIQUE,
			mood mood DEFAULT 'neutral',
			metadata JSONB,
			tags TEXT[],
			score NUMERIC(10,2),
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Posts table with FK to users.
		`CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			author_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			published BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Comments with FK to both posts and users.
		`CREATE TABLE comments (
			id SERIAL PRIMARY KEY,
			post_id INTEGER REFERENCES posts(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			body TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		// Index on posts.
		`CREATE INDEX idx_posts_author ON posts(author_id)`,
		`CREATE INDEX idx_posts_published ON posts(published) WHERE published = true`,

		// View.
		`CREATE VIEW active_users AS SELECT id, name, email FROM users WHERE is_active = true`,

		// Non-public schema table (should also be introspected).
		`CREATE SCHEMA IF NOT EXISTS app`,
		`CREATE TABLE app.settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, sql := range sqls {
		_, err := sharedPG.Pool.Exec(ctx, sql)
		if err != nil {
			t.Fatalf("creating test schema: %s: %v", sql[:50], err)
		}
	}
}

func TestBuildCacheWithTestSchema(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)
	testutil.NotNil(t, cache)
	testutil.False(t, cache.BuiltAt.IsZero(), "builtAt should be set")

	// Should have tables from public and app schemas.
	testutil.True(t, len(cache.Tables) >= 4, "expected at least 4 tables/views")

	// Check schemas detected.
	testutil.True(t, len(cache.Schemas) >= 1, "expected at least 1 schema")
}

func TestBuildCacheTables(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	// Users table.
	users := cache.Tables["public.users"]
	testutil.NotNil(t, users)
	testutil.Equal(t, "users", users.Name)
	testutil.Equal(t, "public", users.Schema)
	testutil.Equal(t, "table", users.Kind)

	// Posts table.
	posts := cache.Tables["public.posts"]
	testutil.NotNil(t, posts)
	testutil.Equal(t, "posts", posts.Name)

	// View.
	view := cache.Tables["public.active_users"]
	testutil.NotNil(t, view)
	testutil.Equal(t, "view", view.Kind)

	// App schema table.
	settings := cache.Tables["app.settings"]
	testutil.NotNil(t, settings)
	testutil.Equal(t, "app", settings.Schema)
}

func TestBuildCacheColumns(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	users := cache.Tables["public.users"]
	testutil.NotNil(t, users)
	testutil.True(t, len(users.Columns) >= 8, "users should have at least 8 columns")

	// Check specific columns.
	idCol := users.ColumnByName("id")
	testutil.NotNil(t, idCol)
	testutil.Equal(t, "integer", idCol.JSONType)
	testutil.True(t, idCol.IsPrimaryKey, "id should be PK")

	nameCol := users.ColumnByName("name")
	testutil.NotNil(t, nameCol)
	testutil.Equal(t, "string", nameCol.JSONType)
	testutil.False(t, nameCol.IsNullable, "name should not be nullable")

	moodCol := users.ColumnByName("mood")
	testutil.NotNil(t, moodCol)
	testutil.Equal(t, "string", moodCol.JSONType)
	testutil.True(t, moodCol.IsEnum, "mood should be enum")
	testutil.SliceLen(t, moodCol.EnumValues, 3)

	metaCol := users.ColumnByName("metadata")
	testutil.NotNil(t, metaCol)
	testutil.Equal(t, "object", metaCol.JSONType)
	testutil.True(t, metaCol.IsJSON, "metadata should be JSON")

	tagsCol := users.ColumnByName("tags")
	testutil.NotNil(t, tagsCol)
	testutil.Equal(t, "array", tagsCol.JSONType)
	testutil.True(t, tagsCol.IsArray, "tags should be array")

	scoreCol := users.ColumnByName("score")
	testutil.NotNil(t, scoreCol)
	testutil.Equal(t, "number", scoreCol.JSONType)

	activeCol := users.ColumnByName("is_active")
	testutil.NotNil(t, activeCol)
	testutil.Equal(t, "boolean", activeCol.JSONType)

	createdCol := users.ColumnByName("created_at")
	testutil.NotNil(t, createdCol)
	testutil.Equal(t, "string", createdCol.JSONType)
}

func TestBuildCachePrimaryKeys(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	users := cache.Tables["public.users"]
	testutil.NotNil(t, users)
	testutil.SliceLen(t, users.PrimaryKey, 1)
	testutil.Equal(t, "id", users.PrimaryKey[0])

	posts := cache.Tables["public.posts"]
	testutil.NotNil(t, posts)
	testutil.SliceLen(t, posts.PrimaryKey, 1)
	testutil.Equal(t, "id", posts.PrimaryKey[0])

	// app.settings has text PK.
	settings := cache.Tables["app.settings"]
	testutil.NotNil(t, settings)
	testutil.SliceLen(t, settings.PrimaryKey, 1)
	testutil.Equal(t, "key", settings.PrimaryKey[0])
}

func TestBuildCacheForeignKeys(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	posts := cache.Tables["public.posts"]
	testutil.NotNil(t, posts)
	testutil.SliceLen(t, posts.ForeignKeys, 1)

	fk := posts.ForeignKeys[0]
	testutil.SliceLen(t, fk.Columns, 1)
	testutil.Equal(t, "author_id", fk.Columns[0])
	testutil.Equal(t, "users", fk.ReferencedTable)
	testutil.SliceLen(t, fk.ReferencedColumns, 1)
	testutil.Equal(t, "id", fk.ReferencedColumns[0])
	testutil.Equal(t, "CASCADE", fk.OnDelete)

	// Comments should have 2 FKs.
	comments := cache.Tables["public.comments"]
	testutil.NotNil(t, comments)
	testutil.SliceLen(t, comments.ForeignKeys, 2)
}

func TestBuildCacheIndexes(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	posts := cache.Tables["public.posts"]
	testutil.NotNil(t, posts)

	// Should have at least: PK index + idx_posts_author + idx_posts_published.
	testutil.True(t, len(posts.Indexes) >= 3, "posts should have at least 3 indexes")

	// Find the author index.
	var authorIdx *schema.Index
	for _, idx := range posts.Indexes {
		if idx.Name == "idx_posts_author" {
			authorIdx = idx
			break
		}
	}
	testutil.NotNil(t, authorIdx)
	testutil.False(t, authorIdx.IsUnique, "idx_posts_author should not be unique")
	testutil.False(t, authorIdx.IsPrimary, "idx_posts_author should not be primary")
}

func TestBuildCacheRelationships(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	// Posts should have many-to-one to users.
	posts := cache.Tables["public.posts"]
	testutil.NotNil(t, posts)
	testutil.True(t, len(posts.Relationships) >= 1, "posts should have relationships")

	var manyToOne *schema.Relationship
	for _, rel := range posts.Relationships {
		if rel.Type == "many-to-one" && rel.ToTable == "users" {
			manyToOne = rel
			break
		}
	}
	testutil.NotNil(t, manyToOne)
	testutil.Equal(t, "author", manyToOne.FieldName)

	// Users should have one-to-many from posts.
	users := cache.Tables["public.users"]
	testutil.NotNil(t, users)
	testutil.True(t, len(users.Relationships) >= 1, "users should have relationships")

	var oneToMany *schema.Relationship
	for _, rel := range users.Relationships {
		if rel.Type == "one-to-many" && rel.ToTable == "posts" {
			oneToMany = rel
			break
		}
	}
	testutil.NotNil(t, oneToMany)
}

func TestBuildCacheEnums(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	// Should have at least the 'mood' enum.
	testutil.True(t, len(cache.Enums) >= 1, "should have at least 1 enum")

	// Find the mood enum.
	var moodEnum *schema.EnumType
	for _, e := range cache.Enums {
		if e.Name == "mood" {
			moodEnum = e
			break
		}
	}
	testutil.NotNil(t, moodEnum)
	testutil.Equal(t, "public", moodEnum.Schema)
	testutil.SliceLen(t, moodEnum.Values, 3)
	testutil.Equal(t, "happy", moodEnum.Values[0])
	testutil.Equal(t, "sad", moodEnum.Values[1])
	testutil.Equal(t, "neutral", moodEnum.Values[2])
}

func TestBuildCacheExcludesSystemSchemas(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)
	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	// Should NOT contain pg_catalog or information_schema tables.
	for key := range cache.Tables {
		testutil.False(t, strings.HasPrefix(key, "pg_catalog.") || strings.HasPrefix(key, "information_schema."),
			"should not contain system schema tables, found: %s", key)
	}
}

func TestBuildCacheExcludesAYBTables(t *testing.T) {
	ctx := context.Background()
	resetDB(t, ctx)

	// Create AYB system tables.
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE _ayb_test (id SERIAL PRIMARY KEY)`)
	testutil.NoError(t, err)

	createTestSchema(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)

	// Should NOT contain _ayb_ prefixed tables.
	for key := range cache.Tables {
		testutil.False(t, key == "public._ayb_test", "should exclude _ayb_ tables")
	}
}
