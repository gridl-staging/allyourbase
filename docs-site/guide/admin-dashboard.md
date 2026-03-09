# Admin Dashboard

AYB includes a built-in admin dashboard for browsing tables, managing records, and inspecting your database schema.

## Access

The dashboard is available at `http://localhost:8090/admin` by default.

## Configuration

```toml
[admin]
enabled = true
path = "/admin"
password = "your-admin-password"
```

When `password` is set, the dashboard requires authentication. Without a password, the dashboard is open (suitable for local development only).

## Features

### Table browser

- Sidebar listing all tables in your database
- Paginated data table with sorting
- Click any row to view full record details

### Record management

- **Create** new records with a form auto-generated from the table schema
- **Edit** existing records inline
- **Delete** records with confirmation

### Schema viewer

- View columns, data types, and constraints for each table
- See primary keys, foreign key relationships, and indexes

### Apps management

- View all registered apps with owner and configured rate limits
- Create new apps (name, description, owner user)
- Delete apps that are no longer needed

### API key app scoping

- Create API keys as legacy user-scoped keys or scoped to a specific app
- See each key's app association directly in the API keys table
- View app-specific rate limit details in the key list and create-key confirmation modal

### Per-app rate limits

- Configure per-app request limits in the Apps UI (`rateLimitRps`, `rateLimitWindowSeconds`)
- Enforce request ceilings for app-scoped keys in API middleware
- Surface app rate-limit values in admin UI for quick audit and troubleshooting

### OAuth client management

When OAuth provider mode is enabled, the dashboard includes an OAuth Clients section:

- View all registered OAuth clients with client ID, name, linked app, type (confidential/public), and active/revoked status
- Register new OAuth clients linked to an existing app, with redirect URIs, scope selection, and client type
- View token stats per client: active access tokens, active refresh tokens, total grants, last token issued
- Revoke clients (soft-delete) and rotate client secrets for confidential clients
- Client secret is displayed once on creation and rotation — it cannot be retrieved later

### Email templates management

The dashboard includes an Email Templates section under Messaging:

- Table view shows system and custom template keys with source badge (`builtin`/`custom`), enabled state, and update timestamp
- System keys are always present (`auth.password_reset`, `auth.email_verification`, `auth.magic_link`) even when no custom override exists
- Selecting a row opens editors for:
  - subject template (`text/template`)
  - HTML template (`html/template`)
- Live preview panel renders subject/HTML/text using JSON variables with debounced preview requests
- Enable/disable toggle controls whether a custom override is active
- `Reset to Default` removes a system-key override and returns to built-in content
- `Delete Template` removes custom app keys
- `Send Test Email` renders current content and sends to a provided recipient address

Validation and safety behavior surfaced in the UI:

- Missing variables or template syntax errors are displayed immediately in preview
- Invalid JSON in preview variables is rejected client-side before any preview request is sent
- Missing or broken custom auth templates safely fall back to built-in defaults during actual auth email sends

### Jobs queue management

When `jobs.enabled = true`, the dashboard includes a Jobs section:

- Queue stats summary: queued/running/completed/failed/canceled and oldest queued age
- Job table with state badge, type, created time, attempts, and last error preview
- Filters by state and job type
- Row actions:
  - Retry failed jobs
  - Cancel queued jobs

When jobs are disabled, jobs endpoints return `503` and the view cannot load queue data.

### Schedules management

When `jobs.enabled = true`, the dashboard includes a Schedules section:

- List schedules with name, job type, cron, timezone, enabled status, last run, next run
- Enable/disable toggle per schedule
- Create/edit modal with validation:
  - cron expression format validation
  - payload JSON validation
- Delete confirmation flow

### Materialized views management

The dashboard includes a Materialized Views section for managing registered views and their refresh lifecycle:

- Table listing registered matviews: schema, view name, refresh mode, last refresh time/status/duration, error preview
- **Refresh now** button per row — triggers an immediate synchronous refresh with duration feedback
- **Register** modal — dropdown of discovered materialized views from the schema cache, refresh mode selection (standard or concurrent)
- **Edit** mode — update refresh mode for existing registrations
- **Unregister** — remove a matview registration (does not drop the view from the database)

Refresh status indicators:
- Green badge for successful last refresh
- Red badge with error preview for failed refreshes
- Advisory lock conflicts show "refresh already in progress"

## Security

For production deployments, always set an admin password:

```bash
AYB_ADMIN_PASSWORD=your-secure-password ayb start
```

Or reset the auto-generated admin password:

```bash
ayb admin reset-password
```

::: warning
Never expose the admin dashboard without a password on a public network.
:::
