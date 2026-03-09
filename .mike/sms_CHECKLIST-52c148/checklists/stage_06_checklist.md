# Stage 6: Provider Expansion

## Step 1: Plivo Provider (TDD)

- [x] RED: Create `internal/sms/plivo_test.go`:
  - `TestPlivoSendSuccess`: httptest mock, verify HTTP Basic auth (`authID:authToken`), POST JSON body `{"src": from, "dst": to, "text": body}` to `/v1/Account/{authID}/Message/`, assert response parsed (`message_uuid`, status)
  - `TestPlivoSendError`: mock returns 4xx, verify error includes Plivo error message
  - `TestPlivoSendNetworkError`: unreachable server, verify wrapped error
- [x] GREEN: Create `internal/sms/plivo.go`:
  - `PlivoProvider` struct: `authID`, `authToken`, `fromNumber`, `baseURL`, `client http.Client`
  - `NewPlivoProvider(authID, authToken, fromNumber, baseURL string) *PlivoProvider` — default baseURL `https://api.plivo.com`
  - `Send(ctx, to, body)` — POST JSON, HTTP Basic auth, parse response `{"message_uuid": [...], "api_id": "..."}`, return first UUID as MessageID
- [x] `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add PlivoProvider for SMS delivery`

## Step 2: Telnyx Provider (TDD)

- [x] RED: Create `internal/sms/telnyx_test.go`:
  - `TestTelnyxSendSuccess`: httptest mock, verify Bearer token auth header, POST JSON body `{"from": from, "to": to, "text": body}` to `/v2/messages`, assert response parsed (`data.id`, `data.type`)
  - `TestTelnyxSendError`: mock returns 4xx with JSON error, verify error message
  - `TestTelnyxSendNetworkError`: unreachable server
- [x] GREEN: Create `internal/sms/telnyx.go`:
  - `TelnyxProvider` struct: `apiKey`, `fromNumber`, `baseURL`, `client http.Client`
  - `NewTelnyxProvider(apiKey, fromNumber, baseURL string) *TelnyxProvider` — default baseURL `https://api.telnyx.com`
  - `Send(ctx, to, body)` — POST JSON, `Authorization: Bearer <key>`, parse response `{"data": {"id": "...", "type": "message"}}`, return `data.id` as MessageID
- [x] `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add TelnyxProvider for SMS delivery`

## Step 3: MSG91 Provider (TDD)

- [x] RED: Create `internal/sms/msg91_test.go`:
  - `TestMSG91SendSuccess`: httptest mock, verify `authkey` header, POST JSON to `/api/v5/flow/`, assert template-based request body `{"template_id": tmplID, "recipients": [{"mobiles": to, "otp": body}]}`
  - `TestMSG91SendError`: mock returns error response
  - `TestMSG91SendNetworkError`: unreachable server
- [x] GREEN: Create `internal/sms/msg91.go`:
  - `MSG91Provider` struct: `authKey`, `templateID`, `baseURL`, `client http.Client`
  - `NewMSG91Provider(authKey, templateID, baseURL string) *MSG91Provider` — default baseURL `https://control.msg91.com`
  - `Send(ctx, to, body)` — POST JSON, `authkey` header, parse response for request ID
- [x] `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add MSG91Provider for SMS delivery`

## Step 4: AWS SNS Provider (TDD)

- [x] RED: Create `internal/sms/sns_test.go`:
  - `TestSNSSendSuccess`: mock SNS client interface, verify `Publish` called with correct `PhoneNumber`, `Message`, returns `MessageId`
  - `TestSNSSendError`: mock returns AWS error, verify wrapped error
  - Note: use interface mock, not httptest — AWS SDK has its own HTTP handling
- [x] GREEN: Create `internal/sms/sns.go`:
  - `SNSPublisher` interface: `Publish(ctx, phoneNumber, message string) (messageID string, error)` — simplified for testability (does NOT use AWS SDK types directly)
  - `SNSProvider` struct: `publisher SNSPublisher`
  - `NewSNSProvider(publisher SNSPublisher) *SNSProvider`
  - **NOTE**: `NewSNSProviderFromConfig` and `github.com/aws/aws-sdk-go-v2/service/sns` dependency are deferred to Step 9 (CLI wiring). The adapter that wraps the real AWS SNS client into `SNSPublisher` will be created there.
  - `Send(ctx, to, body)` — calls `Publish`, returns messageID
- [x] `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add SNSProvider for SMS delivery via AWS`

## Step 5: Vonage Provider (TDD)

- [x] RED: Create `internal/sms/vonage_test.go`:
  - `TestVonageSendSuccess`: httptest mock, verify POST form/JSON to `/sms/json` with `api_key`, `api_secret`, `from`, `to`, `text`, parse response `{"messages": [{"message-id": "...", "status": "0"}]}`
  - `TestVonageSendError`: mock returns non-zero status in messages array
  - `TestVonageSendNetworkError`: unreachable server
- [x] GREEN: Create `internal/sms/vonage.go`:
  - `VonageProvider` struct: `apiKey`, `apiSecret`, `fromNumber`, `baseURL`, `client http.Client`
  - `NewVonageProvider(apiKey, apiSecret, fromNumber, baseURL string) *VonageProvider` — default baseURL `https://rest.nexmo.com`
  - `Send(ctx, to, body)` — POST form-encoded, parse JSON response, check `messages[0].status == "0"`, return `message-id` as MessageID
