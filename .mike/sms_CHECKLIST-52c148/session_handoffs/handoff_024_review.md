# Handoff 024 — Stage 6 Review (Provider Implementations)

## What was reviewed

Full code review of all Stage 6 provider implementations (Steps 1-6) and their test suites:
- `internal/sms/plivo.go` + `plivo_test.go`
- `internal/sms/telnyx.go` + `telnyx_test.go`
- `internal/sms/msg91.go` + `msg91_test.go`
- `internal/sms/sns.go` + `sns_test.go`
- `internal/sms/vonage.go` + `vonage_test.go`
- `internal/sms/webhook.go` + `webhook_test.go`

Also reviewed related earlier code: `twilio.go`, `sms_auth.go`, `sms_mfa.go`.

## Bugs found and fixed: 5

### 1. MSG91: JSON parse before HTTP status code check (`msg91.go`)
**Severity: Medium.** If MSG91 (or an intermediary proxy) returned a non-JSON error response (e.g., HTML 502 Bad Gateway), the code tried `json.Unmarshal` first and produced a confusing "msg91: parse response: invalid character '<'..." error instead of a clear HTTP error. Fixed: check `resp.StatusCode >= 300` first, attempt JSON parse of error body, fall back to raw body string.

### 2. Vonage: JSON parse before HTTP status code check (`vonage.go`)
**Severity: Medium.** Same issue as MSG91. Vonage API normally returns 200 even for errors, but proxies/CDNs can return non-200 with HTML. Fixed: added `resp.StatusCode >= 300` check before JSON parse.

### 3. Twilio: JSON parse before HTTP status code check (`twilio.go`)
**Severity: Low.** Same pattern. Twilio always returns JSON, but proxy errors would produce confusing parse errors. Fixed for consistency with Plivo/Telnyx/Webhook pattern.

### 4. MSG91: Missing test for 200 OK with error type in body (`msg91_test.go`)
**Severity: High (false positive risk).** MSG91 API returns HTTP 200 with `{"type":"error","message":"..."}` for some errors. The `parsed.Type == "error"` check at line 82 had zero test coverage. If someone accidentally removed that check, tests would still pass. Added `TestMSG91SendErrorHTTP200WithErrorType`.

### 5. Missing non-JSON error body tests across providers
**Severity: Medium (false positive risk).** The new HTTP status code checks had no coverage for the non-JSON case. Added:
- `TestMSG91SendErrorNonJSON`
- `TestVonageSendHTTPError`
- `TestTwilioSendHTTPErrorNonJSON`

## Checklist deviations noted (not bugs)

**SNS: Simplified interface, no AWS SDK dependency.** The checklist specified `SNSPublisher` should use `*sns.PublishInput`/`*sns.PublishOutput` from the AWS SDK, plus `NewSNSProviderFromConfig` and `github.com/aws/aws-sdk-go-v2/service/sns` dependency. The implementation uses a simplified `Publish(ctx, phoneNumber, message string) (messageID string, error)` interface instead. This is actually a better design (keeps AWS SDK out of the sms package), but it means Step 9 (CLI wiring) will need to create an adapter. Updated checklist to reflect this.

## Test results
- `go test ./internal/sms/...` — 34 tests PASS
- `go build ./...` — PASS

## What's next
Steps 7-10 remain:
- **Step 7**: Test phone number tests (code already exists in `sendOTPToPhone`, needs test coverage in `sms_auth_test.go`)
- **Step 8**: Config fields & validation for all new providers
- **Step 9**: CLI wiring for all providers (including SNS AWS SDK adapter)
- **Step 10**: Build & test verification

## Files modified
- `internal/sms/msg91.go` — Fixed error handling order: HTTP status check before JSON parse
- `internal/sms/msg91_test.go` — Added `TestMSG91SendErrorHTTP200WithErrorType`, `TestMSG91SendErrorNonJSON`
- `internal/sms/vonage.go` — Added HTTP status code check before JSON parse
- `internal/sms/vonage_test.go` — Added `TestVonageSendHTTPError`
- `internal/sms/twilio.go` — Fixed error handling order: HTTP status check before JSON parse
- `internal/sms/twilio_test.go` — Added `TestTwilioSendHTTPErrorNonJSON`
- `.mike/sms_CHECKLIST-52c148/checklists/stage_06_checklist.md` — Updated: checked Steps 1-6, noted SNS deviation, updated Step 7 status

## Checklist path
- `.mike/sms_CHECKLIST-52c148/checklists/stage_06_checklist.md`

## Previous handoff
- `.mike/sms_CHECKLIST-52c148/session_handoffs/handoff_022_review.md`
