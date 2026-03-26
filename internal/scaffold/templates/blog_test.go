package templates

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBlogSchemaContainsAllTables(t *testing.T) {
	t.Parallel()
	dt := mustBlogTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS posts")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS comments")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS categories")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS post_categories")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS tags")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS post_tags")
}

func TestBlogSchemaContainsRLS(t *testing.T) {
	t.Parallel()
	dt := mustBlogTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "ALTER TABLE posts ENABLE ROW LEVEL SECURITY")
	testutil.Contains(t, schema, "posts_select")
	testutil.Contains(t, schema, "posts_insert")
	testutil.Contains(t, schema, "posts_update")
	testutil.Contains(t, schema, "posts_delete")
	testutil.Contains(t, schema, "comments_select")
	testutil.Contains(t, schema, "comments_insert")
}

func TestBlogSchemaUsesUUIDSessionCast(t *testing.T) {
	t.Parallel()
	dt := mustBlogTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "current_setting('ayb.user_id', true)::uuid")
	testutil.False(t, strings.Contains(schema, "::text"))
}

func TestBlogSeedDataContainsExpectedInserts(t *testing.T) {
	t.Parallel()
	dt := mustBlogTemplate(t)

	seed := dt.SeedData()
	testutil.Contains(t, seed, "INSERT INTO _ayb_users")
	testutil.Contains(t, seed, "ON CONFLICT DO NOTHING")
	testutil.Contains(t, seed, "INSERT INTO posts")
	testutil.Contains(t, seed, "INSERT INTO comments")
	testutil.Contains(t, seed, "INSERT INTO categories")
	testutil.Contains(t, seed, "INSERT INTO tags")
	testutil.Contains(t, seed, "INSERT INTO post_categories")
	testutil.Contains(t, seed, "INSERT INTO post_tags")
}

func TestBlogClientCodeContainsHelpers(t *testing.T) {
	t.Parallel()
	dt := mustBlogTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/blog.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "listPosts")
	testutil.Contains(t, code, "createPost")
	testutil.Contains(t, code, "listComments")
}

func TestNamesIncludesBlog(t *testing.T) {
	t.Parallel()
	names := Names()
	testutil.True(t, len(names) > 0)
	found := false
	for _, name := range names {
		if name == "blog" {
			found = true
			break
		}
	}
	testutil.True(t, found)
}

func mustBlogTemplate(t *testing.T) DomainTemplate {
	t.Helper()
	return mustTemplate(t, "blog")
}
