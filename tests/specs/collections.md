# Collections Test Specification

**User Stories:** B-COLL-001 through B-COLL-010
**Read this file BEFORE implementing collections tests**

---

## Overview

Collections (auto-generated CRUD API) has three test levels:
1. **Integration tests** (Go) — Test API endpoints with real Postgres
2. **Unit tests** (SDK) — Test SDK collections methods with mocked fetch
3. **Browser tests (unmocked) tests** (Playwright) — Test full CRUD flow through admin UI

**This spec focuses on integration and Browser tests (unmocked) tests.**

---

## <a id="list-records"></a>TEST: List Records (Integration)

**BDD Story:** B-COLL-001
**Type:** Integration test
**File:** `internal/api/handler_integration_test.go`

### Prerequisites
- Test database with `posts` table

### Test Cases

#### 1. List Records with Default Pagination

**Fixture:** `tests/fixtures/posts/list-posts.json`
```json
{
  "metadata": {
    "description": "25 test posts for pagination testing",
    "expected_response_status": 200,
    "expected_default_page": 1,
    "expected_default_per_page": 20,
    "expected_total_items": 25,
    "expected_total_pages": 2,
    "expected_items_first_page": 20
  },
  "posts": [
    {"title": "Post 1", "content": "Content 1"},
    {"title": "Post 2", "content": "Content 2"},
    ...
    {"title": "Post 25", "content": "Content 25"}
  ]
}
```

**Execute:**
```go
// Setup: Create 25 test posts
fixture := loadFixture("posts/list-posts.json")
for _, post := range fixture.Posts {
    db.Exec("INSERT INTO posts (title, content) VALUES ($1, $2)", post.Title, post.Content)
}

// List records (default pagination)
resp := makeRequest("GET", "/api/collections/posts", nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.Equal(t, fixture.Metadata.ExpectedDefaultPage, int(body["page"].(float64)))
testutil.Equal(t, fixture.Metadata.ExpectedDefaultPerPage, int(body["perPage"].(float64)))
testutil.Equal(t, fixture.Metadata.ExpectedTotalItems, int(body["totalItems"].(float64)))
testutil.Equal(t, fixture.Metadata.ExpectedTotalPages, int(body["totalPages"].(float64)))

items := body["items"].([]interface{})
testutil.Equal(t, fixture.Metadata.ExpectedItemsFirstPage, len(items))
```

**Cleanup:** Delete test posts

---

#### 2. List Records from Non-Existent Table

**Fixture:** `tests/fixtures/posts/list-non-existent.json`
```json
{
  "metadata": {
    "description": "Request to non-existent table",
    "expected_response_status": 404,
    "expected_error_message": "table not found"
  },
  "table_name": "non_existent_table"
}
```

**Execute & Verify:** Return 404 with error message

---

## <a id="filter-records"></a>TEST: Filter Records (Integration)

**BDD Story:** B-COLL-002
**Type:** Integration test

### Test Cases

#### 1. Filter with Simple Condition

**Fixture:** `tests/fixtures/posts/filter-status.json`
```json
{
  "metadata": {
    "description": "Filter posts by status='published'",
    "expected_response_status": 200,
    "expected_item_count": 2,
    "expected_all_match_status": "published"
  },
  "posts": [
    {"title": "Post 1", "content": "Content 1", "status": "published"},
    {"title": "Post 2", "content": "Content 2", "status": "draft"},
    {"title": "Post 3", "content": "Content 3", "status": "published"}
  ],
  "filter": "status='published'"
}
```

**Execute:**
```go
// Setup: Create posts with different statuses
fixture := loadFixture("posts/filter-status.json")
for _, post := range fixture.Posts {
    db.Exec("INSERT INTO posts (title, content, status) VALUES ($1, $2, $3)",
        post.Title, post.Content, post.Status)
}

// Filter by status
resp := makeRequest("GET", "/api/collections/posts?filter="+url.QueryEscape(fixture.Filter), nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

items := body["items"].([]interface{})
testutil.Equal(t, fixture.Metadata.ExpectedItemCount, len(items))

// Verify all items match filter
for _, item := range items {
    post := item.(map[string]interface{})
    testutil.Equal(t, fixture.Metadata.ExpectedAllMatchStatus, post["status"])
}
```

**Cleanup:** Delete test posts

---

#### 2. Filter with Complex Conditions (AND, OR)

