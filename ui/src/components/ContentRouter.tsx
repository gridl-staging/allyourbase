import type { SchemaCache, Table } from "../types";
import { TableBrowser } from "./TableBrowser";
import { SchemaView } from "./SchemaView";
import { SqlEditor } from "./SqlEditor";
import { Webhooks } from "./Webhooks";
import { StorageBrowser } from "./StorageBrowser";
import { Users } from "./Users";
import { FunctionBrowser } from "./FunctionBrowser";
import { SMSHealth } from "./SMSHealth";
import { SMSMessages } from "./SMSMessages";
import { EdgeFunctions } from "./EdgeFunctions";
import { ApiKeys } from "./ApiKeys";
import { Apps } from "./Apps";
import { OAuthClients } from "./OAuthClients";
import { ApiExplorer } from "./ApiExplorer";
import { RlsPolicies } from "./RlsPolicies";
import { Jobs } from "./Jobs";
import { Schedules } from "./Schedules";
import { MatviewsAdmin } from "./MatviewsAdmin";
import { EmailTemplates } from "./EmailTemplates";
import { PushNotifications } from "./PushNotifications";
import { AuthSettings } from "./AuthSettings";
import { MFAEnrollment } from "./MFAEnrollment";
import { AccountLinking } from "./AccountLinking";
import { Branches } from "./Branches";
import { SchemaDesigner } from "./SchemaDesigner";
import { RealtimeInspector } from "./RealtimeInspector";
import { SecurityAdvisor } from "./SecurityAdvisor";
import { PerformanceAdvisor } from "./PerformanceAdvisor";
import { Backups } from "./Backups";
import { Analytics } from "./Analytics";
import { UsageMetering } from "./UsageMetering";
import { Replicas } from "./Replicas";
import { AIAssistant } from "./AIAssistant";
import { AuditLogs } from "./AuditLogs";
import { AdminLogs } from "./AdminLogs";
import { Secrets } from "./Secrets";
import { SAMLConfig } from "./SAMLConfig";
import { CustomDomains } from "./CustomDomains";
import { Sites } from "./Sites";
import { Extensions } from "./Extensions";
import { VectorIndexes } from "./VectorIndexes";
import { LogDrains } from "./LogDrains";
import { StatsOverview } from "./StatsOverview";
import { AuthHooks } from "./AuthHooks";
import { Notifications } from "./Notifications";
import { FDWManagement } from "./FDWManagement";
import { Incidents } from "./Incidents";
import { SupportTickets } from "./SupportTickets";
import { Tenants } from "./Tenants";
import { Organizations } from "./Organizations";
import type { AdminView, View } from "./layout-types";
import { Code, Columns3, Table as TableIcon, TableProperties } from "lucide-react";
import { cn } from "../lib/utils";

const CONTENT_ROUTER_MAIN_CLASS = "flex-1 flex flex-col overflow-hidden bg-gray-50 dark:bg-gray-950";
const VIEW_TOGGLE_BUTTON_CLASS = "px-3 py-1 text-xs rounded font-medium transition-colors";
const VIEW_TOGGLE_ACTIVE_CLASS = "bg-white dark:bg-gray-900 shadow-sm text-gray-900 dark:text-gray-100";
const VIEW_TOGGLE_INACTIVE_CLASS = "text-gray-600 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200";

interface ContentRouterProps {
  schema: SchemaCache;
  view: View;
  isAdminView: boolean;
  selected: Table | null;
  onRefresh: () => void | Promise<void>;
  onSetView: (view: View) => void;
  onSelectAdminView: (view: AdminView) => void;
}

interface TableViewToggleButtonProps {
  active: boolean;
  icon: typeof TableIcon;
  label: string;
  onClick: () => void;
}

function TableViewToggleButton({
  active,
  icon: Icon,
  label,
  onClick,
}: TableViewToggleButtonProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        VIEW_TOGGLE_BUTTON_CLASS,
        active ? VIEW_TOGGLE_ACTIVE_CLASS : VIEW_TOGGLE_INACTIVE_CLASS,
      )}
    >
      <Icon className="w-3.5 h-3.5 inline mr-1" />
      {label}
    </button>
  );
}

