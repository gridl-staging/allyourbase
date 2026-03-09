# Test Fixtures

**Purpose:** Test fixtures provide sample data with expected values for automated testing.

## Fixture Structure

All fixtures follow this metadata-driven pattern:

```json
{
  "metadata": {
    "description": "Brief description of what this fixture tests",
    "test_case": "TC-XXXX-NNN",
    "expected_response_status": 200,
    "expected_response_fields": ["field1", "field2"],
    "test_tags": ["category", "subcategory", "type"]
  },
  "field1": "value1",
  "field2": "value2"
}
```

## Metadata Fields

- `description` — Human-readable description of the test scenario
- `test_case` — Reference to test spec (e.g., TC-AUTH-001)
- `expected_response_status` — HTTP status code expected from API
- `expected_response_fields` — Array of fields that must be in response
- `expected_*` — Additional expected values (status, counts, errors, etc.)
- `test_tags` — Tags for categorizing tests (feature, happy-path, error-handling)

## Fixture Organization

```
tests/fixtures/
├── admin/           — Admin dashboard fixtures
├── users/           — Auth and user management fixtures
├── posts/           — Collections CRUD fixtures
├── oauth/           — OAuth provider fixtures
├── storage/         — File storage fixtures
├── webhooks/        — Webhook event fixtures
├── realtime/        — SSE realtime fixtures
├── schema/          — Schema introspection fixtures
├── rpc/             — PostgreSQL function call fixtures
└── migration/       — Migration tools fixtures (subdirs: pocketbase, supabase, firebase)
```

## Usage in Tests

```typescript
import fixture from './fixtures/users/valid-registration.json';

test('user registration', async () => {
  const response = await fetch('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({
      email: fixture.email,
      password: fixture.password
    })
  });

  // Use metadata for assertions
  expect(response.status).toBe(fixture.metadata.expected_response_status);
  
  const data = await response.json();
  fixture.metadata.expected_response_fields.forEach(field => {
    expect(data).toHaveProperty(field);
  });
});
```

## Fixture Guidelines

1. **No Made-Up Values** — All expected values come from fixture metadata
2. **Deterministic** — Fixtures should produce consistent results
3. **Self-Contained** — Include setup data when needed (e.g., `setup_records`)
4. **Tagged** — Use tags for filtering tests by category
5. **Documented** — Metadata makes fixtures self-documenting

## Example: Complete Fixture

```json
{
  "metadata": {
    "description": "Valid user login with correct credentials",
    "test_case": "TC-AUTH-002",
    "expected_response_status": 200,
    "expected_response_fields": ["token", "refreshToken", "user"],
    "expected_user_fields": ["id", "email", "created_at"],
    "test_tags": ["auth", "login", "happy-path"]
  },
  "email": "test@example.com",
  "password": "SecurePassword123!",
  "setup": {
    "pre_existing_user": {
      "email": "test@example.com",
      "password_hash": "$2a$10$abc123...",
      "email_verified": true
    }
  }
}
```

---

**Created:** Session 079 (2026-02-13)
**Last Updated:** Session 079
