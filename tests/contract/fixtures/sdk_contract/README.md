<!--
Stage 7 Parity Report (Cross-SDK Deserialization)

Fixtures:
- auth_response.json
- list_response.json
- error_response_numeric_code.json
- error_response_string_code.json
- storage_object.json
- storage_list_response.json
- realtime_event.json

| SDK      | Fixtures Verified | Naming Notes | Gaps Found | Resolution | Final |
|----------|-------------------|--------------|------------|------------|-------|
| Swift    | auth, list, error(numeric+string), storage_object, storage_list, realtime_event | camelCase model fields with snake_case alias support where needed | Missing explicit error/storage-list contract tests | Added tests in ContractFixtureTests and aligned fixtures | PASS |
| Kotlin   | auth, list, error(numeric+string), storage_object, storage_list, realtime_event | camelCase fields with @JsonNames aliases for snake_case | Storage/realtime fixture values diverged from canonical | Updated ContractFixtureTest payloads to canonical values | PASS |
| Go       | auth, list, error(numeric+string), storage_object, storage_list | camelCase struct fields; no RealtimeEvent type in sdk_go | Nullable fields modeled as non-pointer; numeric error code not normalized | Updated model nullability + normalizeError code coercion; expanded contract tests | PASS |
| Python   | auth, list, error(numeric+string), storage_object, storage_list, realtime_event | snake_case model attrs with camelCase aliases | Numeric code parsing, nullable storage updated_at, reconnect test spin, httpx_mock plugin missing locally | Added parsing/nullability fixes, reconnect yield fix, installed pytest-httpx, expanded contract tests | PASS |
| Dart     | auth, list, error(numeric+string), storage_object, storage_list, realtime_event | camelCase primary fields with snake_case alias handling added for contract parity | Missing storage-list contract case; missing alias support for auth/realtime/storage nulls | Added canonical contract assertions + alias/nullable support in SDK types | PASS |
| JS       | auth, list, error(numeric+string), storage_object, storage_list | camelCase response interfaces; runtime normalization now handles snake/camel aliases | No dedicated contract suite; numeric code/docUrl/user alias normalization missing | Added src/contract.test.ts and normalization helpers in core client | PASS |
| React    | auth, list (via core SDK parsing + hook consumption) | hook-facing types are structural wrappers over core SDK | No contract suite | Added tests/contract.test.tsx using core SDK + canonical fixtures | PASS |
| SSR      | auth (via core refresh path consumed by loadServerSession) | session layer consumes core-parsed auth payloads | No contract suite | Added tests/contract.test.ts for canonical auth fixture through SSR flow | PASS |

Summary:
- Field naming conventions: Python exposes snake_case attributes but accepts camelCase aliases; JS/Swift/Kotlin/Dart/Go use camelCase primary API with alias handling where required by server payload variants.
- Realtime fixture in Go is intentionally skipped because sdk_go has no RealtimeEvent model.
- All 8 SDK suites passed in Stage 7 verification run.
-->

# SDK Contract Fixtures

Canonical server response shapes for cross-SDK deserialization parity tests.

Rules:
- Fixtures in this directory are pure JSON payloads (no metadata wrapper).
- Fixtures preserve canonical wire keys exactly as defined by server contracts (mostly camelCase, with required snake_case fields such as `email_verified`, `created_at`, `updated_at`, and `doc_url` where applicable).
- SDK-specific tests may additionally validate alias support (for example snake_case), but these files are the single canonical baseline.
