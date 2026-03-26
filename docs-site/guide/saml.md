# SAML SSO
<!-- audited 2026-03-20 -->

This guide documents AYB's shipped SAML surface for service-provider (SP) login flow and admin provider management.

Source of truth:

- `internal/auth/saml.go`, `internal/auth/handler_oauth.go`
- `internal/server/saml_handler.go`, `internal/server/routes_auth.go`, `internal/server/routes_admin.go`
- `internal/config/config_types.go::SAMLProvider`, `internal/config/config_validate_auth.go`
- `internal/config/config_default_toml.go` SAML examples
- Tests: `internal/auth/saml_test.go`, `internal/server/saml_handler_test.go`, `internal/config/config_test.go`

## What AYB supports

- SP-initiated login redirect: `GET /api/auth/saml/{provider}/login`
- ACS callback handling: `POST /api/auth/saml/{provider}/acs`
- SP metadata endpoint: `GET /api/auth/saml/{provider}/metadata`
- Admin CRUD for providers in `_ayb_saml_providers`
- Metadata source via either URL fetch or inline XML in admin CRUD flows
- Attribute mapping for `email` and `name` during JIT login

## Config-based providers

`auth.saml_providers` entries (`SAMLProvider`):

- `enabled`
- `name`
- `entity_id`
- `idp_metadata_url`
- `idp_metadata_xml`
- `attribute_mapping`
- `sp_cert_file`
- `sp_key_file`

Validation rules (`config_validate_auth.go`):

- `auth.enabled` must be true for enabled SAML providers
- `name` required
- `entity_id` required
- one of `idp_metadata_url` or `idp_metadata_xml` required
- enabled provider names must be unique

Current runtime behavior note:

- Config validation accepts either `idp_metadata_url` or `idp_metadata_xml`.
- Startup registration path (`routes_auth.go` -> `SAMLService.UpsertProvider`) currently consumes `idp_metadata_xml`; it does not fetch metadata from `idp_metadata_url`.
- URL-based metadata fetch is implemented in the admin SAML create/update API (`resolveSAMLMetadata`).

For config-based startup registration today, resolve/export the IdP metadata first and paste the XML into `idp_metadata_xml`.

Example config:

```toml
[auth]
enabled = true
jwt_secret = "replace-with-32+-char-secret"

[[auth.saml_providers]]
enabled = true
name = "okta"
entity_id = "https://api.example.com/api/auth/saml/okta/metadata"
idp_metadata_xml = """
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata">
  <!-- Paste the IdP metadata document here for startup registration. -->
</EntityDescriptor>
"""
# idp_metadata_url = "https://dev-123456.okta.com/app/xyz/sso/saml/metadata"
# sp_cert_file = ""
# sp_key_file = ""

[auth.saml_providers.attribute_mapping]
email = "email"
name = "name"
groups = "groups"
```

Provider-name format is validated by `ValidateSAMLProviderName`:

- regex: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`
- no slashes, dots, or path traversal patterns

## IdP setup patterns

AYB supports any SAML 2.0 IdP that exposes metadata. Common setups:

- Okta: use the app metadata URL in the admin API, or export/paste the metadata XML for config-based startup
- Azure AD: use the Enterprise App federation metadata URL, or export/paste the metadata XML for config-based startup
- Google Workspace: use the metadata URL/XML from SAML app settings; config-based startup currently needs inline XML

In all cases, set the ACS URL in the IdP to:

- `https://<your-ayb-host>/api/auth/saml/<provider>/acs`

And set SP entity ID to your configured `entity_id`.

## SP metadata exchange

Fetch metadata:

```bash
curl http://localhost:8090/api/auth/saml/okta/metadata
```

Returned XML includes:

- `<EntityDescriptor entityID="...">`
- signing cert (from generated or configured SP cert)
- ACS location `/api/auth/saml/{provider}/acs`

If `sp_cert_file` / `sp_key_file` are not provided, AYB generates provider-scoped files in `~/.ayb/saml/` (or temp fallback if that path is not writable).

## Auth flow

### 1) Login redirect

```bash
curl -i "http://localhost:8090/api/auth/saml/okta/login?redirect_to=https://app.example.com/post-auth"
```

Behavior:

- AYB creates an AuthnRequest and stores request state for 5 minutes
- sets `ayb_saml_req_<provider>` HttpOnly cookie
- redirects to IdP SSO URL with `SAMLRequest` (+ optional `RelayState`)

### 2) ACS callback

IdP POSTs to `/api/auth/saml/{provider}/acs` with `SAMLResponse`.

AYB validates request/provider state, assertion validity window, then calls auth JIT login via:

- provider key: `saml:<provider>`
- `ProviderUserID`: assertion subject (fallback email)
- `Email`: mapped email attribute
- `Name`: mapped name attribute

Current implementation detail:

- AYB validates request binding (`request_id` + provider), request TTL, and assertion time bounds (`NotBefore`, `NotOnOrAfter`).
- AYB does not currently perform XML signature or certificate-chain verification on `SAMLResponse` assertions.

Response shape:

- JSON `{token, refreshToken, user}` when `auth.oauth_redirect_url` is empty and no MFA step is pending
- JSON `{mfa_pending, mfa_token}` when `auth.oauth_redirect_url` is empty and the login requires MFA completion
- redirect with token fragment when `auth.oauth_redirect_url` is set and no MFA step is pending
- redirect with `#mfa_pending=true&mfa_token=...` fragment data when `auth.oauth_redirect_url` is set and MFA is pending

## Admin provider management API

Routes (admin token required):

- `GET /api/admin/auth/saml`
- `POST /api/admin/auth/saml`
- `PUT /api/admin/auth/saml/{name}`
- `DELETE /api/admin/auth/saml/{name}`

Dependency behavior:

- Returns `404 auth SAML is not enabled` when auth/SAML services are not wired.
- Returns `503 database is not configured` when the database pool is unavailable.

Create request fields:

- `name`
- `entity_id`
- `idp_metadata_url` or `idp_metadata_xml`
- `attribute_mapping`

Update request fields:

- `entity_id`
- `idp_metadata_url` or `idp_metadata_xml`
- `attribute_mapping`

For updates, the provider name comes from the route path: `PUT /api/admin/auth/saml/{name}`.

Metadata URL fetches use a 10-second timeout and a 1 MiB response cap.

Create example:

```bash
curl -X POST http://localhost:8090/api/admin/auth/saml \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "okta",
    "entity_id": "https://api.example.com/api/auth/saml/okta/metadata",
    "idp_metadata_url": "https://dev-123456.okta.com/app/xyz/sso/saml/metadata",
    "attribute_mapping": {
      "email": "email",
      "name": "displayName",
      "groups": "memberOf"
    }
  }'
```

## Important current limitation

Group mappings are stored and configurable (`attribute_mapping.groups` in config/admin API), but `SAMLService.HandleCallback` currently uses mapped `email` and `name` values for JIT login and does not automatically apply group membership to org/team roles.

## Related guides

- [Authentication](/guide/authentication)
- [Organizations](/guide/organizations)
- [Admin Dashboard](/guide/admin-dashboard#auth)
