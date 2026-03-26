import type { OrgMemberRole, OrgMembership } from "../types/organizations";
import { ORG_MEMBER_ROLE_VALUES } from "../types/organizations";
import type { Tenant } from "../types/tenants";

// ---------------------------------------------------------------------------
// Org info section (editable)
// ---------------------------------------------------------------------------

interface OrgInfoSectionProps {
  orgId: string;
  slug: string;
  planTier: string;
  parentOrgId: string | null;
  createdAt: string;
  updatedAt: string;
  orgNameDraft: string;
  orgSlugDraft: string;
  orgParentIdDraft: string;
  isSaving: boolean;
  error: string | null;
  onNameChange: (value: string) => void;
  onSlugChange: (value: string) => void;
  onParentIdChange: (value: string) => void;
  onSave: () => void;
  onReset: () => void;
}

function InfoRow({ label, value }: { label: string; value: string | null }) {
  return (
    <div className="flex justify-between py-1.5 border-b border-gray-100 dark:border-gray-800 text-sm">
      <span className="text-gray-500 dark:text-gray-400">{label}</span>
      <span className="text-gray-900 dark:text-gray-100">{value ?? "—"}</span>
    </div>
  );
}

export function OrgInfoSection({
  orgId, slug, planTier, parentOrgId, createdAt, updatedAt,
  orgNameDraft, orgSlugDraft, orgParentIdDraft,
  isSaving, error,
  onNameChange, onSlugChange, onParentIdChange, onSave, onReset,
}: OrgInfoSectionProps) {
  return (
    <div data-testid="org-info-section" className="space-y-4">
      <div>
        <InfoRow label="ID" value={orgId} />
        <InfoRow label="Slug" value={slug} />
        <InfoRow label="Plan" value={planTier} />
        <InfoRow label="Parent Org" value={parentOrgId} />
        <InfoRow label="Created" value={createdAt} />
        <InfoRow label="Updated" value={updatedAt} />
      </div>
      <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Update Organization</div>
        <label className="block text-sm text-gray-600 dark:text-gray-300">
          Organization Name
          <input
            aria-label="Organization Name"
            value={orgNameDraft}
            onChange={(e) => onNameChange(e.target.value)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
          />
        </label>
        <label className="block text-sm text-gray-600 dark:text-gray-300">
          Slug
          <input
            aria-label="Organization Slug"
            value={orgSlugDraft}
            onChange={(e) => onSlugChange(e.target.value)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
          />
        </label>
        <label className="block text-sm text-gray-600 dark:text-gray-300">
          Parent Org ID
          <input
            aria-label="Parent Org ID"
            value={orgParentIdDraft}
            onChange={(e) => onParentIdChange(e.target.value)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
          />
        </label>
        {error && <div className="text-sm text-red-600">{error}</div>}
        <div className="flex justify-end gap-2">
          <button
            onClick={onReset}
            disabled={isSaving}
            className="px-3 py-1.5 text-xs rounded border border-gray-300 text-gray-700 hover:bg-gray-100 disabled:opacity-50"
          >
            Reset
          </button>
          <button
            onClick={onSave}
            disabled={isSaving}
            className="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Save Info
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Org members section (editable)
// ---------------------------------------------------------------------------

interface OrgMembersSectionProps {
  members: OrgMembership[];
  roleDraftByUserId: Partial<Record<string, OrgMemberRole>>;
  updatingUserId: string | null;
  addMemberUserId: string;
  addMemberRole: OrgMemberRole;
  isAddingMember: boolean;
  removingUserId: string | null;
  actionError: string | null;
  onRoleDraftChange: (userId: string, nextRole: OrgMemberRole) => void;
  onUpdateRole: (userId: string) => void;
  onAddMemberUserIdChange: (value: string) => void;
  onAddMemberRoleChange: (value: OrgMemberRole) => void;
  onAddMember: () => void;
  onRemoveMember: (userId: string) => void;
}

export function OrgMembersSection({
  members, roleDraftByUserId, updatingUserId,
  addMemberUserId, addMemberRole, isAddingMember, removingUserId, actionError,
  onRoleDraftChange, onUpdateRole,
  onAddMemberUserIdChange, onAddMemberRoleChange, onAddMember, onRemoveMember,
}: OrgMembersSectionProps) {
  return (
    <div data-testid="org-members-section" className="space-y-3">
      <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Add Member</div>
        <div className="flex flex-col gap-2 md:flex-row md:items-end">
          <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
            User ID
            <input
              aria-label="New Member User ID"
              value={addMemberUserId}
              onChange={(e) => onAddMemberUserIdChange(e.target.value)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            />
          </label>
          <label className="text-sm text-gray-600 dark:text-gray-300">
            Role
            <select
              aria-label="New Member Role"
              value={addMemberRole}
              onChange={(e) => onAddMemberRoleChange(e.target.value as OrgMemberRole)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            >
              {ORG_MEMBER_ROLE_VALUES.map((role) => (
                <option key={role} value={role}>{role}</option>
              ))}
            </select>
          </label>
          <button
            onClick={onAddMember}
            disabled={isAddingMember}
            className="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Add Member
          </button>
        </div>
        {actionError && <div className="text-sm text-red-600">{actionError}</div>}
      </div>
      {members.length === 0 ? (
        <div className="text-sm text-gray-400 py-2">No members found</div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-xs text-gray-500">
              <th className="py-2 pr-4">User ID</th>
              <th className="py-2 pr-4">Role</th>
              <th className="py-2 pr-4">Update Role</th>
              <th className="py-2 pr-4">Joined</th>
              <th className="py-2">Remove</th>
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.id} data-testid={`org-member-row-${m.userId}`} className="border-b border-gray-100 dark:border-gray-800">
                <td className="py-1.5 pr-4 text-gray-900 dark:text-gray-100">{m.userId}</td>
                <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">
                  <span data-testid={`org-member-role-${m.userId}`}>{m.role}</span>
                </td>
                <td className="py-1.5 pr-4">
                  <div className="flex items-center gap-2">
                    <select
                      aria-label={`Role for ${m.userId}`}
                      value={roleDraftByUserId[m.userId] ?? m.role}
                      onChange={(e) => onRoleDraftChange(m.userId, e.target.value as OrgMemberRole)}
                      className="border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
                    >
                      {ORG_MEMBER_ROLE_VALUES.map((role) => (
                        <option key={role} value={role}>{role}</option>
                      ))}
                    </select>
                    <button
                      onClick={() => onUpdateRole(m.userId)}
                      disabled={updatingUserId === m.userId}
                      className="px-2 py-1 text-xs rounded border border-gray-300 text-gray-700 hover:bg-gray-100 disabled:opacity-50"
                    >
                      Update Role
                    </button>
                  </div>
                </td>
                <td className="py-1.5 pr-4 text-gray-500 dark:text-gray-400">{m.createdAt}</td>
                <td className="py-1.5">
                  <button
                    onClick={() => onRemoveMember(m.userId)}
                    disabled={removingUserId === m.userId}
                    className="px-2 py-1 text-xs rounded border border-red-300 text-red-700 hover:bg-red-50 disabled:opacity-50"
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tenant assignment section
// ---------------------------------------------------------------------------

interface OrgTenantsSectionProps {
  tenants: Tenant[];
  assignTenantId: string;
  isAssigning: boolean;
  actionError: string | null;
  onTenantIdChange: (value: string) => void;
  onAssign: () => void;
  onUnassign: (tenantId: string) => void;
}

export function OrgTenantsSection({
  tenants, assignTenantId, isAssigning, actionError,
  onTenantIdChange, onAssign, onUnassign,
}: OrgTenantsSectionProps) {
  return (
    <div data-testid="org-tenants-section" className="space-y-3">
      <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Assign Tenant</div>
        <div className="flex flex-col gap-2 md:flex-row md:items-end">
          <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
            Tenant ID
            <input
              aria-label="Tenant ID"
              value={assignTenantId}
              onChange={(e) => onTenantIdChange(e.target.value)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            />
          </label>
          <button
            onClick={onAssign}
            disabled={isAssigning}
            className="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Assign Tenant
          </button>
        </div>
        {actionError && <div className="text-sm text-red-600">{actionError}</div>}
      </div>
      {tenants.length === 0 ? (
        <div className="text-sm text-gray-400 py-2">No tenants assigned</div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-xs text-gray-500">
              <th className="py-2 pr-4">Name</th>
              <th className="py-2 pr-4">Slug</th>
              <th className="py-2 pr-4">State</th>
              <th className="py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {tenants.map((t) => (
              <tr key={t.id} className="border-b border-gray-100 dark:border-gray-800">
                <td className="py-1.5 pr-4 text-gray-900 dark:text-gray-100">{t.name}</td>
                <td className="py-1.5 pr-4 text-gray-500 dark:text-gray-400">{t.slug}</td>
                <td className="py-1.5 pr-4 text-gray-500 dark:text-gray-400">{t.state}</td>
                <td className="py-1.5">
                  <button
                    onClick={() => onUnassign(t.id)}
                    className="px-2 py-1 text-xs rounded border border-red-300 text-red-700 hover:bg-red-50"
                  >
                    Unassign
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
