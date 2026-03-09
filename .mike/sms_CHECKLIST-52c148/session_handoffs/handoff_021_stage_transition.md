# Handoff 021 — Stage 5→6 Transition

## What was done

### Stage 5 Verification
- All 12 steps + review fixes confirmed checked off in `stage_05_checklist.md`
- `go build ./...` — PASS
- `go test ./internal/auth/...` — PASS (cached, all green)
- `go test ./internal/sms/...` — PASS (cached, all green)
- `stages.md` updated: Stage 5 marked `[x]`

### Stage 6 Checklist Generated
Created `checklists/stage_06_checklist.md` with 10 steps covering:
- Steps 1-6: Six new SMS providers (Plivo, Telnyx, MSG91, AWS SNS, Vonage, Webhook) — each TDD with httptest mocks
- Step 7: Test phone numbers — config-driven bypass with predetermined codes
- Step 8: Config fields & validation for all new providers
- Step 9: CLI wiring (`buildSMSProvider` switch expansion)
- Step 10: Build & test verification

### Design decisions for Stage 6
- **Prelude provider omitted** — requires external Go SDK dependency (`github.com/prelude-so/go-sdk`) that may not be stable. Can be added later if needed. The webhook provider serves as a generic escape hatch for any unsupported provider.
- **AWS SNS uses interface mock** instead of httptest — AWS SDK handles its own HTTP, so testing via a `SNSPublisher` interface is cleaner than trying to mock AWS endpoints.
- **MSG91 uses template-based API** (`/api/v5/flow/`) since MSG91 requires DLT-registered templates for India delivery.
- **Test phone numbers** implemented at the auth service level (not provider level) — the predetermined code is stored in `_ayb_sms_codes` like any other OTP, but `provider.Send()` is skipped.
- **Each provider follows the Twilio pattern**: struct with credentials + baseURL for testability, constructor, `Send()` method returning `(*SendResult, error)`.

## What's next
Stage 6 implementation: start with Step 1 (Plivo Provider). Each provider step is independent and can be worked in any order.

## Files created/modified
- `checklists/stage_06_checklist.md` — new Stage 6 checklist (10 steps)
- `stages.md` — Stage 5 marked complete

## Current provider infrastructure (reference for Stage 6)
- `internal/sms/sms.go` — `Provider` interface: `Send(ctx, to, body) (*SendResult, error)`, `Config` struct
- `internal/sms/twilio.go` — reference implementation pattern (baseURL injection, httptest-friendly)
- `internal/sms/twilio_test.go` — reference test pattern (httptest mock server)
- `internal/config/config.go` — config validation switch on `SMSProvider`, env var overrides
- `internal/cli/start.go:773` — `buildSMSProvider()` switch for provider instantiation
