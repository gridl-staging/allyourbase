# Email Templates

AYB supports configurable email templates for built-in auth emails and arbitrary app-specific email flows (for example, club invites and event reminders).

## Template model

AYB uses two template sources:

| Source | Stored in | Keys | Notes |
|---|---|---|---|
| Built-in defaults | Go binary (`//go:embed`) | `auth.password_reset`, `auth.email_verification`, `auth.magic_link` | Always available fallback templates for auth flows |
| Custom overrides | `_ayb_email_templates` table | Any valid dot key (for example `app.club_invite`) | Optional overrides for system keys and custom app templates |

Custom template keys must match:

```text
^[a-z][a-z0-9]*(\.[a-z][a-z0-9_]*)+$
```

Examples:

- valid: `auth.password_reset`, `app.club_invite`, `app.weekly_digest`
- invalid: `Auth.reset`, `app`, `app..invite`, `app.club-invite`

Template size limits:

- `subjectTemplate` max 1000 characters
- `htmlTemplate` max 256000 characters (256 KB)

## Variable substitution

Templates use Go template syntax:

```text
{{.VarName}}
```

Data is always a flat `map[string]string`:

- no nested objects
- no `map[string]any`
- no structs/interfaces with methods

System auth templates use fixed variables:

- `AppName`
- `ActionURL`

Custom app templates accept arbitrary string variables.

Example:

```text
Subject: Welcome to {{.AppName}}
Body:    <p>Click <a href="{{.ActionURL}}">here</a> to continue.</p>
```

Missing variables are hard errors (`missingkey=error`) and never silently render as empty strings.

## Safety model

AYB applies the following controls:

- HTML body templates use `html/template` (auto-escaping for HTML contexts)
- Subject templates use `text/template`
- Empty template function map (no custom template functions)
- Execution data is `map[string]string` only (reduces SSTI surface)
- Render timeout of 5 seconds to prevent pathological template execution from blocking the pipeline
- Template parse/compile validation on save and preview

## Rendering and fallback behavior

`Service.Render(key, vars)` resolves in this order:

1. enabled custom override (if present)
2. built-in default (for supported `auth.*` keys)
3. deterministic error (`ErrNoTemplate`) if neither exists

Auth email sends use graceful degradation: if a custom `auth.*` template fails at render time, AYB logs the error and falls back to the built-in template so password reset/verification/magic-link flows keep working.

Plaintext email content is auto-generated from rendered HTML (`stripHTML()`), so there is no separate text-template editor.

## Admin API

All endpoints require admin auth (`Authorization: Bearer <admin-token>`).

```text
GET    /api/admin/email/templates
GET    /api/admin/email/templates/{key}
PUT    /api/admin/email/templates/{key}
PATCH  /api/admin/email/templates/{key}
DELETE /api/admin/email/templates/{key}
POST   /api/admin/email/templates/{key}/preview
POST   /api/admin/email/send
```

### Upsert a custom template override

```bash
curl -X PUT http://localhost:8090/api/admin/email/templates/auth.password_reset \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subjectTemplate": "Reset your {{.AppName}} password",
    "htmlTemplate": "<p>Reset here: <a href=\"{{.ActionURL}}\">{{.ActionURL}}</a></p>"
  }'
```

### Preview a template without saving

```bash
curl -X POST http://localhost:8090/api/admin/email/templates/app.club_invite/preview \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subjectTemplate": "You are invited to {{.ClubName}}",
    "htmlTemplate": "<p>{{.InviterName}} invited you to {{.ClubName}}</p>",
    "variables": {
      "ClubName": "Sunrise Runners",
      "InviterName": "Maya"
    }
  }'
```

### Send an email using a template key

```bash
curl -X POST http://localhost:8090/api/admin/email/send \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "templateKey": "app.club_invite",
    "to": "user@example.com",
    "variables": {
      "ClubName": "Sunrise Runners",
      "InviterName": "Maya",
      "InviteURL": "https://sigil.example/invites/abc123"
    }
  }'
```

## CLI

Use the `ayb email-templates` command group:

```bash
ayb email-templates list
ayb email-templates get auth.password_reset
ayb email-templates set auth.password_reset --subject 'Reset {{.AppName}} password' --html-file ./password_reset.html
ayb email-templates preview auth.password_reset --vars '{"AppName":"Sigil","ActionURL":"https://example/reset"}'
ayb email-templates disable auth.password_reset
ayb email-templates enable auth.password_reset
ayb email-templates delete app.club_invite
ayb email-templates send app.club_invite --to user@example.com --vars '{"ClubName":"Sunrise Runners"}'
```

`preview` behavior:

- If `--subject` and `--html-file` are provided, preview uses those unsaved values.
- If either is omitted, preview loads the current effective template first, then renders with `--vars`.

## Admin dashboard workflow

In the Admin Dashboard, open `Messaging -> Email Templates` to:

- browse built-in and custom keys
- customize/override system templates
- edit subject + HTML templates
- run live preview with JSON variables
- enable/disable custom overrides
- reset system keys to default (delete override)
- send a test email to a target address

## Troubleshooting

| Symptom | Typical cause | Response |
|---|---|---|
| `400 template parse error` | Invalid Go template syntax | Fix template syntax and save again |
| `400 template render error` | Missing variable or render failure | Provide all required variables in preview/send |
| `404 template not found` | App key has no custom template and no built-in fallback | Create a custom template for that key |
| Auth emails still use old content | Custom override disabled or deleted | Enable override or re-save template |