**Fixture:** `tests/fixtures/posts/filter-complex.json`
```json
{
  "metadata": {
    "description": "Filter with AND/OR: status='published' AND views>100",
    "expected_response_status": 200,
    "expected_item_count": 1
  },
  "posts": [
    {"title": "Post 1", "status": "published", "views": 150},
    {"title": "Post 2", "status": "published", "views": 50},
    {"title": "Post 3", "status": "draft", "views": 200}
  ],
  "filter": "status='published' AND views>100"
}
```

**Execute & Verify:** Same pattern as above

---

#### 3. Filter with Invalid Syntax (SQL Injection Protection)

**Fixture:** `tests/fixtures/posts/filter-sql-injection.json`
```json
{
  "metadata": {
    "description": "Attempt SQL injection via filter",
    "expected_response_status": 400,
    "expected_error_message": "invalid filter syntax"
  },
  "filter": "1=1; DROP TABLE posts;--"
}
```

**Execute & Verify:** Return 400 with error (SQL injection blocked)

---

## <a id="sort-records"></a>TEST: Sort Records (Integration)

**BDD Story:** B-COLL-003
**Type:** Integration test

### Test Cases

#### 1. Sort by Single Field (Descending)

**Fixture:** `tests/fixtures/posts/sort-created-at.json`
```json
{
  "metadata": {
    "description": "Sort posts by created_at descending",
    "expected_response_status": 200,
    "expected_first_title": "Post 3",
    "expected_last_title": "Post 1"
  },
  "posts": [
    {"title": "Post 1", "created_at": "2024-01-01T00:00:00Z"},
    {"title": "Post 2", "created_at": "2024-01-02T00:00:00Z"},
    {"title": "Post 3", "created_at": "2024-01-03T00:00:00Z"}
  ],
  "sort": "-created_at"
}
```

**Execute:**
```go
// Setup: Create posts with different timestamps
fixture := loadFixture("posts/sort-created-at.json")
for _, post := range fixture.Posts {
    db.Exec("INSERT INTO posts (title, created_at) VALUES ($1, $2)",
        post.Title, post.CreatedAt)
}

// Sort by created_at descending
resp := makeRequest("GET", "/api/collections/posts?sort="+fixture.Sort, nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

items := body["items"].([]interface{})
firstItem := items[0].(map[string]interface{})
lastItem := items[len(items)-1].(map[string]interface{})

testutil.Equal(t, fixture.Metadata.ExpectedFirstTitle, firstItem["title"])
testutil.Equal(t, fixture.Metadata.ExpectedLastTitle, lastItem["title"])
```

**Cleanup:** Delete test posts

---

#### 2. Sort by Multiple Fields

**Fixture:** `tests/fixtures/posts/sort-multi-field.json`
```json
{
  "metadata": {
    "description": "Sort by status ascending, then created_at descending",
    "expected_response_status": 200,
    "expected_order": ["Post 2", "Post 3", "Post 1"]
  },
  "posts": [
    {"title": "Post 1", "status": "published", "created_at": "2024-01-03T00:00:00Z"},
    {"title": "Post 2", "status": "draft", "created_at": "2024-01-02T00:00:00Z"},
    {"title": "Post 3", "status": "published", "created_at": "2024-01-01T00:00:00Z"}
  ],
  "sort": "+status,-created_at"
}
```

**Execute & Verify:** Same pattern as above

---

## <a id="search-records"></a>TEST: Search Records (Integration)

**BDD Story:** B-COLL-004
**Type:** Integration test

### Test Cases

#### 1. Full-Text Search

**Fixture:** `tests/fixtures/posts/search-keywords.json`
```json
{
  "metadata": {
    "description": "Search for posts containing 'postgres database'",
    "expected_response_status": 200,
    "expected_item_count": 2
  },
  "posts": [
    {"title": "PostgreSQL Tips", "content": "Learn about postgres database optimization"},
    {"title": "MySQL Guide", "content": "MySQL is another database system"},
    {"title": "Database Design", "content": "How to design a postgres database schema"}
  ],
  "search": "postgres database"
}
```

**Execute:**
```go
// Setup: Create posts
fixture := loadFixture("posts/search-keywords.json")
for _, post := range fixture.Posts {
    db.Exec("INSERT INTO posts (title, content) VALUES ($1, $2)",
        post.Title, post.Content)
}

// Search
resp := makeRequest("GET", "/api/collections/posts?search="+url.QueryEscape(fixture.Search), nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

items := body["items"].([]interface{})
testutil.Equal(t, fixture.Metadata.ExpectedItemCount, len(items))
```

**Cleanup:** Delete test posts

---

## <a id="expand-foreign-keys"></a>TEST: Expand Foreign Keys (Integration)

**BDD Story:** B-COLL-005
**Type:** Integration test

### Test Cases

