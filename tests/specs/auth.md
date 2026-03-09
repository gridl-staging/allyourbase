# Auth Test Specification

**User Stories:** B-AUTH-001 through B-AUTH-007
**Read this file BEFORE implementing auth tests**

---

## Overview

Auth flow has four test levels:
1. **Unit tests** (Go) — Test password hashing, JWT generation logic
2. **Integration tests** (Go) — Test auth handlers with real Postgres
3. **Unit tests** (SDK) — Test SDK auth client methods
4. **Browser tests (unmocked) tests** (Playwright) — Test full auth flow through admin UI

**This spec focuses on integration and Browser tests (unmocked) tests.** Unit tests already exist.

---

## <a id="registration"></a>TEST: User Registration (Integration)

**BDD Story:** B-AUTH-001
**Type:** Integration test
**File:** `internal/auth/handler_integration_test.go`
**Purpose:** Test registration endpoint with real Postgres

### Prerequisites
- Test database (via `testutil.SetupTestDB(t)`)
- No existing user with test email

### Test Cases

#### 1. Successful Registration with Valid Credentials

**Fixture:** `tests/fixtures/users/valid-user.json`
```json
{
  "metadata": {
    "description": "Valid user with email and password",
    "expected_response_status": 201,
    "expected_user_fields": ["id", "email", "created_at", "updated_at"],
    "expected_token_type": "Bearer",
    "expected_token_fields": ["token", "refreshToken", "user"]
  },
  "email": "test@example.com",
  "password": "SecurePass123!"
}
```

**Execute:**
```go
fixture := loadFixture("users/valid-user.json")
resp := makeRequest("POST", "/api/auth/register", fixture)
```

**Verify:**
```go
testutil.Equal(t, fixture.Metadata.ExpectedResponseStatus, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

// Verify token structure
testutil.True(t, body["token"] != nil)
testutil.True(t, body["refreshToken"] != nil)
testutil.True(t, body["user"] != nil)

// Verify user fields
user := body["user"].(map[string]interface{})
testutil.True(t, user["id"] != nil)
testutil.Equal(t, fixture.Email, user["email"])
testutil.True(t, user["created_at"] != nil)

// Verify password hash in DB (argon2id format)
var hash string
db.QueryRow("SELECT password_hash FROM ayb_users WHERE email = $1", fixture.Email).Scan(&hash)
testutil.True(t, strings.HasPrefix(hash, "$argon2id$"))
```

**Cleanup:**
```go
t.Cleanup(func() {
    db.Exec("DELETE FROM ayb_users WHERE email = $1", fixture.Email)
})
```

---

#### 2. Registration with Duplicate Email

**Fixture:** `tests/fixtures/users/duplicate-email.json`
```json
{
  "metadata": {
    "description": "User with email that already exists",
    "expected_response_status": 409,
    "expected_error_message": "email already exists"
  },
  "email": "existing@example.com",
  "password": "SecurePass123!"
}
```

**Execute:**
```go
// First, create user
db.Exec("INSERT INTO ayb_users (email, password_hash) VALUES ($1, $2)",
    fixture.Email, hashPassword("existing"))

// Try to register with same email
resp := makeRequest("POST", "/api/auth/register", fixture)
```

**Verify:**
```go
testutil.Equal(t, fixture.Metadata.ExpectedResponseStatus, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)
testutil.Contains(t, body["error"], fixture.Metadata.ExpectedErrorMessage)
```

**Cleanup:** Delete test user

---

#### 3. Registration with Invalid Email

**Fixture:** `tests/fixtures/users/invalid-email.json`
```json
{
  "metadata": {
    "description": "User with invalid email format",
    "expected_response_status": 400,
    "expected_error_message": "invalid email"
  },
  "email": "not-an-email",
  "password": "SecurePass123!"
}
```

**Execute & Verify:** Same pattern as duplicate email test

---

#### 4. Registration with Weak Password

