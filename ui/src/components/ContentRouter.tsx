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

interface ContentRouterProps {
  schema: SchemaCache;
  view: View;
  isAdminView: boolean;
  selected: Table | null;
  onRefresh: () => void | Promise<void>;
  onSetView: (view: View) => void;
  onSelectAdminView: (view: AdminView) => void;
}

export function ContentRouter({
  schema,
  view,
  isAdminView,
  selected,
  onRefresh,
  onSetView,
  onSelectAdminView: _onSelectAdminView,
}: ContentRouterProps) {
  if (isAdminView) {
    return (
      <main className="flex-1 flex flex-col overflow-hidden bg-gray-50 dark:bg-gray-950">
        <div className="flex-1 overflow-auto">
          {view === "webhooks" ? (
            <Webhooks />
          ) : view === "storage" ? (
            <StorageBrowser />
          ) : view === "sites" ? (
            <Sites />
          ) : view === "functions" ? (
            <FunctionBrowser functions={schema.functions || {}} />
          ) : view === "edge-functions" ? (
            <EdgeFunctions />
          ) : view === "apps" ? (
            <Apps />
          ) : view === "api-keys" ? (
            <ApiKeys />
          ) : view === "oauth-clients" ? (
            <OAuthClients />
          ) : view === "api-explorer" ? (
            <ApiExplorer schema={schema} />
          ) : view === "rls" ? (
            <RlsPolicies schema={schema} />
          ) : view === "sql-editor" ? (
            <SqlEditor onSchemaChange={onRefresh} />
          ) : view === "sms-health" ? (
            <SMSHealth />
          ) : view === "sms-messages" ? (
            <SMSMessages />
          ) : view === "email-templates" ? (
            <EmailTemplates />
          ) : view === "push" ? (
            <PushNotifications />
          ) : view === "jobs" ? (
            <Jobs />
          ) : view === "schedules" ? (
            <Schedules />
          ) : view === "matviews" ? (
            <MatviewsAdmin schema={schema} />
          ) : view === "schema-designer" ? (
            <SchemaDesigner schema={schema} />
          ) : view === "auth-settings" ? (
            <AuthSettings />
          ) : view === "mfa-management" ? (
            <MFAEnrollment />
          ) : view === "account-linking" ? (
            <AccountLinking onLinked={() => {}} />
          ) : view === "branches" ? (
            <Branches />
          ) : view === "realtime-inspector" ? (
            <RealtimeInspector />
          ) : view === "security-advisor" ? (
            <SecurityAdvisor />
          ) : view === "performance-advisor" ? (
            <PerformanceAdvisor />
          ) : view === "backups" ? (
            <Backups />
          ) : view === "analytics" ? (
            <Analytics />
          ) : view === "usage" ? (
            <UsageMetering />
          ) : view === "replicas" ? (
            <Replicas />
          ) : view === "ai-assistant" ? (
            <AIAssistant />
          ) : view === "audit-logs" ? (
            <AuditLogs />
          ) : view === "admin-logs" ? (
            <AdminLogs />
          ) : view === "secrets" ? (
            <Secrets />
          ) : view === "saml" ? (
            <SAMLConfig />
          ) : view === "custom-domains" ? (
            <CustomDomains />
          ) : view === "extensions" ? (
            <Extensions />
          ) : view === "vector-indexes" ? (
            <VectorIndexes />
          ) : view === "log-drains" ? (
            <LogDrains />
          ) : view === "stats" ? (
            <StatsOverview />
          ) : view === "auth-hooks" ? (
            <AuthHooks />
          ) : view === "notifications" ? (
            <Notifications />
          ) : view === "fdw" ? (
            <FDWManagement />
          ) : view === "incidents" ? (
            <Incidents />
          ) : view === "support-tickets" ? (
            <SupportTickets />
          ) : view === "tenants" ? (
            <Tenants />
          ) : view === "organizations" ? (
            <Organizations />
          ) : (
            <Users />
          )}
        </div>
      </main>
    );
  }

  if (selected) {
    return (
      <main className="flex-1 flex flex-col overflow-hidden bg-gray-50 dark:bg-gray-950">
        <header className="border-b border-gray-200 dark:border-gray-700 px-6 py-3 flex items-center gap-4">
          <h1 className="font-semibold text-gray-900 dark:text-gray-100">
            {selected.schema !== "public" && (
              <span className="text-gray-400 dark:text-gray-500">{selected.schema}.</span>
            )}
            {selected.name}
          </h1>
          <span className="text-xs text-gray-500 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 rounded px-2 py-0.5">
            {selected.kind}
          </span>

          <div className="ml-auto flex gap-1 bg-gray-100 dark:bg-gray-800 rounded p-0.5">
            <button
              onClick={() => onSetView("data")}
              className={cn(
                "px-3 py-1 text-xs rounded font-medium transition-colors",
                view === "data"
                  ? "bg-white dark:bg-gray-900 shadow-sm text-gray-900 dark:text-gray-100"
                  : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200",
              )}
            >
              <TableIcon className="w-3.5 h-3.5 inline mr-1" />
              Data
            </button>
            <button
              onClick={() => onSetView("schema")}
              className={cn(
                "px-3 py-1 text-xs rounded font-medium transition-colors",
                view === "schema"
                  ? "bg-white dark:bg-gray-900 shadow-sm text-gray-900 dark:text-gray-100"
                  : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200",
              )}
            >
              <Columns3 className="w-3.5 h-3.5 inline mr-1" />
              Schema
            </button>
            <button
              onClick={() => onSetView("sql")}
              className={cn(
                "px-3 py-1 text-xs rounded font-medium transition-colors",
                view === "sql"
                  ? "bg-white dark:bg-gray-900 shadow-sm text-gray-900 dark:text-gray-100"
                  : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200",
              )}
            >
              <Code className="w-3.5 h-3.5 inline mr-1" />
              SQL
            </button>
          </div>
        </header>

        <div className="flex-1 overflow-auto">
          {view === "data" ? (
            <TableBrowser table={selected} />
          ) : view === "schema" ? (
            <SchemaView table={selected} />
          ) : (
            <SqlEditor onSchemaChange={onRefresh} />
          )}
        </div>
      </main>
    );
  }

  return (
    <main className="flex-1 flex flex-col overflow-hidden bg-gray-50 dark:bg-gray-950">
      <div className="flex-1 flex flex-col items-center justify-center text-gray-500 dark:text-gray-400">
        <TableProperties className="w-12 h-12 text-gray-300 dark:text-gray-700 mb-3" />
        <p className="text-sm mb-1">Select a table from the sidebar</p>
        <p className="text-xs text-gray-400 dark:text-gray-500">
          Use SQL Editor from the sidebar to create one.
        </p>
      </div>
    </main>
  );
}
