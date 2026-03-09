# Stage 8: Transactional SMS API

Phase 4 of the SMS implementation. Adds a programmatic SMS sending endpoint (`POST /api/messaging/sms/send`)
with API-key auth, a messages table for persistence and audit, and delivery status webhooks to track
provider-reported outcomes. Uses `SendResult.MessageID` from the Phase 1 provider interface.

**Key context:**
- Migration numbering: 015 is taken (SMS stats columns from stage 7). Next is **016**.
- Phone utilities (`normalizePhone`, `phoneCountry`, `isAllowedCountry`) live in `internal/auth/sms_auth.go` as
  unexported functions. The messaging handler needs them too — extract to `internal/sms/` for DRY reuse.
- SMS provider is currently set only on `auth.Service`. Server needs direct access for the messaging handler.
- API key auth: `auth.RequireAuth(svc)` already validates JWT or API key and sets claims in context.
  Messaging endpoint should reject `readonly` scope API keys (sending SMS is a write operation).
  Use `auth.CheckWriteScope(claims)` — returns `auth.ErrScopeReadOnly` for readonly keys.
- `auth.Claims` struct: `Subject` = user ID, `APIKeyScope` = scope string, `Email` = user email.
  Get claims via `auth.ClaimsFromContext(r.Context())`.
- `auth.Claims` does NOT currently have an `APIKeyID` field. The `api_key_id` column in the messages
  table will be NULL until `APIKeyID` is added to Claims (small follow-up). Keep the column for future use.

---

## Step 1: Extract Phone Utilities to `internal/sms/phone.go` (Refactor)

This is a pure move-and-export refactor. The three unexported phone functions in `internal/auth/sms_auth.go`
need to be usable by the messaging handler in `internal/server/`. Moving them to `internal/sms/` is safe
because `auth` already imports `sms` (for `sms.Config`, `sms.Provider`), so no circular dependency.

- [x] Create `internal/sms/phone.go` with exported functions:
  - `NormalizePhone(input string) (string, error)` — E.164 normalization via phonenumbers
  - `PhoneCountry(phone string) string` — ISO 3166-1 alpha-2 country code
  - `IsAllowedCountry(phone string, allowed []string) bool` — country allowlist check
  - `var ErrInvalidPhoneNumber = errors.New("invalid phone number")`
  - Copy the exact implementation from `internal/auth/sms_auth.go` (lines 33-87)
- [x] Create `internal/sms/phone_test.go` — port existing tests from `internal/auth/sms_auth_test.go`:
  - `TestNormalizePhone` (basic valid/invalid cases)
  - `TestNormalizePhone_LibPhoneNumber` (various format inputs → E.164)
  - `TestNormalizePhone_RejectsInvalidForCountry` (digit-count-valid but country-invalid)
  - `TestNormalizePhone_AcceptsGlobalNumbers` (India, Japan, Brazil, Nigeria, etc.)
  - `TestPhoneCountry` (US, GB, IN extraction)
  - `TestPhoneCountry_DistinguishesNANP` (US vs CA on shared +1)
  - `TestPhoneCountry_Caribbean` (Jamaica ≠ US/CA)
  - `TestIsAllowedCountry` (empty list allows all, populated list filters)
- [x] `go test ./internal/sms/... -run "TestNormalizePhone|TestPhoneCountry|TestIsAllowedCountry"` — PASS
- [x] Update `internal/auth/sms_auth.go`:
  - Delete local `normalizePhone()`, `phoneCountry()`, `isAllowedCountry()` function bodies
  - Replace all call sites with `sms.NormalizePhone()`, `sms.PhoneCountry()`, `sms.IsAllowedCountry()`
  - Replace local `ErrInvalidPhoneNumber` definition with `var ErrInvalidPhoneNumber = sms.ErrInvalidPhoneNumber`
- [x] Update `internal/auth/sms_mfa.go`: uses `normalizePhone()` in `EnrollSMSMFA` and `ConfirmSMSMFAEnrollment` — update to `sms.NormalizePhone()`
- [x] `go test ./internal/auth/...` — all existing tests still PASS (no regressions)
- [x] Commit: `refactor: extract phone utilities to sms package for reuse`