function renderAdminContent(
  view: View,
  schema: SchemaCache,
  onRefresh: () => void | Promise<void>,
) {
  switch (view) {
    case "webhooks":
      return <Webhooks />;
    case "storage":
      return <StorageBrowser />;
    case "sites":
      return <Sites />;
    case "functions":
      return <FunctionBrowser functions={schema.functions || {}} />;
    case "edge-functions":
      return <EdgeFunctions />;
    case "apps":
      return <Apps />;
    case "api-keys":
      return <ApiKeys />;
    case "oauth-clients":
      return <OAuthClients />;
    case "api-explorer":
      return <ApiExplorer schema={schema} />;
    case "rls":
      return <RlsPolicies schema={schema} />;
    case "sql-editor":
      return <SqlEditor onSchemaChange={onRefresh} />;
    case "sms-health":
      return <SMSHealth />;
    case "sms-messages":
      return <SMSMessages />;
    case "email-templates":
      return <EmailTemplates />;
    case "push":
      return <PushNotifications />;
    case "jobs":
      return <Jobs />;
    case "schedules":
      return <Schedules />;
    case "matviews":
      return <MatviewsAdmin schema={schema} />;
    case "schema-designer":
      return <SchemaDesigner schema={schema} />;
    case "auth-settings":
      return <AuthSettings />;
    case "mfa-management":
      return <MFAEnrollment />;
    case "account-linking":
      return <AccountLinking onLinked={() => {}} />;
    case "branches":
      return <Branches />;
    case "realtime-inspector":
      return <RealtimeInspector />;
    case "security-advisor":
      return <SecurityAdvisor />;
    case "performance-advisor":
      return <PerformanceAdvisor />;
    case "backups":
      return <Backups />;
    case "analytics":
      return <Analytics />;
    case "usage":
      return <UsageMetering />;
    case "replicas":
      return <Replicas />;
    case "ai-assistant":
      return <AIAssistant />;
    case "audit-logs":
      return <AuditLogs />;
    case "admin-logs":
      return <AdminLogs />;
    case "secrets":
      return <Secrets />;
    case "saml":
      return <SAMLConfig />;
    case "custom-domains":
      return <CustomDomains />;
    case "extensions":
      return <Extensions />;
    case "vector-indexes":
      return <VectorIndexes />;
    case "log-drains":
      return <LogDrains />;
    case "stats":
      return <StatsOverview />;
    case "auth-hooks":
      return <AuthHooks />;
    case "notifications":
      return <Notifications />;
    case "fdw":
      return <FDWManagement />;
    case "incidents":
      return <Incidents />;
    case "support-tickets":
      return <SupportTickets />;
    case "tenants":
      return <Tenants />;
    case "organizations":
      return <Organizations />;
    default:
      return <Users />;
  }
}

function renderSelectedContent(
  view: View,
  selected: Table,
  onRefresh: () => void | Promise<void>,
) {
  switch (view) {
    case "schema":
      return <SchemaView table={selected} />;
    case "sql":
      return <SqlEditor onSchemaChange={onRefresh} />;
    case "data":
    default:
      return <TableBrowser table={selected} />;
  }
}

export function ContentRouter({
  schema,
  view,
  isAdminView,
  selected,
  onRefresh,
  onSetView,
}: ContentRouterProps) {
  if (isAdminView) {
    return (
      <main className={CONTENT_ROUTER_MAIN_CLASS}>
        <div className="flex-1 overflow-auto">{renderAdminContent(view, schema, onRefresh)}</div>
      </main>
    );
  }

  if (selected) {
    return (
      <main className={CONTENT_ROUTER_MAIN_CLASS}>
        <header className="border-b border-gray-200 dark:border-gray-700 px-6 py-3 flex items-center gap-4">
          <h1 className="font-semibold text-gray-900 dark:text-gray-100">
            {selected.schema !== "public" && (
              <span className="text-gray-600 dark:text-gray-400">{selected.schema}.</span>
            )}
            {selected.name}
          </h1>
          <span className="text-xs text-gray-600 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded px-2 py-0.5">
            {selected.kind}
          </span>

          <div className="ml-auto flex gap-1 bg-gray-100 dark:bg-gray-800 rounded p-0.5">
            <TableViewToggleButton
              active={view === "data"}
              icon={TableIcon}
              label="Data"
              onClick={() => onSetView("data")}
            />
            <TableViewToggleButton
              active={view === "schema"}
              icon={Columns3}
              label="Schema"
              onClick={() => onSetView("schema")}
            />
            <TableViewToggleButton
              active={view === "sql"}
              icon={Code}
              label="SQL"
              onClick={() => onSetView("sql")}
            />
          </div>
        </header>

        <div className="flex-1 overflow-auto">{renderSelectedContent(view, selected, onRefresh)}</div>
      </main>
    );
  }

  return (
    <main className={CONTENT_ROUTER_MAIN_CLASS}>
      <div className="flex-1 flex flex-col items-center justify-center text-gray-500 dark:text-gray-400">
        <TableProperties className="w-12 h-12 text-gray-300 dark:text-gray-700 mb-3" />
        <p className="text-sm mb-1">Select a table from the sidebar</p>
        <p className="text-xs text-gray-600 dark:text-gray-400">
          Use SQL Editor from the sidebar to create one.
        </p>
      </div>
    </main>
  );
}
