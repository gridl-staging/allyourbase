import type { MemberRole, Tenant, TenantMembership } from "../types/tenants";
import { MEMBER_ROLE_VALUES } from "../types/tenants";

interface TenantInfoSectionProps {
  tenant: Tenant;
  tenantNameDraft: string;
  tenantOrgMetadataDraft: string;
  isSaving: boolean;
  error: string | null;
  onTenantNameChange: (value: string) => void;
  onTenantOrgMetadataChange: (value: string) => void;
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

export function TenantInfoSection({
  tenant,
  tenantNameDraft,
  tenantOrgMetadataDraft,
  isSaving,
  error,
  onTenantNameChange,
  onTenantOrgMetadataChange,
  onSave,
  onReset,
}: TenantInfoSectionProps) {
  return (
    <div data-testid="tenant-info-section" className="space-y-4">
      <div>
        <InfoRow label="ID" value={tenant.id} />
        <InfoRow label="Slug" value={tenant.slug} />
        <InfoRow label="Isolation" value={tenant.isolationMode} />
        <InfoRow label="Plan" value={tenant.planTier} />
        <InfoRow label="Region" value={tenant.region} />
        <InfoRow label="State" value={tenant.state} />
        <InfoRow label="Created" value={tenant.createdAt} />
        <InfoRow label="Updated" value={tenant.updatedAt} />
      </div>
      <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Update Tenant</div>
        <label className="block text-sm text-gray-600 dark:text-gray-300">
          Tenant Name
          <input
            aria-label="Tenant Name"
            value={tenantNameDraft}
            onChange={(event) => onTenantNameChange(event.target.value)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
          />
        </label>
        <label className="block text-sm text-gray-600 dark:text-gray-300">
          Org Metadata
          <textarea
            aria-label="Org Metadata"
            rows={4}
            value={tenantOrgMetadataDraft}
            onChange={(event) => onTenantOrgMetadataChange(event.target.value)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm font-mono bg-white dark:bg-gray-900"
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

interface TenantMembersSectionProps {
  members: TenantMembership[];
  roleDraftByUserId: Partial<Record<string, MemberRole>>;
  updatingUserId: string | null;
  addMemberUserId: string;
  addMemberRole: MemberRole;
  isAddingMember: boolean;
  removingUserId: string | null;
  actionError: string | null;
  onRoleDraftChange: (userId: string, nextRole: MemberRole) => void;
  onUpdateRole: (userId: string) => void;
  onAddMemberUserIdChange: (value: string) => void;
  onAddMemberRoleChange: (value: MemberRole) => void;
  onAddMember: () => void;
  onRemoveMember: (userId: string) => void;
}

export function TenantMembersSection({
  members,
  roleDraftByUserId,
  updatingUserId,
  addMemberUserId,
  addMemberRole,
  isAddingMember,
  removingUserId,
  actionError,
  onRoleDraftChange,
  onUpdateRole,
  onAddMemberUserIdChange,
  onAddMemberRoleChange,
  onAddMember,
  onRemoveMember,
}: TenantMembersSectionProps) {
  return (
    <div data-testid="tenant-members-section" className="space-y-3">
      <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Add Member</div>
        <div className="flex flex-col gap-2 md:flex-row md:items-end">
          <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
            User ID
            <input
              aria-label="New Member User ID"
              value={addMemberUserId}
              onChange={(event) => onAddMemberUserIdChange(event.target.value)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            />
          </label>
          <label className="text-sm text-gray-600 dark:text-gray-300">
            Role
            <select
              aria-label="New Member Role"
              value={addMemberRole}
              onChange={(event) => onAddMemberRoleChange(event.target.value as MemberRole)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            >
              {MEMBER_ROLE_VALUES.map((role) => (
                <option key={role} value={role}>
                  {role}
                </option>
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
              <tr
                key={m.id}
                data-testid={`tenant-member-row-${m.userId}`}
                className="border-b border-gray-100 dark:border-gray-800"
              >
                <td className="py-1.5 pr-4 text-gray-900 dark:text-gray-100">{m.userId}</td>
                <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">
                  <span data-testid={`tenant-member-role-${m.userId}`}>{m.role}</span>
                </td>
                <td className="py-1.5 pr-4">
                  <div className="flex items-center gap-2">
                    <label htmlFor={`member-role-${m.userId}`} className="sr-only">
                      Role for {m.userId}
                    </label>
                    <select
                      id={`member-role-${m.userId}`}
                      aria-label={`Role for ${m.userId}`}
                      value={roleDraftByUserId[m.userId] ?? m.role}
                      onChange={(event) => onRoleDraftChange(m.userId, event.target.value as MemberRole)}
                      className="border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
                    >
                      {MEMBER_ROLE_VALUES.map((role) => (
                        <option key={role} value={role}>
                          {role}
                        </option>
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