## Step 2: Migration 016 — SMS Messages Table

- [x] Create `internal/migrations/sql/016_ayb_sms_messages.sql`:
  ```sql
  CREATE TABLE IF NOT EXISTS _ayb_sms_messages (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      user_id UUID NOT NULL REFERENCES _ayb_users(id),
      api_key_id UUID REFERENCES _ayb_api_keys(id),
      to_phone TEXT NOT NULL,
      body TEXT NOT NULL,
      provider TEXT NOT NULL DEFAULT '',
      provider_message_id TEXT DEFAULT '',
      status TEXT NOT NULL DEFAULT 'pending',
      error_message TEXT DEFAULT '',
      created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
  );

  CREATE INDEX IF NOT EXISTS idx_sms_messages_user_created
      ON _ayb_sms_messages(user_id, created_at DESC);
  CREATE INDEX IF NOT EXISTS idx_sms_messages_status
      ON _ayb_sms_messages(status);
  CREATE INDEX IF NOT EXISTS idx_sms_messages_provider_msg_id
      ON _ayb_sms_messages(provider_message_id)
      WHERE provider_message_id != '';
  ```
  Key differences from previous version:
  - `user_id` is `NOT NULL` — every message must have an owner
  - `api_key_id` is nullable — NULL when sent via JWT auth (populated later when `APIKeyID` added to Claims)
  - Compound index `(user_id, created_at DESC)` replaces separate `user_id` and `created_at` indexes — serves both the list query filter and sort in a single index scan
  - Removed standalone `created_at DESC` index (redundant with compound index)
- [x] Migration runner uses `//go:embed sql/*.sql` with filename sort — 016 auto-discovered. No code changes needed.
- [x] Commit: `feat: add migration 016 for SMS messages table`

## Step 3: SMS Provider Accessor on Server

- [x] Add fields to `Server` struct in `internal/server/server.go`:
  ```go
  smsProvider         sms.Provider // nil when SMS disabled
  smsProviderName     string       // "twilio", "plivo", etc. — stored in messages for audit
  smsAllowedCountries []string     // country allowlist from config
  ```
- [x] Add `SetSMSProvider(name string, p sms.Provider, allowedCountries []string)` method on `*Server`
- [x] In `internal/cli/start.go`: after `srv := server.New(...)`, call
  `srv.SetSMSProvider(cfg.Auth.SMSProvider, smsProvider, cfg.Auth.SMSAllowedCountries)` when `smsProvider != nil`.
  **Bug fix (session 036):** previous session renamed `p` to `smsProvider` without declaring the variable,
  breaking the build. Also missing the `srv.SetSMSProvider(...)` call entirely. Both fixed.
- [x] `go build ./...` — PASS
- [x] Commit: `feat: expose SMS provider on server for messaging API`

## Step 4: Send SMS Endpoint (TDD)

- [x] RED: Create `internal/server/messaging_test.go` with tests:
  - `TestMessagingSMSSend_Success`: valid auth + valid phone + body → 200, JSON response has `id`, `message_id`, `status`, `to`; provider `Send()` called with correct args
  - `TestMessagingSMSSend_PersistsMessage`: after successful send, row exists in `_ayb_sms_messages` with correct user_id, to_phone, body, provider_message_id, status
  - `TestMessagingSMSSend_InvalidPhone`: malformed phone → 400 `"invalid phone number"`
  - `TestMessagingSMSSend_MissingBody`: empty body → 400 `"body is required"`
  - `TestMessagingSMSSend_BodyTooLong`: body > 1600 chars → 400 `"body exceeds maximum length"`
  - `TestMessagingSMSSend_MissingTo`: empty to → 400 `"to is required"`
  - `TestMessagingSMSSend_RequiresAuth`: no auth header → 401
  - `TestMessagingSMSSend_RejectsReadonlyKey`: API key with `readonly` scope → 403
  - `TestMessagingSMSSend_SMSDisabled`: no provider set → 404 (same pattern as auth SMS handlers)
  - `TestMessagingSMSSend_CountryBlocked`: phone from non-allowed country → 400 `"phone number country not allowed"`
  - `TestMessagingSMSSend_ProviderError`: provider returns error → 500, message stored with status `"failed"` and error_message
  - `TestMessagingSMSSend_DBErrorOnInsert`: DB fails on insert → 500, provider never called (no SMS sent without audit record)