**Fixture:** `tests/fixtures/users/weak-password.json`
```json
{
  "metadata": {
    "description": "User with password shorter than configured minimum (default 8)",
    "expected_response_status": 422,
    "expected_error_message": "password must be at least 8 characters"
  },
  "email": "test@example.com",
  "password": "short"
}
```

**Execute & Verify:** Same pattern as above

---

## <a id="login"></a>TEST: User Login (Integration)

**BDD Story:** B-AUTH-002
**Type:** Integration test
**File:** `internal/auth/handler_integration_test.go`

### Test Cases

#### 1. Successful Login with Valid Credentials

**Fixture:** `tests/fixtures/users/login-valid.json`
```json
{
  "metadata": {
    "description": "Valid login credentials",
    "expected_response_status": 200,
    "expected_token_type": "Bearer"
  },
  "email": "login-test@example.com",
  "password": "SecurePass123!"
}
```

**Execute:**
```go
// Setup: Create user
db.Exec("INSERT INTO ayb_users (email, password_hash) VALUES ($1, $2)",
    fixture.Email, hashPassword(fixture.Password))

// Login
resp := makeRequest("POST", "/api/auth/login", fixture)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.True(t, body["token"] != nil)
testutil.True(t, body["refreshToken"] != nil)
testutil.Equal(t, fixture.Email, body["user"].(map[string]interface{})["email"])
```

**Cleanup:** Delete test user

---

#### 2. Login with Invalid Credentials

**Fixture:** `tests/fixtures/users/login-invalid.json`
```json
{
  "metadata": {
    "description": "Invalid password",
    "expected_response_status": 401,
    "expected_error_message": "invalid credentials"
  },
  "email": "login-test@example.com",
  "password": "WrongPassword123!"
}
```

**Execute & Verify:** Same pattern as above

---

#### 3. Progressive Password Re-hashing (bcrypt → argon2id)

**Fixture:** `tests/fixtures/users/login-bcrypt.json`
```json
{
  "metadata": {
    "description": "User with bcrypt password hash that should be re-hashed to argon2id",
    "expected_response_status": 200,
    "expected_old_hash_prefix": "$2a$",
    "expected_new_hash_prefix": "$argon2id$"
  },
  "email": "bcrypt-user@example.com",
  "password": "SecurePass123!"
}
```

**Execute:**
```go
// Setup: Create user with bcrypt hash
bcryptHash := bcryptHashPassword(fixture.Password) // $2a$...
db.Exec("INSERT INTO ayb_users (email, password_hash) VALUES ($1, $2)",
    fixture.Email, bcryptHash)

// Verify bcrypt hash before login
var hashBefore string
db.QueryRow("SELECT password_hash FROM ayb_users WHERE email = $1", fixture.Email).Scan(&hashBefore)
testutil.True(t, strings.HasPrefix(hashBefore, fixture.Metadata.ExpectedOldHashPrefix))

// Login
resp := makeRequest("POST", "/api/auth/login", fixture)

// Verify login success
testutil.Equal(t, 200, resp.StatusCode)

// Verify hash was upgraded to argon2id
var hashAfter string
db.QueryRow("SELECT password_hash FROM ayb_users WHERE email = $1", fixture.Email).Scan(&hashAfter)
testutil.True(t, strings.HasPrefix(hashAfter, fixture.Metadata.ExpectedNewHashPrefix))
```

**Cleanup:** Delete test user

---

## <a id="token-refresh"></a>TEST: Token Refresh (Integration)

**BDD Story:** B-AUTH-003
**Type:** Integration test
**File:** `internal/auth/handler_integration_test.go`

### Test Cases

#### 1. Successful Token Refresh

**Fixture:** `tests/fixtures/users/refresh-valid.json`
```json
{
  "metadata": {
    "description": "Valid refresh token",
    "expected_response_status": 200,
    "expected_new_tokens": ["token", "refreshToken"]
  },
  "email": "refresh-test@example.com",
  "password": "SecurePass123!"
}
```

