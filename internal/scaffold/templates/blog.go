// Package templates Implements a blog domain template with SQL schema, seed data, TypeScript client code, and user documentation.
package templates

type blogTemplate struct{}

func init() {
	Register(blogTemplate{})
}

func (blogTemplate) Name() string {
	return "blog"
}

// Schema returns the SQL CREATE TABLE statements for the blog domain, including posts, comments, categories, tags, and their associations, with row-level security policies for multi-tenant access control.
func (blogTemplate) Schema() string {
	return `-- Blog domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    body TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'published')),
    author_id UUID REFERENCES _ayb_users(id),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_name TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS post_categories (
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, category_id)
);

CREATE TABLE IF NOT EXISTS tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS post_tags (
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, tag_id)
);

ALTER TABLE posts ENABLE ROW LEVEL SECURITY;
ALTER TABLE comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE post_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE tags ENABLE ROW LEVEL SECURITY;
ALTER TABLE post_tags ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS posts_select ON posts;
CREATE POLICY posts_select ON posts FOR SELECT
    USING (
        status = 'published'
        OR author_id = current_setting('ayb.user_id', true)::uuid
    );

DROP POLICY IF EXISTS posts_insert ON posts;
CREATE POLICY posts_insert ON posts FOR INSERT
    WITH CHECK (author_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS posts_update ON posts;
CREATE POLICY posts_update ON posts FOR UPDATE
    USING (author_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (author_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS posts_delete ON posts;
CREATE POLICY posts_delete ON posts FOR DELETE
    USING (author_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS comments_select ON comments;
CREATE POLICY comments_select ON comments FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM posts p
            WHERE p.id = comments.post_id
              AND p.status = 'published'
        )
    );

DROP POLICY IF EXISTS comments_insert ON comments;
CREATE POLICY comments_insert ON comments FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS categories_select ON categories;
CREATE POLICY categories_select ON categories FOR SELECT
    USING (true);

DROP POLICY IF EXISTS tags_select ON tags;
CREATE POLICY tags_select ON tags FOR SELECT
    USING (true);

DROP POLICY IF EXISTS post_categories_select ON post_categories;
CREATE POLICY post_categories_select ON post_categories FOR SELECT
    USING (true);

DROP POLICY IF EXISTS post_tags_select ON post_tags;
CREATE POLICY post_tags_select ON post_tags FOR SELECT
    USING (true);
`
}