- [x] `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add VonageProvider for SMS delivery`

## Step 6: Webhook Provider (TDD)

- [x] RED: Create `internal/sms/webhook_test.go`:
  - `TestWebhookSendSuccess`: httptest mock, verify POST JSON body `{"to": to, "body": body, "timestamp": "..."}`, verify `X-Webhook-Signature` header (HMAC-SHA256 of request body using secret), parse response `{"message_id": "..."}`
  - `TestWebhookSendError`: mock returns non-2xx
  - `TestWebhookSendSignatureVerification`: verify signature computation is correct (compute independently and compare)
  - `TestWebhookSendNetworkError`: unreachable server
- [x] GREEN: Create `internal/sms/webhook.go`:
  - `WebhookProvider` struct: `url`, `secret`, `client http.Client`
  - `NewWebhookProvider(url, secret string) *WebhookProvider`
  - `Send(ctx, to, body)` — POST JSON, compute HMAC-SHA256 of body with secret, set `X-Webhook-Signature` header, parse response for `message_id`
- [x] `go test ./internal/sms/...` — confirm PASS
- [x] Commit: `feat: add WebhookProvider for custom SMS delivery`

## Step 7: Test Phone Numbers (TDD)

- [x] GREEN: Implementation already exists in `internal/auth/sms_mfa.go:sendOTPToPhone()`:
  - `TestPhoneNumbers map[string]string` field already in `sms.Config`
  - `sendOTPToPhone` checks `TestPhoneNumbers` map, uses predetermined code, skips `provider.Send()`
- [x] RED: Tests exist in `internal/auth/auth_integration_test.go` (not `sms_auth_test.go` — requires DB):
  - `TestRequestSMSCode_TestPhoneNumber`: configured test phone `+15550001234` → no provider.Send() called, code still stored
  - `TestConfirmSMSCode_TestPhoneNumber`: test phone with predetermined code `000000` → succeeds without provider
  - `TestRequestSMSCode_TestPhoneNumber_NotConfigured`: test phone feature disabled → normal flow
- [x] `go test ./internal/auth/...` — confirm PASS (integration tests pass with managed Postgres)
- [x] Commit: `feat: add test phone number support for SMS OTP` (already committed in prior sessions)

## Step 8: Config Fields & Validation for All New Providers

- [x] RED: Add tests in `internal/config/config_test.go`:
  - `TestValidate_SMSProvider_Plivo`: requires `plivo_auth_id`, `plivo_auth_token`, `plivo_from`
  - `TestValidate_SMSProvider_Telnyx`: requires `telnyx_api_key`, `telnyx_from`
  - `TestValidate_SMSProvider_MSG91`: requires `msg91_auth_key`, `msg91_template_id`
  - `TestValidate_SMSProvider_SNS`: requires `aws_region` (creds from env)
  - `TestValidate_SMSProvider_Vonage`: requires `vonage_api_key`, `vonage_api_secret`, `vonage_from`
  - `TestValidate_SMSProvider_Webhook`: requires `sms_webhook_url`, `sms_webhook_secret`
  - `TestValidate_SMSProvider_Invalid`: unknown provider name → error
- [x] GREEN: Update `internal/config/config.go`:
  - Add config struct fields for each provider's credentials (TOML tags)
  - Add cases to `Validate()` switch for each provider
  - Add env var overrides in `ApplyEnv()`
  - Update `sms_provider` error message to list all valid providers
  - Add fields to `knownKeys` map and `GetConfigValue`/`SetConfigValue`
  - Update TOML template with commented examples for each provider
- [x] `go test ./internal/config/...` — confirm PASS
- [x] Commit: `feat: add config fields and validation for all SMS providers`

## Step 9: CLI Wiring for All Providers

- [x] Update `buildSMSProvider()` in `internal/cli/start.go`:
  - Add switch cases: `"plivo"`, `"telnyx"`, `"msg91"`, `"sns"`, `"vonage"`, `"webhook"`
  - Each case constructs the provider from config fields
  - SNS case: create adapter that wraps `sns.Client` into `SNSPublisher` interface, add AWS SDK dependency here
  - Add 10DLC / registration warnings for providers that require it (Plivo, Vonage)
- [x] Add test phone number config wiring: pass `TestPhoneNumbers` through to `sms.Config`
- [x] `go test ./internal/cli/...` — confirm PASS
- [x] Commit: `feat: wire all SMS providers into CLI startup`

## Step 10: Build & Test Verification

- [x] Run `go build ./...` — confirm PASS
- [x] Run `go test ./internal/sms/...` — confirm all provider tests PASS (42 tests)
- [x] Run `go test ./internal/auth/...` — confirm test phone number tests PASS
- [x] Run `go test ./internal/config/...` — confirm config validation tests PASS
- [x] Run `go test ./internal/cli/...` — confirm CLI tests PASS
- [x] Final commit if needed — fix: add missing GetValue case for sms_test_phone_numbers

## Master Stages
1. [x] SMS Provider Layer
2. [x] Database & Config
3. [x] Auth Service & Handlers
4. [x] Server Wiring & Integration Tests
5. [x] SMS MFA Second Factor
6. Provider Expansion — Plivo, Telnyx, MSG91, AWS SNS, Vonage, webhook provider, test phone numbers
7. Fraud Hardening & Monitoring
8. Transactional SMS API
