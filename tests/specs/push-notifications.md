# Push Notifications Test Specification (Stage 6)

## Scope

Stage 6 validates provider-abstracted push notifications via FCM and APNS:

- device token CRUD (admin + user-facing with ownership validation)
- FCM and APNS provider adapters (hand-rolled HTTP, no third-party SDKs)
- async delivery via Stage 3 job queue with retries and exponential backoff
- stale token cleanup (270-day inactivity threshold)
- provider auth token caching (FCM OAuth2, APNS ES256 JWT)
- payload size validation (4KB limit)
- admin API/CLI/dashboard with delivery audit trail

## Test matrix

| Area | Required behavior | Automated coverage |
|---|---|---|
| FCM credential loading | Parse service account JSON, extract project ID | `internal/push/fcm_test.go` (`TestFCMProviderNewRequiresProjectID`) |
| FCM OAuth2 token | RS256 JWT grant → access token, caching, refresh near expiry | `internal/push/fcm_test.go` (`TestFCMProviderAccessTokenCachingAndRefresh`) |
| FCM send | POST to FCM v1 API with correct headers, body, auth | `internal/push/fcm_test.go` (`TestFCMProviderSend*`) |
| FCM error mapping | INVALID_ARGUMENT, UNREGISTERED, QUOTA_EXCEEDED, UNAVAILABLE, INTERNAL, etc. | `internal/push/fcm_test.go` (`TestFCMProviderErrorMapping`) |
| APNS key loading | Parse PKCS8 EC private key from .p8 file | `internal/push/apns_test.go` (`TestNewAPNSProviderEnvironmentDefaults`) |
| APNS JWT generation | ES256 JWT with team ID issuer, key ID header, caching, refresh | `internal/push/apns_test.go` (`TestAPNSProviderJWT*`) |
| APNS send | POST to APNS `/3/device/{token}` with correct headers, payload | `internal/push/apns_test.go` (`TestAPNSProviderSend*`) |
| APNS error mapping | BadDeviceToken, Unregistered, ExpiredToken, TooManyRequests, etc. | `internal/push/apns_test.go` (`TestAPNSProviderErrorMapping*`) |
| APNS 410 fallback | `410 Gone` with empty body → ErrUnregistered | `internal/push/apns_test.go` (`TestAPNSProviderErrorMappingFallbackByStatus`) |
| LogProvider | Logs message, returns fake ID | `internal/push/push_test.go` (`TestLogProviderSend`) |
| CaptureProvider | Records calls for test assertions | `internal/push/push_test.go` (`TestCaptureProviderSendAndReset`) |
| Token registration validation | Empty token rejected, invalid provider/platform rejected | `internal/push/service_test.go` (`TestServiceRegisterToken*`) |
| Send fan-out | N active tokens → N deliveries + N enqueued jobs | `internal/push/service_test.go` (`TestServiceSendToUser*`) |
| Send to single token | Active token succeeds, inactive token rejected | `internal/push/service_test.go` (`TestServiceSendToToken*`) |
| Payload size validation | >4KB payload rejected before send | `internal/push/service_test.go` (`TestServiceSendToUserRejectsOversizedPayload`) |
| Empty title/body | Rejected with ErrInvalidPayload | `internal/push/service_test.go` (`TestServiceSendToUserEmptyTitleBody`) |
| Zero tokens | No deliveries created, no error | `internal/push/service_test.go` (`TestServiceSendToUserZeroTokens`) |
| Delivery processing (success) | Status → sent, provider message ID set, last_used updated | `internal/push/service_test.go` (`TestServiceProcessDeliverySuccess`) |
| Delivery processing (permanent) | ErrUnregistered/ErrInvalidToken → status=invalid_token, token inactive, no retry | `internal/push/service_test.go` (`TestServiceProcessDeliveryInvalidToken*`) |
| Delivery processing (transient) | ErrProviderError → status=failed, returns error for retry | `internal/push/service_test.go` (`TestServiceProcessDeliveryTransientFailureReturnsError`) |
| Delivery processing (auth) | ErrProviderAuth → returns error for retry | `internal/push/service_test.go` (`TestServiceProcessDeliveryProviderAuthReturnsError`) |
| Enqueue failure | Partial fan-out failure marks delivery failed | `internal/push/service_test.go` (`TestServiceSendToUserEnqueueFailureMarksDeliveryFailed`) |
| Stale token cleanup | Tokens >270 days without refresh → marked inactive | `internal/push/service_test.go` (`TestServiceRunTokenCleanup`) |
| Provider case insensitivity | Uppercase provider keys normalized in NewService | `internal/push/service_test.go` (`TestServiceProviderCaseInsensitivity`) |
| List deliveries passthrough | Parameters forwarded to store | `internal/push/service_test.go` (`TestServiceListDeliveries`) |
| Get delivery passthrough | ID forwarded to store | `internal/push/service_test.go` (`TestServiceGetDelivery`) |
| Job handler (push_delivery) | Deserializes payload, calls ProcessDelivery | `internal/push/service_test.go` (`TestPushDeliveryJobHandler*`) |
| Job handler (push_token_cleanup) | Calls CleanupStaleTokens(270) | `internal/push/service_test.go` (`TestPushTokenCleanupJobHandler*`) |
| Migration SQL constraints | Provider/platform enum checks, token length, title/body length, status enum, data_payload size | `internal/migrations/push_sql_test.go` |
| Migration FK cascades | Delete user/app → tokens/deliveries cascaded | `internal/migrations/push_migrations_integration_test.go` (`-tags integration`) |
| Admin list devices | Filters by app_id, user_id, include_inactive | `internal/server/push_handler_test.go` (`TestAdminPushListDevices*`) |
| Admin register device | Creates token via admin API | `internal/server/push_handler_test.go` (`TestAdminPushRegisterDevice*`) |
| Admin revoke device | Revokes token, 404 on missing | `internal/server/push_handler_test.go` (`TestAdminPushRevokeDevice*`) |
| Admin send | Fans out to user, required field validation | `internal/server/push_handler_test.go` (`TestAdminPushSend*`) |
| Admin send to token | Sends to specific token | `internal/server/push_handler_test.go` (`TestAdminPushSendToToken*`) |
| Admin list deliveries | Filters by app_id, user_id, status; rejects invalid status | `internal/server/push_handler_test.go` (`TestAdminPushListDeliveries*`) |
| Admin get delivery | Returns delivery detail, 404 on missing | `internal/server/push_handler_test.go` (`TestAdminPushGetDelivery*`) |
| User register device | JWT auth, user_id from claims | `internal/server/push_handler_test.go` (`TestUserPushRegister*`) |
| User list devices | JWT auth, app_id required, own tokens only | `internal/server/push_handler_test.go` (`TestUserPushListDevices*`) |
| User revoke device | Ownership validation (can't revoke another user's token) | `internal/server/push_handler_test.go` (`TestUserPushRevokeDevice*`) |
| Server push routes wiring | SetPushService gating, 503 when not configured | `internal/server/push_handler_test.go` (`TestServerPushAdminRoutes*`) |
| Push not enabled | Returns 503 when push service not set | `internal/server/push_handler_test.go` |
| CLI list-devices | Query params, table/JSON/CSV output | `internal/cli/push_cli_test.go` (`TestPushListDevices*`) |
| CLI register-device | Required flags, provider/platform validation | `internal/cli/push_cli_test.go` (`TestPushRegisterDevice*`) |
| CLI revoke-device | Sends DELETE, success message | `internal/cli/push_cli_test.go` (`TestPushRevokeDevice*`) |
| CLI send | Required flags, data parsing, delivery count output | `internal/cli/push_cli_test.go` (`TestPushSend*`) |
| CLI list-deliveries | Status filter validation, output formats | `internal/cli/push_cli_test.go` (`TestPushListDeliveries*`) |
| CLI provider/platform validation | Rejects invalid enums with local error | `internal/cli/push_cli_test.go` (`TestPushRegisterDeviceRejectsInvalid*`) |
| Config validation | push.enabled requires jobs.enabled, provider field validation, FCM JSON validation | `internal/config/config_test.go` (`TestValidatePushFCM*`) |
| Startup wiring | buildPushProviders, pushProviderNames helpers, cleanup schedule registration | `internal/cli/start_push_test.go` |
| UI component: devices tab | Table rendering, register/revoke workflows, filter application | `ui/src/components/__tests__/PushNotifications.test.tsx` |
| UI component: deliveries tab | Table rendering, status filtering, send-test modal, expandable details | `ui/src/components/__tests__/PushNotifications.test.tsx` |
| UI component: filter trim | Filters trim whitespace on apply, refresh uses applied filters | `ui/src/components/__tests__/PushNotifications.test.tsx` |
| UI nav wiring | Layout and CommandPalette include push entry | `ui/src/components/__tests__/Layout.test.tsx`, `CommandPalette.test.tsx` |
| Browser-mocked push flows | Register + send + filter flows with API intercepts | `ui/browser-tests-mocked/push-notifications.spec.ts` |
| Browser-unmocked fixtures | Fixture SQL correctness, isPushEnabled status handling, SQL literal escaping | `ui/src/__tests__/browser_unmocked_push_fixtures.test.ts` |
| Browser-unmocked lifecycle | Register device → send push → verify delivery lifecycle | `ui/browser-tests-unmocked/full/push-notifications-lifecycle.spec.ts` |

