import type {
  BreakerStateResponse,
  DetailTab,
  Tenant,
  TenantAuditEvent,
  TenantMaintenanceState,
} from "../types/tenants";
import { DETAIL_TAB_VALUES } from "../types/tenants";
import { FilterBar, type FilterField } from "./shared/FilterBar";

// ---------------------------------------------------------------------------
// Tenant list panel (left side)
// ---------------------------------------------------------------------------

interface TenantListPanelProps {
  items: Tenant[];
  page: number;
  totalPages: number;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onCreateTenant: () => void;
  onNextPage: () => void;
  onPrevPage: () => void;
}

const STATE_BADGE_COLORS: Record<string, string> = {
  active: "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300",
  suspended: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300",
  provisioning: "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300",
  deleting: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300",
  deleted: "bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400",
};

export function TenantListPanel({
  items,
  page,
  totalPages,
  selectedId,
  onSelect,
  onCreateTenant,
  onNextPage,
  onPrevPage,
}: TenantListPanelProps) {
  return (
    <div
      data-testid="tenant-list-panel"
      className="w-72 flex flex-col border-r border-gray-200 dark:border-gray-700 overflow-hidden"
    >
      <div className="px-3 py-2 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-300">Tenants</h2>
        <button
          onClick={onCreateTenant}
          className="px-2 py-1 text-xs font-medium rounded bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900 dark:text-blue-300"
        >
          Create Tenant
        </button>
      </div>
      <div className="flex-1 overflow-auto">
        {items.map((t) => (
          <button
            key={t.id}
            onClick={() => onSelect(t.id)}
            className={`w-full text-left px-3 py-2 text-sm border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-900 ${
              selectedId === t.id ? "bg-blue-50 dark:bg-blue-950" : ""
            }`}
          >
            <div className="font-medium text-gray-900 dark:text-gray-100">{t.name}</div>
            <div className="flex items-center gap-2 mt-0.5">
              <span className="text-xs text-gray-500 dark:text-gray-400">{t.slug}</span>
              <span
                className={`text-xs px-1.5 py-0.5 rounded ${STATE_BADGE_COLORS[t.state] ?? "bg-gray-100 text-gray-500"}`}
              >
                {t.state}
              </span>
            </div>
          </button>
        ))}
      </div>
      {totalPages > 0 && (
        <div className="px-3 py-2 border-t border-gray-200 dark:border-gray-700 flex items-center justify-between text-xs text-gray-500">
          <button onClick={onPrevPage} disabled={page <= 1} className="disabled:opacity-40">
            ← Prev
          </button>
          <span>Page {page} of {totalPages}</span>
          <button onClick={onNextPage} disabled={page >= totalPages} className="disabled:opacity-40">
            Next →
          </button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Detail header with tabs + action buttons
// ---------------------------------------------------------------------------

interface TenantDetailHeaderProps {
  tenant: Tenant;
  activeTab: DetailTab;
  onTabChange: (tab: DetailTab) => void;
  onSuspend: () => void;
  onResume: () => void;
  onDelete: () => void;
}

const TAB_LABELS: Record<DetailTab, string> = {
  info: "Info",
  members: "Members",
  maintenance: "Maintenance",
  audit: "Audit",
};

export function TenantDetailHeader({
  tenant,
  activeTab,
  onTabChange,
  onSuspend,
  onResume,
  onDelete,
}: TenantDetailHeaderProps) {
  return (
    <div className="mb-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">{tenant.name}</h2>
        <div className="flex gap-2">
          {tenant.state === "active" && (
            <button
              onClick={onSuspend}
              className="px-3 py-1 text-xs rounded bg-yellow-100 text-yellow-700 hover:bg-yellow-200 dark:bg-yellow-900 dark:text-yellow-300"
            >
              Suspend
            </button>
          )}
          {tenant.state === "suspended" && (
            <button
              onClick={onResume}
              className="px-3 py-1 text-xs rounded bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900 dark:text-green-300"
            >
              Resume
            </button>
          )}
          {(tenant.state === "active" || tenant.state === "suspended") && (
            <button
              onClick={onDelete}
              className="px-3 py-1 text-xs rounded bg-red-100 text-red-700 hover:bg-red-200 dark:bg-red-900 dark:text-red-300"
            >
              Delete
            </button>
          )}
        </div>
      </div>
      <div className="flex gap-1 bg-gray-100 dark:bg-gray-800 rounded p-0.5">
        {DETAIL_TAB_VALUES.map((tab) => (
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
// Maintenance section (maintenance + breaker)
// ---------------------------------------------------------------------------

interface TenantMaintenanceSectionProps {
  maintenance: TenantMaintenanceState | null;
  breaker: BreakerStateResponse | null;
  onEnableMaintenance: () => void;
  onDisableMaintenance: () => void;
  onResetBreaker: () => void;
}

export function TenantMaintenanceSection({
  maintenance,
  breaker,
  onEnableMaintenance,
  onDisableMaintenance,
  onResetBreaker,
}: TenantMaintenanceSectionProps) {
  return (
    <div data-testid="tenant-maintenance-section" className="space-y-4">
      <div>
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">Maintenance Mode</h3>
        {maintenance ? (
          <div className="text-sm space-y-1">
            <div>Status: {maintenance.enabled ? "Enabled" : "Disabled"}</div>
            {maintenance.reason && <div>Reason: {maintenance.reason}</div>}
            <div className="mt-2">
              {maintenance.enabled ? (
                <button
                  onClick={onDisableMaintenance}
                  className="px-3 py-1 text-xs rounded bg-green-100 text-green-700 hover:bg-green-200"
                >
                  Disable Maintenance
                </button>
              ) : (
                <button
                  onClick={onEnableMaintenance}
                  className="px-3 py-1 text-xs rounded bg-yellow-100 text-yellow-700 hover:bg-yellow-200"
                >
                  Enable Maintenance
                </button>
              )}
            </div>
          </div>
        ) : (
          <div className="text-sm text-gray-400">No maintenance state</div>
        )}
      </div>
      <div>
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">Circuit Breaker</h3>
        {breaker ? (
          <div className="text-sm space-y-1">
            <div>State: {breaker.state}</div>
            <div>Consecutive Failures: {breaker.consecutiveFailures}</div>
            <div>Half-Open Probes: {breaker.halfOpenProbes}</div>
            {breaker.state !== "closed" && (
              <button
                onClick={onResetBreaker}
                className="mt-2 px-3 py-1 text-xs rounded bg-blue-100 text-blue-700 hover:bg-blue-200"
              >
                Reset Breaker
              </button>
            )}
          </div>
        ) : (
          <div className="text-sm text-gray-400">No breaker state</div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Audit section
// ---------------------------------------------------------------------------

interface TenantAuditSectionProps {
  items: TenantAuditEvent[];
  count: number;
  filterValues: Record<string, string>;
  onFilterChange: (name: string, value: string) => void;
  onApplyFilters: (values: Record<string, string>) => void;
  onResetFilters: () => void;
}

const AUDIT_FILTER_FIELDS: FilterField[] = [
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
  { name: "action", label: "Action", type: "text", placeholder: "tenant.suspended" },
  { name: "result", label: "Result", type: "text", placeholder: "success" },
  { name: "actorId", label: "Actor ID", type: "text", placeholder: "user-123" },
];

export function TenantAuditSection({
  items,
  count,
  filterValues,
  onFilterChange,
  onApplyFilters,
  onResetFilters,
}: TenantAuditSectionProps) {
  const hasItems = items.length > 0;

  if (items.length === 0) {
    return (
      <div data-testid="tenant-audit-section" className="text-sm text-gray-400 py-4">
        <FilterBar
          fields={AUDIT_FILTER_FIELDS}
          values={filterValues}
          onChange={onFilterChange}
          onApply={onApplyFilters}
          onReset={onResetFilters}
        />
        No audit events found
      </div>
    );
  }

  return (
    <div data-testid="tenant-audit-section">
      <FilterBar
        fields={AUDIT_FILTER_FIELDS}
        values={filterValues}
        onChange={onFilterChange}
        onApply={onApplyFilters}
        onReset={onResetFilters}
      />
      <div className="text-xs text-gray-500 mb-2">{count} event{count !== 1 ? "s" : ""}</div>
      {hasItems && <table className="w-full text-sm">
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
      </table>}
    </div>
  );
}
