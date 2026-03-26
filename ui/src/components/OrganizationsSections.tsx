import type { OrgAuditEvent, OrgDetailResponse, OrgDetailTab, OrgUsageSummary, Organization } from "../types/organizations";
import { ORG_DETAIL_TAB_VALUES } from "../types/organizations";
import { FilterBar, type FilterField } from "./shared/FilterBar";

// ---------------------------------------------------------------------------
// Org list panel (left side)
// ---------------------------------------------------------------------------

interface OrgListPanelProps {
  items: Organization[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  newOrgName: string;
  newOrgSlug: string;
  newOrgPlanTier: string;
  newOrgParentId: string;
  isCreatingOrg: boolean;
  createOrgError: string | null;
  onNewOrgNameChange: (value: string) => void;
  onNewOrgSlugChange: (value: string) => void;
  onNewOrgPlanTierChange: (value: string) => void;
  onNewOrgParentIdChange: (value: string) => void;
  onCreateOrg: () => void;
}

const TIER_BADGE_COLORS: Record<string, string> = {
  free: "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400",
  pro: "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300",
  enterprise: "bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300",
};

export function OrgListPanel({
  items,
  selectedId,
  onSelect,
  newOrgName,
  newOrgSlug,
  newOrgPlanTier,
  newOrgParentId,
  isCreatingOrg,
  createOrgError,
  onNewOrgNameChange,
  onNewOrgSlugChange,
  onNewOrgPlanTierChange,
  onNewOrgParentIdChange,
  onCreateOrg,
}: OrgListPanelProps) {
  return (
    <div
      data-testid="org-list-panel"
      className="w-72 flex flex-col border-r border-gray-200 dark:border-gray-700 overflow-hidden"
    >
      <div className="px-3 py-2 border-b border-gray-200 dark:border-gray-700">
        <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Organizations</h2>
        <div className="mt-2 space-y-2">
          <input
            aria-label="Create Org Name"
            value={newOrgName}
            onChange={(event) => onNewOrgNameChange(event.target.value)}
            placeholder="Org name"
            className="w-full border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
          />
          <input
            aria-label="Create Org Slug"
            value={newOrgSlug}
            onChange={(event) => onNewOrgSlugChange(event.target.value)}
            placeholder="org-slug"
            className="w-full border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
          />
          <select
            aria-label="Create Org Plan Tier"
            value={newOrgPlanTier}
            onChange={(event) => onNewOrgPlanTierChange(event.target.value)}
            className="w-full border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
          >
            <option value="free">free</option>
            <option value="pro">pro</option>
            <option value="enterprise">enterprise</option>
          </select>
          <input
            aria-label="Create Org Parent ID"
            value={newOrgParentId}
            onChange={(event) => onNewOrgParentIdChange(event.target.value)}
            placeholder="Parent org ID (optional)"
            className="w-full border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
          />
          <button
            onClick={onCreateOrg}
            disabled={isCreatingOrg}
            className="w-full px-2 py-1 text-xs font-medium rounded bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900 dark:text-blue-300 disabled:opacity-60"
          >
            Create Org
          </button>
          {createOrgError && <div className="text-xs text-red-600">{createOrgError}</div>}
        </div>
      </div>
      <div className="flex-1 overflow-auto">
        {items.map((org) => (
          <button
            key={org.id}
            onClick={() => onSelect(org.id)}
            className={`w-full text-left px-3 py-2 text-sm border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-900 ${
              selectedId === org.id ? "bg-blue-50 dark:bg-blue-950" : ""
            }`}
          >
            <div className="font-medium text-gray-900 dark:text-gray-100">{org.name}</div>
            <div className="flex items-center gap-2 mt-0.5">
              <span className="text-xs text-gray-500 dark:text-gray-400">{org.slug}</span>
              <span className={`text-xs px-1.5 py-0.5 rounded ${TIER_BADGE_COLORS[org.planTier] ?? "bg-gray-100 text-gray-500"}`}>
                {org.planTier}
              </span>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Detail header with tabs + action buttons
// ---------------------------------------------------------------------------

interface OrgDetailHeaderProps {
  org: OrgDetailResponse;
  activeTab: OrgDetailTab;
  onTabChange: (tab: OrgDetailTab) => void;
  onDelete: () => void;
  isDeleting: boolean;
}

const TAB_LABELS: Record<OrgDetailTab, string> = {
  info: "Info",
  members: "Members",
  teams: "Teams",
  tenants: "Tenants",
  usage: "Usage",
  audit: "Audit",
};

export function OrgDetailHeader({ org, activeTab, onTabChange, onDelete, isDeleting }: OrgDetailHeaderProps) {
  return (
    <div className="mb-4">
      <div className="flex items-center justify-between mb-3">
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{org.name}</h2>
          <div className="flex gap-3 text-xs text-gray-500 dark:text-gray-400 mt-1">
            <span>{org.childOrgCount} child org{org.childOrgCount !== 1 ? "s" : ""}</span>
            <span>{org.teamCount} team{org.teamCount !== 1 ? "s" : ""}</span>
            <span>{org.tenantCount} tenant{org.tenantCount !== 1 ? "s" : ""}</span>
          </div>
        </div>
        <div className="flex gap-2">
          <button
            onClick={onDelete}
            disabled={isDeleting}
            className="px-3 py-1 text-xs rounded bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900 dark:text-red-300"
          >
            {isDeleting ? "Deleting…" : "Delete"}
          </button>
        </div>
      </div>
      <div className="flex gap-1 bg-gray-100 dark:bg-gray-800 rounded p-0.5">
        {ORG_DETAIL_TAB_VALUES.map((tab) => (
          <button
            key={tab}
            onClick={() => onTabChange(tab)}
            className={`px-3 py-1 text-xs rounded font-medium transition-colors ${
              activeTab === tab
                ? "bg-white dark:bg-gray-900 shadow-sm text-gray-900 dark:text-gray-100"
                : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
            }`}
          >
            {TAB_LABELS[tab]}
          </button>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Usage section (read-only)
// ---------------------------------------------------------------------------

interface OrgUsageSectionProps {
  summary: OrgUsageSummary | null;
  filterValues: Record<string, string>;
  onFilterChange: (name: string, value: string) => void;
  onApplyFilters: (values: Record<string, string>) => void;
  onResetFilters: () => void;
}

const USAGE_FILTER_FIELDS: FilterField[] = [
  {
    name: "period",
    label: "Period",
    type: "select",
    options: [
      { value: "day", label: "day" },
      { value: "week", label: "week" },
      { value: "month", label: "month" },
    ],
  },
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
];

export function OrgUsageSection({
  summary,
  filterValues,
  onFilterChange,
  onApplyFilters,
  onResetFilters,
}: OrgUsageSectionProps) {
  return (
    <div data-testid="org-usage-section" className="space-y-4">
      <FilterBar
        fields={USAGE_FILTER_FIELDS}
        values={filterValues}
        onChange={onFilterChange}
        onApply={onApplyFilters}
        onReset={onResetFilters}
      />
      {!summary ? (
        <div className="text-sm text-gray-400 py-4">No usage data available</div>
      ) : (
        <>
          <div className="text-sm text-gray-500 dark:text-gray-400">
            Period: {summary.period} | Tenants: {summary.tenantCount}
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="rounded border border-gray-200 dark:border-gray-700 p-3">
              <div className="text-xs text-gray-500 mb-1">API Requests</div>
              <div className="text-lg font-semibold text-gray-900 dark:text-gray-100">{summary.totals.apiRequests}</div>
            </div>
            <div className="rounded border border-gray-200 dark:border-gray-700 p-3">
              <div className="text-xs text-gray-500 mb-1">Storage (bytes)</div>
              <div className="text-lg font-semibold text-gray-900 dark:text-gray-100">{summary.totals.storageBytesUsed}</div>
            </div>
            <div className="rounded border border-gray-200 dark:border-gray-700 p-3">
              <div className="text-xs text-gray-500 mb-1">Bandwidth (bytes)</div>
              <div className="text-lg font-semibold text-gray-900 dark:text-gray-100">{summary.totals.bandwidthBytes}</div>
            </div>
            <div className="rounded border border-gray-200 dark:border-gray-700 p-3">
              <div className="text-xs text-gray-500 mb-1">Function Invocations</div>
              <div className="text-lg font-semibold text-gray-900 dark:text-gray-100">{summary.totals.functionInvocations}</div>
            </div>
          </div>
          {summary.data.length > 0 && (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-xs text-gray-500">
                  <th className="py-2 pr-4">Date</th>
                  <th className="py-2 pr-4">API Reqs</th>
                  <th className="py-2 pr-4">Storage</th>
                  <th className="py-2 pr-4">Bandwidth</th>
                  <th className="py-2">Functions</th>
                </tr>
              </thead>
              <tbody>
                {summary.data.map((day) => (
                  <tr key={day.date} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="py-1.5 pr-4 text-gray-900 dark:text-gray-100">{day.date}</td>
                    <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">{day.apiRequests}</td>
                    <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">{day.storageBytesUsed}</td>
                    <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">{day.bandwidthBytes}</td>
                    <td className="py-1.5 text-gray-600 dark:text-gray-300">{day.functionInvocations}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Audit section (reuses FilterBar pattern from tenant audit)
// ---------------------------------------------------------------------------

interface OrgAuditSectionProps {
  items: OrgAuditEvent[];
  count: number;
  filterValues: Record<string, string>;
  onFilterChange: (name: string, value: string) => void;
  onApplyFilters: (values: Record<string, string>) => void;
  onResetFilters: () => void;
}

const AUDIT_FILTER_FIELDS: FilterField[] = [
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
  { name: "action", label: "Action", type: "text", placeholder: "org.updated" },
  { name: "result", label: "Result", type: "text", placeholder: "success" },
  { name: "actorId", label: "Actor ID", type: "text", placeholder: "user-123" },
];

export function OrgAuditSection({
  items,
  count,
  filterValues,
  onFilterChange,
  onApplyFilters,
  onResetFilters,
}: OrgAuditSectionProps) {
  return (
    <div data-testid="org-audit-section">
      <FilterBar
        fields={AUDIT_FILTER_FIELDS}
        values={filterValues}
        onChange={onFilterChange}
        onApply={onApplyFilters}
        onReset={onResetFilters}
      />
      {items.length === 0 ? (
        <div className="text-sm text-gray-400 py-4">No audit events found</div>
      ) : (
        <>
          <div className="text-xs text-gray-500 mb-2">{count} event{count !== 1 ? "s" : ""}</div>
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-xs text-gray-500">
                <th className="py-2 pr-4">Action</th>
                <th className="py-2 pr-4">Result</th>
                <th className="py-2 pr-4">Actor</th>
                <th className="py-2">Time</th>
              </tr>
            </thead>
            <tbody>
              {items.map((event) => (
                <tr key={event.id} className="border-b border-gray-100 dark:border-gray-800">
                  <td className="py-1.5 pr-4 text-gray-900 dark:text-gray-100">{event.action}</td>
                  <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">{event.result}</td>
                  <td className="py-1.5 pr-4 text-gray-500 dark:text-gray-400">{event.actorId ?? "—"}</td>
                  <td className="py-1.5 text-gray-500 dark:text-gray-400">{event.createdAt}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  );
}
