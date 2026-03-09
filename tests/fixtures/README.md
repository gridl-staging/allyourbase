# Test Fixtures

**Purpose:** Test data with expected values metadata for fixture-driven testing

---

## Structure

```
tests/fixtures/
├── README.md          # This file
├── users/             # Auth user fixtures
├── posts/             # Collections (posts) fixtures
├── oauth/             # OAuth provider fixtures
└── storage/           # Storage (file upload) fixtures
```

---

## Fixture Format

**All fixtures MUST include metadata with expected values.**

### Metadata Fields

- **description** — Human-readable description of what this fixture tests
- **expected_response_status** — Expected HTTP status code (200, 201, 400, 404, etc.)
- **expected_*_fields** — Array of expected response fields
- **expected_*_message** — Expected error message substring
- **expected_*_code** — Expected error code
- **test_tags** — Array of tags for categorization (e.g., ["auth", "registration", "happy-path"])

### Example Fixture

```json
{
  "metadata": {
    "description": "Valid user registration with email and password",
    "expected_response_status": 201,
    "expected_user_fields": ["id", "email", "created_at"],
    "expected_token_type": "Bearer",
    "test_tags": ["auth", "registration", "happy-path"]
  },
  "email": "test@example.com",
  "password": "SecurePass123!"
}
```

---

## Usage in Tests

### Go Integration Tests

```go
import "github.com/allyourbase/ayb/internal/testutil"

func TestUserRegistration(t *testing.T) {
    fixture := testutil.LoadFixture("users/valid-user.json")

    resp := makeRequest("POST", "/api/auth/register", fixture)

    // Use fixture metadata for assertions
    testutil.Equal(t, fixture.Metadata.ExpectedResponseStatus, resp.StatusCode)

    var body map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&body)

    // Verify expected fields exist
    for _, field := range fixture.Metadata.ExpectedUserFields {
        testutil.True(t, body["user"].(map[string]interface{})[field] != nil)
    }
}
```

### TypeScript Tests

```typescript
import fixture from '../fixtures/users/valid-user.json';

test('user registration', async () => {
  const response = await client.auth.register(fixture.email, fixture.password);

  // Use fixture metadata for assertions
  expect(response.status).toBe(fixture.metadata.expected_response_status);

  fixture.metadata.expected_user_fields.forEach(field => {
    expect(response.data.user).toHaveProperty(field);
  });
});
```

---

## Rules

1. **Never hard-code expected values in tests** — Use fixture metadata
2. **One fixture per test scenario** — Each fixture tests one specific case
3. **Include cleanup data** — Fixtures should be self-contained with data needed for cleanup
4. **Realistic test data** — Use realistic emails, names, content (not "test1", "test2")
5. **Reusable fixtures** — Design fixtures to be reusable across multiple tests when appropriate

---

## Fixture Categories

### Happy Path Fixtures
Test successful operations with valid data.

**Examples:**
- `users/valid-user.json` — Valid registration
- `posts/create-valid.json` — Valid post creation
- `storage/upload-image.json` — Valid file upload

### Error Handling Fixtures
Test error cases with invalid data.

**Examples:**
- `users/duplicate-email.json` — Unique constraint violation
- `users/weak-password.json` — Validation error
- `posts/create-null-violation.json` — NOT NULL constraint violation

### Edge Case Fixtures
Test boundary conditions and edge cases.

**Examples:**
- `posts/list-posts.json` — 25 posts for pagination (20 per page)
- `posts/filter-complex.json` — Complex AND/OR filter conditions
- `storage/upload-too-large.json` — File exceeding size limit

### Contract Test Fixtures
Test external API contracts (OAuth providers, etc.).

**Examples:**
- `oauth/google-auth-code.json` — Google OAuth authorization code
- `oauth/github-access-token.json` — GitHub access token

---

## Adding New Fixtures

1. **Create fixture file** in appropriate subdirectory
2. **Include comprehensive metadata** with all expected values
3. **Add test tags** for categorization
4. **Document in test spec** — Reference fixture in `tests/specs/{feature}.md`
5. **Use in tests** — Load fixture and assert using metadata

---

**Last Updated:** 2026-02-13 (Session 078)
