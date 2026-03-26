<!-- audited 2026-03-21 -->
# Admin Dashboard

AYB's admin dashboard is a built-in web UI for managing your project's database, services, users, and infrastructure. It combines a dynamic table browser with fixed admin views.

## Access

The dashboard is available at `http://localhost:8090/admin` by default.

```toml
[admin]
enabled = true
path = "/admin"
password = "your-admin-password"
login_rate_limit = 20       # admin login attempts per minute per IP
allowed_ips = []             # empty = allow all; set CIDRs to restrict admin access
```

Configure `admin.password` before relying on the dashboard. When `admin.password` is unset, admin login is not configured and admin-authenticated endpoints fail closed.

::: warning
Never expose the admin dashboard with a weak or reused password on a public network.
:::

For production deployments, set an admin password explicitly:

```bash
AYB_ADMIN_PASSWORD=your-secure-password ayb start
```

If you rely on AYB's generated password, reset it with:

```bash
ayb admin reset-password
```

If `admin.password` or `AYB_ADMIN_PASSWORD` is explicitly set, change that value and restart instead of using `reset-password`.

## Navigating the Dashboard

The sidebar organizes views into six actual sections: **Database**, **Services**, **Messaging**, **Admin**, **AI**, and **Auth**. Use the **Command Palette** (click the search hint at the top of the sidebar or press <kbd>Cmd/Ctrl+K</kbd>) to jump to any view by name.

### Table browser

The **Tables** section at the top of the sidebar lists your project's database tables, generated from the schema at runtime. Click **+ New Table** to open the SQL Editor and create one. When you select a table, the content area shows three tabs:

- **Data** — browse and edit rows
- **Schema** — inspect columns and constraints
- **SQL** — run queries scoped to the selected table

### Theme

Toggle between light and dark mode using the theme button at the bottom of the sidebar.

---

## Actual Sidebar Sections

These are the sidebar sections and view names implemented in `Sidebar.tsx` and routed in `ContentRouter.tsx`.

### Database

- **SQL Editor** — run ad hoc SQL for schema changes, data queries, and migrations.
- **Functions** — browse schema-discovered database functions.
- **RLS Policies** — manage row-level security policies.
- **Matviews** — register and refresh materialized views.
- **Schema Designer** — inspect and model schema relationships visually.
- **FDW** — manage foreign data wrapper servers and foreign tables.

### Services

- **Storage** — manage object storage.
- **Sites** — manage hosted static sites.
- **Edge Functions** — build and deploy edge functions.
- **Webhooks** — manage outbound webhook delivery.

### Messaging

- **SMS Health** — inspect SMS provider health.
- **SMS Messages** — inspect sent and queued SMS records.
- **Email Templates** — edit and preview email templates.
- **Push Notifications** — manage push devices and deliveries.

### Admin

- **Users** — manage user accounts and related state.
- **Apps** — manage app records and app-scoped rate limits.
- **API Keys** — create, rotate, and revoke API keys.
- **OAuth Clients** — manage OAuth provider-mode clients.
- **API Explorer** — interactively test AYB endpoints.
- **Jobs** — inspect background job execution.
- **Schedules** — manage recurring schedules.
- **Realtime Inspector** — inspect active realtime connections.
- **Security Advisor** — review security findings.
- **Performance Advisor** — review performance findings.
- **Backups** — manage backups and PITR flows.
- **Analytics** — inspect analytics views.
- **Usage** — inspect usage and metering views.
- **Replicas** — manage read replicas.
- **Branches** — manage database branches.
- **Audit Logs** — inspect administrative audit events.
- **Admin Logs** — inspect operational admin logs.
- **Secrets** — manage encrypted vault-backed secrets.
- **Custom Domains** — manage custom domain mappings.
- **Extensions** — enable and inspect PostgreSQL extensions.
- **Vector Indexes** — create and inspect vector indexes.
- **Log Drains** — manage external log sinks.
- **Stats** — inspect service and platform stats.
- **Notifications** — manage in-app notifications.
- **Incidents** — manage incidents.
- **Support Tickets** — manage support ticket workflows.
- **Tenants** — manage tenant lifecycle and membership.
- **Organizations** — manage organizations, teams, members, usage, and tenant assignments.

### AI

- **AI Assistant** — use the dashboard AI assistant. Vector indexes are not in the AI sidebar section; they live under **Admin**.

### Auth

- **Auth Settings** — configure auth providers and flows.
- **MFA Management** — manage MFA enrollment and assurance state.
- **Account Linking** — manage linked identities.
- **SAML** — configure SAML/SSO.
- **Auth Hooks** — manage auth hook extensions.

## Workflow Crosswalk

The rest of this guide uses workflow-oriented groupings. These cut across the actual sidebar sections above.

### Schema & Data Modeling

- **SQL Editor**, **Schema Designer**, **Functions**, **RLS Policies**, **Matviews**, **FDW**, and **API Explorer**.

### Identity & Access

- **Users**, **Auth Settings**, **MFA Management**, **Account Linking**, **SAML**, **Auth Hooks**, **API Keys**, and **OAuth Clients**.

### App Services & Delivery

- **Storage**, **Sites**, **Edge Functions**, **Webhooks**, **Custom Domains**, **Extensions**, **AI Assistant**, and **Vector Indexes**.

### Messaging & Engagement

- **SMS Health**, **SMS Messages**, **Email Templates**, **Push Notifications**, and **Notifications**.

### Operations & Observability

- **Jobs**, **Schedules**, **Realtime Inspector**, **Security Advisor**, **Performance Advisor**, **Backups**, **Analytics**, **Usage**, **Replicas**, **Branches**, **Audit Logs**, **Admin Logs**, **Log Drains**, **Stats**, **Incidents**, and **Support Tickets**.

### Platform Governance

- **Apps**, **Tenants**, and **Organizations**.

### Apps management

Use **Admin -> Apps** to create and maintain application records that represent distinct clients (for example: web app, iOS app, and partner integration). App records are used by app-scoped API keys and per-app request controls.

### API key app scoping

When creating keys in **Admin -> API Keys**, set an app scope so a key can only access data and operations for its assigned app. This keeps tenant-wide keys from being reused across unrelated client surfaces and makes key rotation safer by app boundary.

### Per-app rate limits

Use **Admin -> Apps** to configure per-app rate limits so noisy or abusive traffic from one app is constrained without impacting other apps in the same project. Requests that exceed the configured cap return `429 Too Many Requests`.

MCP server integration is documented separately; it is not a dashboard sidebar view. See [MCP Server](/guide/mcp).