// SeedData returns SQL INSERT statements that populate test users, categories, tags, and sample blog posts with both published and draft status, plus seeded comments.
func (blogTemplate) SeedData() string {
	return `-- Blog domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('11111111-1111-1111-1111-111111111111', 'author.one@example.com', 'seeded-password-hash'),
    ('22222222-2222-2222-2222-222222222222', 'author.two@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO categories (id, name, slug)
VALUES
    ('70000000-0000-0000-0000-000000000001', 'Engineering', 'engineering'),
    ('70000000-0000-0000-0000-000000000002', 'Product', 'product'),
    ('70000000-0000-0000-0000-000000000003', 'Company', 'company')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tags (id, name)
VALUES
    ('80000000-0000-0000-0000-000000000001', 'go'),
    ('80000000-0000-0000-0000-000000000002', 'postgres'),
    ('80000000-0000-0000-0000-000000000003', 'release'),
    ('80000000-0000-0000-0000-000000000004', 'roadmap'),
    ('80000000-0000-0000-0000-000000000005', 'security'),
    ('80000000-0000-0000-0000-000000000006', 'observability'),
    ('80000000-0000-0000-0000-000000000007', 'sdk'),
    ('80000000-0000-0000-0000-000000000008', 'templates'),
    ('80000000-0000-0000-0000-000000000009', 'testing'),
    ('80000000-0000-0000-0000-000000000010', 'performance')
ON CONFLICT (id) DO NOTHING;

INSERT INTO posts (id, title, slug, body, status, author_id, published_at)
VALUES
    ('90000000-0000-0000-0000-000000000001', 'Shipping Domain Templates in AYB', 'shipping-domain-templates', 'How we built reusable domain scaffolds for AYB.', 'published', '11111111-1111-1111-1111-111111111111', now() - interval '9 days'),
    ('90000000-0000-0000-0000-000000000002', 'RLS Patterns for Multi-tenant Apps', 'rls-patterns-multi-tenant', 'Practical RLS rules for secure product teams.', 'published', '22222222-2222-2222-2222-222222222222', now() - interval '6 days'),
    ('90000000-0000-0000-0000-000000000003', 'Draft: Pricing Experiments', 'draft-pricing-experiments', 'Working notes for upcoming pricing changes.', 'draft', '11111111-1111-1111-1111-111111111111', NULL),
    ('90000000-0000-0000-0000-000000000004', 'AYB March Reliability Update', 'march-reliability-update', 'Reliability and performance improvements shipped in March.', 'published', '22222222-2222-2222-2222-222222222222', now() - interval '2 days'),
    ('90000000-0000-0000-0000-000000000005', 'Draft: Internal Launch Checklist', 'draft-internal-launch-checklist', 'Internal pre-launch checklist for release week.', 'draft', '11111111-1111-1111-1111-111111111111', NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO post_categories (post_id, category_id)
VALUES
    ('90000000-0000-0000-0000-000000000001', '70000000-0000-0000-0000-000000000001'),
    ('90000000-0000-0000-0000-000000000002', '70000000-0000-0000-0000-000000000001'),
    ('90000000-0000-0000-0000-000000000002', '70000000-0000-0000-0000-000000000002'),
    ('90000000-0000-0000-0000-000000000003', '70000000-0000-0000-0000-000000000002'),
    ('90000000-0000-0000-0000-000000000004', '70000000-0000-0000-0000-000000000001'),
    ('90000000-0000-0000-0000-000000000004', '70000000-0000-0000-0000-000000000003'),
    ('90000000-0000-0000-0000-000000000005', '70000000-0000-0000-0000-000000000003')
ON CONFLICT (post_id, category_id) DO NOTHING;

INSERT INTO post_tags (post_id, tag_id)
VALUES
    ('90000000-0000-0000-0000-000000000001', '80000000-0000-0000-0000-000000000001'),
    ('90000000-0000-0000-0000-000000000001', '80000000-0000-0000-0000-000000000008'),
    ('90000000-0000-0000-0000-000000000001', '80000000-0000-0000-0000-000000000009'),
    ('90000000-0000-0000-0000-000000000002', '80000000-0000-0000-0000-000000000002'),
    ('90000000-0000-0000-0000-000000000002', '80000000-0000-0000-0000-000000000005'),
    ('90000000-0000-0000-0000-000000000003', '80000000-0000-0000-0000-000000000004'),
    ('90000000-0000-0000-0000-000000000003', '80000000-0000-0000-0000-000000000003'),
    ('90000000-0000-0000-0000-000000000004', '80000000-0000-0000-0000-000000000006'),
    ('90000000-0000-0000-0000-000000000004', '80000000-0000-0000-0000-000000000010'),
    ('90000000-0000-0000-0000-000000000004', '80000000-0000-0000-0000-000000000007'),
    ('90000000-0000-0000-0000-000000000005', '80000000-0000-0000-0000-000000000009')
ON CONFLICT (post_id, tag_id) DO NOTHING;

INSERT INTO comments (id, post_id, author_name, body)
VALUES
    ('a0000000-0000-0000-0000-000000000001', '90000000-0000-0000-0000-000000000001', 'Nina', 'This template structure is exactly what we needed.'),
    ('a0000000-0000-0000-0000-000000000002', '90000000-0000-0000-0000-000000000001', 'Marco', 'Can we get this pattern for ecommerce too?'),
    ('a0000000-0000-0000-0000-000000000003', '90000000-0000-0000-0000-000000000001', 'Priya', 'The RLS examples are very clear.'),
    ('a0000000-0000-0000-0000-000000000004', '90000000-0000-0000-0000-000000000002', 'Helen', 'Great breakdown of tenant-safe SQL.'),
    ('a0000000-0000-0000-0000-000000000005', '90000000-0000-0000-0000-000000000002', 'Jordan', 'Would love a full walkthrough with tests.'),
    ('a0000000-0000-0000-0000-000000000006', '90000000-0000-0000-0000-000000000004', 'Sam', 'Latency is noticeably better after this release.'),
    ('a0000000-0000-0000-0000-000000000007', '90000000-0000-0000-0000-000000000004', 'Ravi', 'Thanks for posting exact migration details.'),
    ('a0000000-0000-0000-0000-000000000008', '90000000-0000-0000-0000-000000000004', 'Alex', 'The benchmarks look solid.'),
    ('a0000000-0000-0000-0000-000000000009', '90000000-0000-0000-0000-000000000002', 'Dana', 'Please publish the SDK snippets in docs.')
ON CONFLICT (id) DO NOTHING;
`
}

