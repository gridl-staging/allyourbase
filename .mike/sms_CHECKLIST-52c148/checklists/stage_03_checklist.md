# Stage 3: Auth Service & Handlers

## Step 1: Unit Tests (RED)

- [x] Create `internal/auth/sms_auth_test.go` with `package auth` (matches `magic_link_test.go`)
- [x] Adapt `_dev/messaging/impl/06_auth_unit_tests.go` to match existing patterns:
  - Use `testutil.*` helpers (not testify `assert`/`require`) — matches all existing auth tests
  - Use `newTestService()` from `middleware_test.go` + `NewHandler(svc, testutil.DiscardLogger())`
  - Use `httptest.NewRequest` / `httptest.NewRecorder` directly (no `newTestRequest` wrapper)
  - Route through `h.Routes()` + `router.ServeHTTP(w, req)` — matches `magic_link_test.go` pattern
- [x] Add `newSMSHandler(enabled bool) *Handler` test helper (mirrors `newMagicLinkHandler`)
- [x] Tests to add:
  - **OTP generation:**
    - `TestGenerateOTP`: lengths 4-8 produce correct-length digit-only strings
    - `TestGenerateOTPIsRandom`: 100 generations should produce >50 unique values
  - **Phone normalization:**
    - `TestNormalizePhone`: valid formats (`"+1 415 555 2671"` → `"+14155552671"`, with dashes, spaces, international)
    - `TestNormalizePhoneRejectsInvalid`: `"4155552671"` (no +), `"+1"` (too short), `"+1234567890123456"` (too long), `"+abc"`, `""`, `"not-a-phone"`, non-ASCII digits
  - **Country allow-list:** (added during review)
    - `TestIsAllowedCountry`: empty list allows all, explicit list filters correctly, unknown code blocks
  - **SMS request handler:**
    - `TestHandleSMSRequest_DisabledReturns404`: smsEnabled=false → 404 + "not enabled"
    - `TestHandleSMSRequest_MissingPhone_Returns400`: `{}` → 400
    - `TestHandleSMSRequest_InvalidPhoneFormat_Returns400`: `"not-a-phone"` → 400
    - `TestHandleSMSRequest_ValidPhoneAlwaysReturns200`: `"+14155552671"` → 200 (anti-enumeration)
    - `TestHandleSMSRequest_MalformedJSON_Returns400`: bad JSON → 400
  - **SMS confirm handler:**
    - `TestHandleSMSConfirm_Disabled_Returns404`: smsEnabled=false → 404
    - `TestHandleSMSConfirm_MissingFields_Returns400`: missing code → 400, missing phone → 400
    - `TestHandleSMSConfirm_MalformedJSON_Returns400`: bad JSON → 400
  - **Route registration:**
    - `TestSMSRoutesRegistered`: POST `/sms` and `/sms/confirm` respond (not 404/405)
  - **SetSMSEnabled:**
    - `TestSetSMSEnabled`: default=false, set true, set false
