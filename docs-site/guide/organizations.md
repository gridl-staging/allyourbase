<!-- audited 2026-03-21 -->
# Organizations

AYB supports hierarchical multi-tenancy with organizations, teams, members, and tenant assignments. Organization APIs are admin-token protected under `/api/admin/orgs`.

## Use cases

Use organizations when one AYB deployment serves multiple business units or customers:

- B2B SaaS: one org per customer, teams per department, tenants per environment (`prod`, `staging`, `sandbox`).
- Enterprise internal platform: one org per division, shared users across teams, central tenant ownership for billing and audit.
- Agency model: one org per client account, multiple project teams, and explicit tenant assignment/unassignment during onboarding/offboarding.

## Hierarchy and model

`internal/tenant/org.go` defines the core entities:

- **Organization**: top-level entity with `id`, `name`, `slug`, optional `parentOrgId`, and `planTier`.
- **Team**: child entity inside an org with `id`, `orgId`, `name`, and `slug`.
- **OrgMembership**: user membership in an org (`orgId`, `userId`, `role`).
- **TeamMembership**: user membership in a team (`teamId`, `userId`, `role`).

## Roles and permissions

AYB defines four org roles and two team roles as constants in `internal/tenant/org.go`.

### Organization roles

- `owner`
- `admin`
- `member`
- `viewer`

### Team roles

- `lead`
- `member`

### Effective permission mapping

`internal/tenant/permissions.go` maps org/team roles to effective tenant permission levels:

- Org `owner` and `admin` map to tenant-level admin capability.
- Org `member` and `viewer` map to tenant-level viewer capability.
- Team `lead` maps to tenant-level member capability.
- Team `member` maps to tenant-level viewer capability.

## API inventory

The following endpoints are registered in `internal/server/routes_admin_orgs.go`.

### Organizations

- `POST /api/admin/orgs`
- `GET /api/admin/orgs`
- `GET /api/admin/orgs/{orgId}`
- `PUT /api/admin/orgs/{orgId}`
- `DELETE /api/admin/orgs/{orgId}`
- `GET /api/admin/orgs/{orgId}/usage`
- `GET /api/admin/orgs/{orgId}/audit`

### Teams

- `POST /api/admin/orgs/{orgId}/teams`
- `GET /api/admin/orgs/{orgId}/teams`
- `GET /api/admin/orgs/{orgId}/teams/{teamId}`
- `PUT /api/admin/orgs/{orgId}/teams/{teamId}`
- `DELETE /api/admin/orgs/{orgId}/teams/{teamId}`

### Team members

- `POST /api/admin/orgs/{orgId}/teams/{teamId}/members`
- `GET /api/admin/orgs/{orgId}/teams/{teamId}/members`
- `PUT /api/admin/orgs/{orgId}/teams/{teamId}/members/{userId}/role`
- `DELETE /api/admin/orgs/{orgId}/teams/{teamId}/members/{userId}`

### Organization members

- `POST /api/admin/orgs/{orgId}/members`
- `GET /api/admin/orgs/{orgId}/members`
- `PUT /api/admin/orgs/{orgId}/members/{userId}/role`
- `DELETE /api/admin/orgs/{orgId}/members/{userId}`

### Tenant assignment

- `POST /api/admin/orgs/{orgId}/tenants`
- `GET /api/admin/orgs/{orgId}/tenants`
- `DELETE /api/admin/orgs/{orgId}/tenants/{tenantId}`

## Safety rules enforced by handlers

These constraints are enforced in `org_handler.go`, `org_membership_handler.go`, and `team_membership_handler.go`:

- Slug validation for organizations/teams (`tenant.IsValidSlug`).
- Parent org validation before create/update.
- Circular parent-org protection (`ErrCircularParentOrg`).
- Last-owner protection when removing or demoting org owners (`ErrLastOwner`).
- Team membership requires existing org membership first.
- Deleting an org requires `?confirm=true` and no assigned tenants.

## SDK and curl examples

### JavaScript admin workflow (fetch + admin token)

`@allyourbase/js` currently focuses on auth/data APIs and does not yet expose typed org-admin helpers. Organization admin routes are protected by the separate admin-token flow, so resolve an admin token from `/api/admin/auth`, then call org endpoints with `fetch`.

```ts
const baseURL = "http://localhost:8090";
const adminPassword = process.env.AYB_ADMIN_PASSWORD ?? "<admin-password>";

const adminAuth = await fetch(`${baseURL}/api/admin/auth`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ password: adminPassword }),
});

if (!adminAuth.ok) {
  throw new Error(`Admin auth failed: ${adminAuth.status}`);
}

const { token: adminToken } = (await adminAuth.json()) as { token: string };

async function adminRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${baseURL}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${adminToken}`,
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });

  if (!response.ok) {
    throw new Error(`Admin request failed: ${response.status}`);
  }

  return response.status === 204 ? (undefined as T) : ((await response.json()) as T);
}

const org = await adminRequest<{ id: string }>("/api/admin/orgs", {
  method: "POST",
  body: JSON.stringify({ name: "Acme", slug: "acme", planTier: "pro" }),
});

await adminRequest("/api/admin/orgs/" + org.id + "/members", {
  method: "POST",
  body: JSON.stringify({ userId: "00000000-0000-0000-0000-000000000001", role: "admin" }),
});
```

### curl: create org, add member, assign tenant

```bash
# Create org
curl -X POST http://localhost:8090/api/admin/orgs \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme","slug":"acme","planTier":"pro"}'

# Add org member
curl -X POST http://localhost:8090/api/admin/orgs/<orgId>/members \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"userId":"<user-uuid>","role":"owner"}'

# Assign tenant to org
curl -X POST http://localhost:8090/api/admin/orgs/<orgId>/tenants \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"tenantId":"<tenant-uuid>"}'
```

## Dashboard

The Admin dashboard includes dedicated `organizations` and `tenants` views under the **Admin** sidebar section (see `ui/src/components/layout-types.ts`, `Sidebar.tsx`, and `ContentRouter.tsx`), in addition to API Explorer and direct admin API access.

## Related guides

- [Authentication](/guide/authentication)
- [SAML SSO](/guide/saml)
- [Security](/guide/security)
- [Admin Dashboard](/guide/admin-dashboard)
