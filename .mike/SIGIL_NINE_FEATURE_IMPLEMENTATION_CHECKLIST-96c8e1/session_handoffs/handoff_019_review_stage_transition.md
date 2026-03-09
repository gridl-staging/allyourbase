# Handoff 019 — Stage 2 Checklist Review

## What I did

1. **Verified Stage 1 completion**: All 35 checklist items confirmed checked. stages.md already marked complete.

2. **Researched OAuth 2.0 best practices**: Reviewed RFC 9700 (OAuth 2.0 Security BCP, published Jan 2025) and OAuth 2.1 draft guidance. Key findings:
   - PKCE required for ALL clients (not just public)
   - Implicit and ROPC grants deprecated — do not implement
   - Refresh token rotation with reuse detection mandatory
   - Exact redirect_uri matching, no wildcards
   - S256 only for PKCE (plain method prohibited)

3. **Critically reviewed Stage 2 checklist** — found and fixed 9 significant issues:

   | Issue | Previous | Corrected |
   |-------|----------|-----------|
   | Token format | JWT with Claims extension + revocation check | Opaque tokens (SHA-256 hash in DB) |
   | PKCE scope | Required for public clients, plain optional | Required for ALL clients, S256 only |
   | Scope model | New `api:read`/`api:write`/`api:*` | Reuse existing `readonly`/`readwrite`/`*` + AllowedTables |
   | Refresh reuse | Not addressed | Reuse detection: revoke all grant tokens on double-use |
   | Client auth | Not specified | HTTP Basic or POST body per RFC 6749 §2.3 |
   | Error format | Not specified | RFC 6749 §5.2 (`error` + `error_description`) |
   | State param | Mentioned in passing | Explicitly required, reject if missing |
   | Client ID format | Unspecified | `ayb_cid_` prefix + 24 random hex |
   | Token introspection | In scope (RFC 7662) | Moved to non-goals (single-server BaaS doesn't need it) |

4. **Redesigned tokens table**: Previous plan used paired `access_token_hash` + `refresh_token_hash` columns in a single row. Changed to single `token_hash` + `token_type` per row with shared `grant_id`, enabling independent lifecycle and simpler reuse detection.

5. **Added token prefixes**: `ayb_at_` for access tokens, `ayb_rt_` for refresh tokens — aids debugging, log analysis, and middleware token-type detection.

## Rationale: Opaque tokens over JWTs

The previous checklist proposed extending the JWT Claims struct with `OAuthClientID` and `OAuthScopes` fields, then checking revocation status in `ValidateToken` on every request. This is the worst of both worlds:
- JWTs exist to avoid DB lookups — but revocation checks require a DB lookup anyway
- Mixing OAuth claims into session JWTs creates a confusing dual-purpose token
- Opaque tokens are simpler: generate random bytes, hash for storage, lookup on use
- Clean separation: session tokens remain JWTs, OAuth tokens are opaque DB-backed tokens

## Key architectural decisions for Stage 2

- **Opaque tokens**: Random bytes with prefix (`ayb_at_`/`ayb_rt_`), stored as SHA-256 hash
- **PKCE for all clients**: S256 only, per RFC 9700 and OAuth 2.1
- **Scope alignment**: Reuse `readonly`/`readwrite`/`*` from API key model, not new scope strings
- **Grant-level tracking**: `grant_id` UUID links access + refresh tokens for cascade revocation
- **No introspection endpoint**: Single-server BaaS; token validation is internal only
- **Client IDs**: `ayb_cid_` prefix for consistency with `ayb_` API key prefix
- **Middleware integration**: `validateTokenOrAPIKey` extended to try opaque token lookup when JWT parse fails

## Files modified

- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/checklists/stage_02_checklist.md` (rewritten)
- `_dev/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST.md` (added review note)
- `.mike/SIGIL_NINE_FEATURE_IMPLEMENTATION_CHECKLIST-96c8e1/session_handoffs/handoff_019_review_stage_transition.md` (this file)

## What's next

- Begin Stage 2 implementation: Discovery & Design phase
- First implementation session should write the ADR, then tackle database schema migrations (019-022)
- After schema: client registration backend + tests (TDD)

## References

- RFC 9700: Best Current Practice for OAuth 2.0 Security (Jan 2025)
- RFC 6749: The OAuth 2.0 Authorization Framework
- RFC 7636: Proof Key for Code Exchange (PKCE)
- RFC 7009: OAuth 2.0 Token Revocation
- RFC 8252: OAuth 2.0 for Native Apps (localhost redirect guidance)