- [x] Run `go test ./internal/auth/...` — confirm FAIL (SMS types/methods don't exist yet)

## Step 2: Service Layer Implementation (GREEN)

- [x] Add `Phone string` field to `User` struct in `auth.go` with `json:"phone,omitempty"`
- [x] Add SMS fields to `Service` struct in `auth.go`:
  - `smsProvider sms.Provider` (nil = feature disabled, mirrors `mailer` pattern)
  - `smsConfig  sms.Config`
- [x] Add service methods to `auth.go`:
  - `SetSMSProvider(p sms.Provider)` — sets `s.smsProvider`
  - `SetSMSConfig(c sms.Config)` — sets `s.smsConfig`
  - `DB() *pgxpool.Pool` — returns `s.pool` (needed by integration tests in stage 4)
- [x] Add sentinel errors in `auth.go`:
  - `ErrDailyLimitExceeded = errors.New("daily SMS limit exceeded")`
  - `ErrInvalidSMSCode = errors.New("invalid or expired SMS code")`
  - `ErrInvalidPhoneNumber = errors.New("invalid phone number")`
- [x] Create `internal/auth/sms_auth.go` with service-level functions:
  - `generateOTP(length int) (string, error)` — use `crypto/rand`, produce N-digit string (digits 0-9 only)
  - `normalizePhone(input string) (string, error)` — strip spaces/dashes/parens, validate E.164 format (`+` followed by 7-15 digits), return error for invalid
  - `isAllowedCountry(phone string, allowed []string) bool` — extract country code prefix, check against allowed list
  - `RequestSMSCode(ctx context.Context, phone string) error`:
    1. Check `s.smsProvider == nil` → return nil (disabled)
    2. Normalize phone → on invalid format, return `ErrInvalidPhoneNumber` (handler turns this into 400)
    3. Geo check `isAllowedCountry` → if blocked, return nil (anti-enumeration: no error, no SMS sent)
    4. Check daily limit: query `_ayb_sms_daily_counts` for today's count → if `>= DailyLimit` and `DailyLimit > 0`, return `ErrDailyLimitExceeded`
    5. Delete existing codes for this phone: `DELETE FROM _ayb_sms_codes WHERE phone = $1`
    6. Generate OTP with `generateOTP(s.smsConfig.CodeLength)`
    7. Hash OTP with `bcrypt.GenerateFromPassword` (NOT SHA-256 — matches migration comment)
    8. Insert into `_ayb_sms_codes` (phone, code_hash, expires_at)
    9. Increment daily count: `INSERT INTO _ayb_sms_daily_counts ... ON CONFLICT (date) DO UPDATE SET count = count + 1`
    10. Send via provider: `s.smsProvider.Send(ctx, phone, "Your code is: "+otp)`
    11. Return nil (always, for anti-enumeration — log errors internally)
  - `ConfirmSMSCode(ctx context.Context, phone, code string) (*User, string, string, error)`:
    1. Normalize phone
    2. Look up code row: `SELECT id, code_hash, expires_at, attempts FROM _ayb_sms_codes WHERE phone = $1`
    3. If no row → return `ErrInvalidSMSCode`
    4. If expired → delete row, return `ErrInvalidSMSCode`
    5. If `attempts >= MaxAttempts` → delete row, return `ErrInvalidSMSCode`
    6. `bcrypt.CompareHashAndPassword(code_hash, code)` → if mismatch: increment attempts, return `ErrInvalidSMSCode`
    7. On match → delete code row (consumed)
    8. Find or create user by phone: `SELECT id, email, phone, created_at, updated_at FROM _ayb_users WHERE phone = $1` → if not found, `INSERT INTO _ayb_users (phone, password_hash) VALUES ($1, $2) RETURNING ...` (random password, same pattern as magic link)
    9. Handle unique constraint race (23505) — same pattern as `ConfirmMagicLink`
    10. Return `s.issueTokens(ctx, &user)`

## Step 3: Handler Implementation (GREEN)

- [x] Add `smsEnabled bool` field to `Handler` struct in `handler.go`
- [x] Add `SetSMSEnabled(enabled bool)` method in `handler.go`
- [x] Add SMS request/response types in `sms_auth.go` (or `handler.go`):
  - `smsRequest struct { Phone string \`json:"phone"\` }`
  - `smsConfirmRequest struct { Phone string \`json:"phone"\`; Code string \`json:"code"\` }`
- [x] Add `handleSMSRequest(w, r)` handler in `sms_auth.go`:
  1. Check `h.smsEnabled` → 404 with "SMS authentication is not enabled" (mirrors magic link pattern)
  2. Decode JSON body
  3. Validate phone not empty → 400
  4. Normalize phone → if `ErrInvalidPhoneNumber`, return 400
  5. Call `h.auth.RequestSMSCode(r.Context(), req.Phone)` — log errors but always return 200
  6. Return 200 with `{"message": "if valid, a verification code has been sent"}`
- [x] Add `handleSMSConfirm(w, r)` handler in `sms_auth.go`:
  1. Check `h.smsEnabled` → 404
  2. Decode JSON body
  3. Validate phone and code not empty → 400
  4. Call `h.auth.ConfirmSMSCode(r.Context(), req.Phone, req.Code)`
  5. On `ErrInvalidSMSCode` → 401 with error message
  6. On `ErrDailyLimitExceeded` → 429 with error message
  7. On success → 200 with `authResponse{Token, RefreshToken, User}`

## Step 4: Route Wiring

- [x] Add SMS routes in `handler.go` `Routes()` method:
  - `r.Post("/sms", h.handleSMSRequest)`
  - `r.Post("/sms/confirm", h.handleSMSConfirm)`
- [x] Run `go test ./internal/auth/...` — confirm all unit tests PASS
- [x] Run `go build ./...` — confirm PASS
- [x] Commit: `feat: add SMS OTP auth service and handlers`


## Master Stages
1. [x] SMS Provider Layer - Provider interface, LogProvider, TwilioProvider, CaptureProvider (Phase 1 Steps 1-3)
2. [x] Database & Config - Migration 013, SMS config fields with validation, startup wiring (Phase 1 Steps 4-5)
3. Auth Service & Handlers - OTP generation, phone normalization, geo check, request/confirm flows, route wiring (Phase 1 Step 6)
4. Server Wiring & Integration Tests - Server/CLI wiring, integration tests, smoke test (Phase 1 Steps 7-9)
5. SMS MFA Second Factor - Migration 014, enroll/challenge endpoints, MFA pending token gating (Phase 2)
6. Provider Expansion - Plivo, Telnyx, Prelude, MSG91, AWS SNS, Vonage, webhook provider, test phone numbers (Phase 3 first half)
7. Fraud Hardening & Monitoring - libphonenumber upgrade, conversion rate monitoring, admin health endpoint (Phase 3 second half)
8. Transactional SMS API - Migration 015, messaging endpoint, delivery status webhook (Phase 4)