## Focused command set

Use focused commands for Stage 6 verification:

```bash
go test ./internal/push -count=1
go test ./internal/server -run Push -count=1
go test ./internal/cli -run 'TestPush|TestBuildPushProviders|TestRegisterPushTokenCleanupSchedule' -count=1
go test ./internal/migrations -run 'TestPushMigration' -count=1
go test ./internal/config -run 'TestValidatePush' -count=1
```

UI/component and browser-mocked checks:

```bash
cd ui && npm test -- src/components/__tests__/PushNotifications.test.tsx src/components/__tests__/Layout.test.tsx src/components/__tests__/CommandPalette.test.tsx src/__tests__/browser_unmocked_push_fixtures.test.ts
cd ui && npm run lint:browser-tests:mocked -- browser-tests-mocked/push-notifications.spec.ts
cd ui && npm run lint:browser-tests
cd ui && npx playwright test browser-tests-unmocked/full/push-notifications-lifecycle.spec.ts --list
```

Integration-tag tests remain environment-dependent on `TEST_DATABASE_URL`.
Browser-unmocked runtime execution is environment-dependent in sandboxed CI/dev shells where Chromium launch or local webserver binding is blocked.

## Browser 3-tier status

- Tier 1 (component): complete
- Tier 2 (browser-mocked): complete
- Tier 3 (browser-unmocked): complete (runtime execution still environment-dependent for sandboxed shells)