- [x] GREEN: Create `internal/server/messaging_handler.go`:
  - `handleMessagingSMSSend(w, r)` on `*Server`
  - Guard: `s.smsProvider == nil` → 404 with doc URL
  - Parse JSON body: `{"to": "+1...", "body": "Hello world"}`
  - Validate: `to` required, `body` required, `len(body) <= 1600` (Twilio API max; provider handles segmentation)
  - Extract claims: `auth.ClaimsFromContext(r.Context())`
  - Scope check: `auth.CheckWriteScope(claims)` → 403 if readonly
  - Normalize: `sms.NormalizePhone(to)` → 400 on error
  - Country check: `sms.IsAllowedCountry(phone, s.smsAllowedCountries)` → 400 if blocked
  - Insert row into `_ayb_sms_messages` with `user_id = claims.Subject`, `status = "pending"`, `provider = s.smsProviderName`
  - Call `s.smsProvider.Send(ctx, phone, body)`
  - On success: update row with `provider_message_id = result.MessageID`, `status = result.Status` (or "queued" if empty)
  - On error: update row with `status = "failed"`, `error_message = err.Error()`; return 500
  - Return JSON: `{"id": "<uuid>", "message_id": "<provider-sid>", "status": "queued", "to": "+1..."}`
- [x] `go test ./internal/server/... -run TestMessagingSMSSend` — PASS
- [x] Commit: `feat: add transactional SMS API with message history and delivery webhooks`

## Step 5: Message History Endpoints (TDD)

- [x] RED: Add tests in `internal/server/messaging_test.go`:
  - `TestMessagingSMSList_ReturnsPaginated`: multiple messages → JSON array with limit/offset, default limit 50
  - `TestMessagingSMSList_RequiresAuth`: no auth → 401
  - `TestMessagingSMSList_ScopedToUser`: user A cannot see user B's messages (filter by `claims.Subject`)
  - `TestMessagingSMSList_OrderByCreatedDesc`: newest first
  - `TestMessagingSMSGet_Success`: GET by ID → full message JSON
  - `TestMessagingSMSGet_NotFound`: unknown UUID → 404
  - `TestMessagingSMSGet_WrongUser`: user A requests user B's message ID → 404 (not 403, prevents enumeration)
  - `TestMessagingSMSList_EmptyReturnsArray`: no messages → 200 with [] (not null — API contract)
  - `TestMessagingSMSList_LimitClamp`: ?limit=200 clamped to 100