#### 1. Expand Single FK

**Fixture:** `tests/fixtures/posts/expand-author.json`
```json
{
  "metadata": {
    "description": "Expand author FK in posts",
    "expected_response_status": 200,
    "expected_expanded_fields": ["id", "name", "email"]
  },
  "author": {
    "name": "Jane Doe",
    "email": "jane@example.com"
  },
  "post": {
    "title": "My Post",
    "content": "Post content"
  },
  "expand": "author"
}
```

**Execute:**
```go
// Setup: Create author and post
fixture := loadFixture("posts/expand-author.json")
authorID := createTestAuthor(t, db, fixture.Author.Name, fixture.Author.Email)
postID := createTestPost(t, db, fixture.Post.Title, fixture.Post.Content, authorID)

// Expand author FK
resp := makeRequest("GET", "/api/collections/posts/"+postID+"?expand="+fixture.Expand, nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

// Verify author is expanded (not just ID)
author := body["author"].(map[string]interface{})
testutil.Equal(t, fixture.Author.Name, author["name"])
testutil.Equal(t, fixture.Author.Email, author["email"])
```

**Cleanup:** Delete test data

---

## <a id="create-record"></a>TEST: Create Record (Integration)

**BDD Story:** B-COLL-006
**Type:** Integration test

### Test Cases

#### 1. Create Record with Valid Data

**Fixture:** `tests/fixtures/posts/create-valid.json`
```json
{
  "metadata": {
    "description": "Create new post with valid data",
    "expected_response_status": 201,
    "expected_fields": ["id", "title", "content", "created_at"]
  },
  "title": "New Post",
  "content": "This is a new post"
}
```

**Execute:**
```go
fixture := loadFixture("posts/create-valid.json")
resp := makeRequest("POST", "/api/collections/posts", fixture)
```

**Verify:**
```go
testutil.Equal(t, 201, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.Equal(t, fixture.Title, body["title"])
testutil.Equal(t, fixture.Content, body["content"])
testutil.True(t, body["id"] != nil)
testutil.True(t, body["created_at"] != nil)
```

**Cleanup:** Delete created post

---

#### 2. Create Record with NOT NULL Violation

**Fixture:** `tests/fixtures/posts/create-null-violation.json`
```json
{
  "metadata": {
    "description": "Attempt to create post without required title",
    "expected_response_status": 422,
    "expected_error_message": "title cannot be null"
  },
  "content": "Post content without title"
}
```

**Execute & Verify:** Return 422 with error

---

#### 3. Create Record with Unique Constraint Violation

**Fixture:** `tests/fixtures/posts/create-unique-violation.json`
```json
{
  "metadata": {
    "description": "Attempt to create post with duplicate unique field",
    "expected_response_status": 409,
    "expected_error_message": "unique constraint violation"
  },
  "slug": "duplicate-slug",
  "title": "Test Post",
  "content": "Content"
}
```

**Execute & Verify:** Return 409 with error

---

## <a id="get-single-record"></a>TEST: Get Single Record (Integration)

**BDD Story:** B-COLL-007
**Type:** Integration test

### Test Cases

#### 1. Get Existing Record

**Fixture:** `tests/fixtures/posts/get-single.json`
```json
{
  "metadata": {
    "description": "Get single post by ID",
    "expected_response_status": 200
  },
  "title": "Test Post",
  "content": "Test content"
}
```

**Execute:**
```go
// Setup: Create post
fixture := loadFixture("posts/get-single.json")
postID := createTestPost(t, db, fixture.Title, fixture.Content)

// Get post
resp := makeRequest("GET", "/api/collections/posts/"+postID, nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.Equal(t, fixture.Title, body["title"])
testutil.Equal(t, fixture.Content, body["content"])
```

**Cleanup:** Delete test post

---

#### 2. Get Non-Existent Record

**Fixture:** `tests/fixtures/posts/get-non-existent.json`
```json
{
  "metadata": {
    "description": "Get post with non-existent ID",
    "expected_response_status": 404,
    "expected_error_message": "record not found"
  },
  "id": "99999"
}
```

**Execute & Verify:** Return 404 with error

---

## <a id="update-record"></a>TEST: Update Record (Integration)

**BDD Story:** B-COLL-008
**Type:** Integration test

### Test Cases

#### 1. Update Record with Valid Data

**Fixture:** `tests/fixtures/posts/update-valid.json`
```json
{
  "metadata": {
    "description": "Update post with new title",
    "expected_response_status": 200,
    "expected_updated_title": "Updated Title"
  },
  "original_title": "Original Title",
  "original_content": "Original content",
  "updates": {
    "title": "Updated Title"
  }
}
```