// ClientCode returns a map of generated TypeScript client files providing typed interfaces and helper functions for blog CRUD operations wrapped around the ayb.records API.
func (blogTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/blog.ts": `import { ayb } from "./ayb";

export type PostStatus = "draft" | "published";

export interface Post {
  id: string;
  title: string;
  slug: string;
  body: string;
  status: PostStatus;
  author_id: string;
  published_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface Comment {
  id: string;
  post_id: string;
  author_name: string;
  body: string;
  created_at: string;
}

export interface CreatePostInput {
  title: string;
  slug: string;
  body: string;
  status: PostStatus;
  author_id: string;
  published_at?: string | null;
}

export interface UpdatePostInput {
  title?: string;
  slug?: string;
  body?: string;
  status?: PostStatus;
  published_at?: string | null;
}

export interface CreateCommentInput {
  author_name: string;
  body: string;
}

export function listPosts(filter?: string) {
  if (filter) {
    return ayb.records.list("posts", { filter, sort: "-published_at" });
  }
  return ayb.records.list("posts", { sort: "-published_at" });
}

export function getPost(id: string) {
  return ayb.records.get("posts", id);
}

export function createPost(data: CreatePostInput) {
  return ayb.records.create("posts", data);
}

export function updatePost(id: string, data: UpdatePostInput) {
  return ayb.records.update("posts", id, data);
}

export function deletePost(id: string) {
  return ayb.records.delete("posts", id);
}

export function listComments(postId: string) {
  return ayb.records.list("comments", {
    filter: "post_id='" + postId + "'",
    sort: "created_at",
  });
}

export function createComment(postId: string, data: CreateCommentInput) {
  return ayb.records.create("comments", {
    post_id: postId,
    ...data,
  });
}
`,
	}
}

// Readme returns documentation describing the blog template's included schema, setup commands, code examples, and quick-start instructions.
func (blogTemplate) Readme() string {
	return `# Blog Template

This scaffold provisions a blog-ready schema and client helpers.

## Included schema

- ` + "`posts`" + `: article content, publication status, and author ownership
- ` + "`comments`" + `: post comments with cascading deletes from posts
- ` + "`categories`" + ` and ` + "`post_categories`" + `: category taxonomy and post assignment
- ` + "`tags`" + ` and ` + "`post_tags`" + `: tag vocabulary and post assignment

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## Query via generated SDK helpers

` + "```ts" + `
import {
  listPosts,
  getPost,
  createPost,
  updatePost,
  deletePost,
  listComments,
  createComment,
} from "./src/lib/blog";

const { items } = await listPosts("status='published'");
const first = await getPost(items[0].id);
await createComment(first.id, { author_name: "Reader", body: "Great post" });
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data from this project directory.
3. Import helpers from ` + "`src/lib/blog.ts`" + ` in your app code.
4. Build features using the typed CRUD wrappers over ` + "`ayb.records`" + `.
`
}