- [x] GREEN: Add handlers in `internal/server/messaging_handler.go`:
  - `handleMessagingSMSList(w, r)` on `*Server`
    - Parse `limit` (default 50, max 100) and `offset` (default 0) from query params
    - Query uses compound index: `SELECT id, to_phone, body, provider, provider_message_id, status, error_message, created_at, updated_at FROM _ayb_sms_messages WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
    - Return JSON array of message objects
  - `handleMessagingSMSGet(w, r)` on `*Server`
    - Parse `{id}` from URL path via `chi.URLParam(r, "id")`
    - Query: `SELECT ... FROM _ayb_sms_messages WHERE id = $1 AND user_id = $2`
    - Return JSON message object or 404
- [x] `go test ./internal/server/... -run "TestMessagingSMSList|TestMessagingSMSGet"` — PASS
- [x] Commit: (combined with step 4 commit)

## Step 6: Delivery Status Webhook (TDD)

Twilio POSTs `application/x-www-form-urlencoded` status callbacks. Key fields:
- `MessageSid` — provider message ID (matches `provider_message_id`)
- `MessageStatus` — one of: `accepted`, `queued`, `sending`, `sent`, `delivered`, `undelivered`, `failed`, `read`, `canceled`
- `ErrorCode` — numeric error code (present on failure)
- `ErrorMessage` — human-readable error (present on failure)

Note: status callbacks may arrive out of order. Only update if the new status is "later" in the lifecycle.

- [x] RED: Add tests in `internal/server/messaging_test.go`:
  - `TestSMSDeliveryWebhook_TwilioUpdatesStatus`: POST with form-encoded Twilio callback (MessageSid, MessageStatus=delivered) → 200, message row updated
  - `TestSMSDeliveryWebhook_StatusProgression`: queued → sent → delivered updates in sequence
  - `TestSMSDeliveryWebhook_FailedStatus`: MessageStatus=failed + ErrorCode → message updated with error_message containing code
  - `TestSMSDeliveryWebhook_UnknownMessageSid`: unknown provider_message_id → 200 (idempotent, no error — Twilio retries on non-2xx)
  - `TestSMSDeliveryWebhook_MissingFields`: no MessageSid → 400
  - `TestSMSDeliveryWebhook_OutOfOrderStatus`: delivered then sent → status stays delivered (not regressed)
  - `TestSMSDeliveryWebhook_DBError`: UpdateDeliveryStatus DB failure → still 200 (Twilio must not retry)
- [x] GREEN: Add to `internal/server/messaging_handler.go`:
  - `handleSMSDeliveryWebhook(w, r)` on `*Server`
  - Parse form values: `r.FormValue("MessageSid")`, `r.FormValue("MessageStatus")`, `r.FormValue("ErrorCode")`
  - Validate: `MessageSid` required → 400 if empty
  - Map Twilio status to internal status (passthrough — store raw Twilio status string)
  - Build error_message: if ErrorCode present, format as `"error <code>: <message>"`
  - `deliveryStatusRank(status)` helper: pending(0) < accepted(1) < queued(2) < sending(3) < sent(4) < terminal(5)
  - `UPDATE _ayb_sms_messages SET status=$1, error_message=$2, updated_at=now() WHERE provider_message_id=$3 AND rank(status) <= rank($1)` — prevents out-of-order regressions
  - Return 200 with empty JSON body (Twilio expects 200)
  - Add TODO comment: `// TODO: Twilio request signature verification — requires webhook URL in config + auth token on server`
- [x] `go test ./internal/server/... -run TestSMSDelivery` — PASS
- [x] Commit: (combined with step 4 commit)

## Step 7: Wire Messaging Routes into Server

- [x] Update `internal/server/server.go` route registration:
  - Inside the `/api` route group, inside the JSON content-type group, when `authSvc != nil`:
    ```go
    r.Route("/messaging/sms", func(r chi.Router) {
        r.Use(auth.RequireAuth(authSvc))
        r.Post("/send", s.handleMessagingSMSSend)
        r.Get("/messages", s.handleMessagingSMSList)
        r.Get("/messages/{id}", s.handleMessagingSMSGet)
    })
    ```
  - Inside the `/api` route group but OUTSIDE the JSON content-type group (Twilio sends form-encoded):
    ```go
    r.Post("/webhooks/sms/status", s.handleSMSDeliveryWebhook)
    ```
    This results in the full path `/api/webhooks/sms/status` since it's within `r.Route("/api", ...)`.
- [x] `go test ./internal/server/...` — PASS
- [x] Commit: (combined with step 4 commit)

## Step 8: Build & Test Verification

- [x] `go build ./...` — PASS
- [x] `go test ./internal/sms/...` — phone utility + provider tests PASS
- [x] `go test ./internal/auth/...` — no regressions from phone utility refactor
- [x] `go test ./internal/server/...` — all messaging endpoint tests PASS
- [x] `go test ./internal/config/...` — config tests still PASS
- [x] Final commit if needed

## Master Stages
1. [x] SMS Provider Layer
2. [x] Database & Config
3. [x] Auth Service & Handlers
4. [x] Server Wiring & Integration Tests
5. [x] SMS MFA Second Factor
6. [x] Provider Expansion
7. [x] Fraud Hardening & Monitoring
8. [x] Transactional SMS API — phone utility refactor, messages table, send endpoint, delivery webhook