**Execute:**
```go
// Setup: Create post
fixture := loadFixture("posts/update-valid.json")
postID := createTestPost(t, db, fixture.OriginalTitle, fixture.OriginalContent)

// Update post
resp := makeRequest("PATCH", "/api/collections/posts/"+postID, fixture.Updates)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.Equal(t, fixture.Metadata.ExpectedUpdatedTitle, body["title"])
testutil.Equal(t, fixture.OriginalContent, body["content"]) // Unchanged
```

**Cleanup:** Delete test post

---

## <a id="delete-record"></a>TEST: Delete Record (Integration)

**BDD Story:** B-COLL-009
**Type:** Integration test

### Test Cases

#### 1. Delete Existing Record

**Fixture:** `tests/fixtures/posts/delete-valid.json`
```json
{
  "metadata": {
    "description": "Delete existing post",
    "expected_response_status": 204
  },
  "title": "Post to Delete",
  "content": "This will be deleted"
}
```

**Execute:**
```go
// Setup: Create post
fixture := loadFixture("posts/delete-valid.json")
postID := createTestPost(t, db, fixture.Title, fixture.Content)

// Delete post
resp := makeRequest("DELETE", "/api/collections/posts/"+postID, nil)
```

**Verify:**
```go
testutil.Equal(t, 204, resp.StatusCode)

// Verify post is deleted
var count int
db.QueryRow("SELECT COUNT(*) FROM posts WHERE id = $1", postID).Scan(&count)
testutil.Equal(t, 0, count)
```

---

## <a id="batch-operations"></a>TEST: Batch Operations (Integration)

**BDD Story:** B-COLL-010
**Type:** Integration test

### Test Cases

#### 1. Batch Create

**Fixture:** `tests/fixtures/posts/batch-create.json`
```json
{
  "metadata": {
    "description": "Batch create 3 posts",
    "expected_response_status": 200,
    "expected_success_count": 3
  },
  "operations": [
    {"method": "create", "body": {"title": "Post 1", "content": "Content 1"}},
    {"method": "create", "body": {"title": "Post 2", "content": "Content 2"}},
    {"method": "create", "body": {"title": "Post 3", "content": "Content 3"}}
  ]
}
```

**Execute:**
```go
fixture := loadFixture("posts/batch-create.json")
resp := makeRequest("POST", "/api/collections/posts/batch", fixture)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body []map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.Equal(t, fixture.Metadata.ExpectedSuccessCount, len(body))

// Verify all succeeded
for _, result := range body {
    testutil.Equal(t, 201, int(result["status"].(float64)))
}
```

**Cleanup:** Delete created posts

---

## Browser tests (unmocked) Tests

**Location:** `ui/browser-tests-unmocked/full/collections-crud.spec.ts`
**Purpose:** Test full CRUD flow through admin UI

### Test Cases

#### 1. Create Table and Add Record

**Execute:**
1. Navigate to admin dashboard
2. Execute SQL to create table
3. Refresh and navigate to table
4. Click "New" button
5. Fill in form and submit
6. Verify record appears

**Verify:**
- Table appears in sidebar
- Record visible in data view
- All fields populated correctly

#### 2. Edit Record

**Execute:**
1. Click on record row
2. Click "Edit" button
3. Modify field values
4. Click "Save"

**Verify:**
- Updated values visible in table
- Changes persisted in database

#### 3. Delete Record

**Execute:**
1. Click on record row
2. Click "Delete" button
3. Confirm deletion

**Verify:**
- Record removed from table
- Empty state shown (if no records left)

---

## Fixture Data Needed

**Create these fixtures in `tests/fixtures/posts/`:**

1. `list-posts.json` — 25 posts for pagination
2. `filter-status.json` — Posts with different statuses
3. `filter-complex.json` — Posts for AND/OR filtering
4. `filter-sql-injection.json` — SQL injection attempt
5. `sort-created-at.json` — Posts with different timestamps
6. `sort-multi-field.json` — Posts for multi-field sorting
7. `search-keywords.json` — Posts for full-text search
8. `expand-author.json` — Post with author FK
9. `create-valid.json` — Valid post data
10. `create-null-violation.json` — Missing required field
11. `create-unique-violation.json` — Duplicate unique field
12. `get-single.json` — Single post data
13. `get-non-existent.json` — Non-existent ID
14. `update-valid.json` — Post update data
15. `delete-valid.json` — Post to delete
16. `batch-create.json` — Batch operations

---

**Spec Version:** 1.0
**Last Updated:** 2026-02-13 (Session 078)