**Execute:**
```go
// Setup: Create user and get initial tokens
userID := createTestUser(t, db, fixture.Email, fixture.Password)
initialTokens := login(t, fixture.Email, fixture.Password)

// Refresh token
resp := makeRequest("POST", "/api/auth/refresh", map[string]string{
    "refreshToken": initialTokens["refreshToken"],
})
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.True(t, body["token"] != nil)
testutil.True(t, body["refreshToken"] != nil)

// Verify tokens are different from initial tokens
testutil.True(t, body["token"] != initialTokens["token"])
testutil.True(t, body["refreshToken"] != initialTokens["refreshToken"])

// Verify old refresh token is invalidated
oldTokenResp := makeRequest("POST", "/api/auth/refresh", map[string]string{
    "refreshToken": initialTokens["refreshToken"],
})
testutil.Equal(t, 401, oldTokenResp.StatusCode)
```

**Cleanup:** Delete test user

---

## <a id="logout"></a>TEST: User Logout (Integration)

**BDD Story:** B-AUTH-004
**Type:** Integration test

### Test Cases

#### 1. Successful Logout

**Execute:**
```go
// Setup: Create user and login
userID := createTestUser(t, db, "logout-test@example.com", "SecurePass123!")
tokens := login(t, "logout-test@example.com", "SecurePass123!")

// Logout
resp := makeRequest("POST", "/api/auth/logout", map[string]string{
    "refreshToken": tokens["refreshToken"],
})
```

**Verify:**
```go
testutil.Equal(t, 204, resp.StatusCode)

// Verify refresh token is invalidated
refreshResp := makeRequest("POST", "/api/auth/refresh", map[string]string{
    "refreshToken": tokens["refreshToken"],
})
testutil.Equal(t, 401, refreshResp.StatusCode)
```

**Cleanup:** Delete test user

---

## <a id="get-current-user"></a>TEST: Get Current User (Integration)

**BDD Story:** B-AUTH-005
**Type:** Integration test

### Test Cases

#### 1. Get User with Valid Token

**Execute:**
```go
// Setup: Create user and login
userID := createTestUser(t, db, "me-test@example.com", "SecurePass123!")
tokens := login(t, "me-test@example.com", "SecurePass123!")

// Get current user
resp := makeRequest("GET", "/api/auth/me", nil, map[string]string{
    "Authorization": "Bearer " + tokens["token"],
})
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var body map[string]interface{}
json.NewDecoder(resp.Body).Decode(&body)

testutil.Equal(t, "me-test@example.com", body["email"])
testutil.True(t, body["id"] != nil)
testutil.True(t, body["created_at"] != nil)

// Verify password_hash is NOT returned
testutil.Nil(t, body["password_hash"])
```

**Cleanup:** Delete test user

---

## Browser tests (unmocked) Tests

**Location:** `ui/browser-tests-unmocked/smoke/auth-registration.spec.ts`
**Purpose:** Test full auth flow through admin UI

### Test Cases

#### 1. Admin Login

**Execute:**
1. Navigate to `/admin/`
2. Enter admin password
3. Click login button

**Verify:**
- Admin dashboard loads
- SQL editor accessible
- Sidebar visible

#### 2. Register User via UI (if user management UI exists)

**Execute:**
1. Navigate to Users section
2. Click "New User" button
3. Fill in email and password
4. Click "Create"

**Verify:**
- User appears in users list
- Password is hashed in DB

---

## Fixture Data Needed

**Create these fixtures:**

1. `tests/fixtures/users/valid-user.json` — Valid registration
2. `tests/fixtures/users/duplicate-email.json` — Duplicate email error
3. `tests/fixtures/users/invalid-email.json` — Invalid email format
4. `tests/fixtures/users/weak-password.json` — Weak password error
5. `tests/fixtures/users/login-valid.json` — Valid login
6. `tests/fixtures/users/login-invalid.json` — Invalid login
7. `tests/fixtures/users/login-bcrypt.json` — Progressive re-hashing test
8. `tests/fixtures/users/refresh-valid.json` — Token refresh

---

**Spec Version:** 1.0
**Last Updated:** 2026-02-13 (Session 078)
