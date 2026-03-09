import { useState, useCallback, useEffect } from "react";
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
import { ApiKeys } from "./ApiKeys";
import { Apps } from "./Apps";
import { OAuthClients } from "./OAuthClients";
import { ApiExplorer } from "./ApiExplorer";
import { RlsPolicies } from "./RlsPolicies";
import { Jobs } from "./Jobs";
import { Schedules } from "./Schedules";
import { MatviewsAdmin } from "./MatviewsAdmin";
import { EmailTemplates } from "./EmailTemplates";
import { CommandPalette, CommandPaletteHint } from "./CommandPalette";
import type { CommandAction } from "./CommandPalette";
import {
  Table as TableIcon,
  Columns3,
  Code,
  LogOut,
  RefreshCw,
  Webhook,
  HardDrive,
  Users as UsersIcon,
  Zap,
  KeyRound,
  Compass,
  Shield,
  Plus,
  TableProperties,
  MessageCircle,
  MessageSquare,
  Box,
  Fingerprint,
  CalendarClock,
  ListTodo,
  Layers,
  Mail,
} from "lucide-react";
import { cn } from "../lib/utils";

type View = "data" | "schema" | "sql" | "webhooks" | "storage" | "users" | "functions" | "apps" | "api-keys" | "oauth-clients" | "api-explorer" | "rls" | "sql-editor" | "sms-health" | "sms-messages" | "email-templates" | "jobs" | "schedules" | "matviews";

interface LayoutProps {
  schema: SchemaCache;
  onLogout: () => void;
  onRefresh: () => void;
}

export function Layout({ schema, onLogout, onRefresh }: LayoutProps) {
  const tables = Object.values(schema.tables).sort((a, b) =>
    `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`),
  );
  const [selected, setSelected] = useState<Table | null>(
    tables.length > 0 ? tables[0] : null,
  );
  const [view, setView] = useState<View>("data");
  const [cmdOpen, setCmdOpen] = useState(false);

  const handleSelect = useCallback((t: Table) => {
    setSelected(t);
    setView("data");
  }, []);

  const handleAdminView = useCallback((v: "webhooks" | "storage" | "users" | "functions" | "apps" | "api-keys" | "oauth-clients" | "api-explorer" | "rls" | "sql-editor" | "sms-health" | "sms-messages" | "email-templates" | "jobs" | "schedules" | "matviews") => {
    setSelected(null);
    setView(v);
  }, []);

  const handleCommand = useCallback((action: CommandAction) => {
    if (action.kind === "table") {
      handleSelect(action.table);
    } else {
      handleAdminView(action.view as any);
    }
  }, [handleSelect, handleAdminView]);

  // Global Cmd+K / Ctrl+K listener
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setCmdOpen((prev) => !prev);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const isAdminView = view === "webhooks" || view === "storage" || view === "users" || view === "functions" || view === "apps" || view === "api-keys" || view === "oauth-clients" || view === "api-explorer" || view === "rls" || view === "sql-editor" || view === "sms-health" || view === "sms-messages" || view === "email-templates" || view === "jobs" || view === "schedules" || view === "matviews";

  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <CommandPalette
        open={cmdOpen}
        onClose={() => setCmdOpen(false)}
        onSelect={handleCommand}
        tables={tables}
      />

      <aside className="w-60 border-r bg-white flex flex-col">
        <div className="px-4 py-3 border-b flex items-center gap-2">
          <span className="text-base">👾</span>
          <span className="font-semibold text-sm">Allyourbase</span>
        </div>

        <CommandPaletteHint onClick={() => setCmdOpen(true)} />

        <nav className="flex-1 overflow-y-auto py-2">
          {/* Tables section */}
          <div className="px-4 pb-1 flex items-center justify-between">
            <p className="text-[10px] font-medium text-gray-400 uppercase tracking-wider">
              Tables
            </p>
            <button
              onClick={() => handleAdminView("sql-editor")}
              className="text-[10px] text-gray-400 hover:text-gray-600 flex items-center gap-0.5"
              title="New Table (opens SQL Editor)"
              aria-label="New Table"
            >
              <Plus className="w-3 h-3" />
              New Table
            </button>
          </div>
          {tables.length === 0 ? (
            <div className="px-4 py-6 text-center">
              <TableProperties className="w-8 h-8 text-gray-300 mx-auto mb-2" />
              <p className="text-xs text-gray-400 mb-1">No tables yet</p>
              <p className="text-[11px] text-gray-300 mb-3">
                Create your first table to get started.
              </p>
              <button
                onClick={() => handleAdminView("sql-editor")}
                className="px-3 py-1.5 text-xs bg-gray-900 text-white rounded hover:bg-gray-800 font-medium"
              >
                Open SQL Editor
              </button>
            </div>
          ) : (
            tables.map((t) => {
              const key = `${t.schema}.${t.name}`;
              const isSelected =
                !isAdminView &&
                selected &&
                selected.schema === t.schema &&
                selected.name === t.name;
              return (
                <button
                  key={key}
                  onClick={() => handleSelect(t)}
                  className={cn(
                    "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100",
                    isSelected && "bg-gray-100 font-medium",
                  )}
                >
                  <TableIcon className="w-3.5 h-3.5 text-gray-400 shrink-0" />
                  <span className="truncate">
                    {t.schema !== "public" && (
                      <span className="text-gray-400">{t.schema}.</span>
                    )}
                    {t.name}
                  </span>
                </button>
              );
            })
          )}

          {/* Database section */}
          <div className="mt-3 pt-3 border-t mx-3">
            <p className="px-1 pb-1 text-[10px] font-medium text-gray-400 uppercase tracking-wider">
              Database
            </p>
            <button
              onClick={() => handleAdminView("sql-editor")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "sql-editor" && "bg-gray-100 font-medium",
              )}
            >
              <Code className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              SQL Editor
            </button>
            <button
              onClick={() => handleAdminView("functions")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "functions" && "bg-gray-100 font-medium",
              )}
            >
              <Zap className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Functions
            </button>
            <button
              onClick={() => handleAdminView("rls")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "rls" && "bg-gray-100 font-medium",
              )}
            >
              <Shield className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              RLS Policies
            </button>
            <button
              onClick={() => handleAdminView("matviews")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "matviews" && "bg-gray-100 font-medium",
              )}
            >
              <Layers className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Matviews
            </button>
          </div>

          {/* Services section */}
          <div className="mt-3 pt-3 border-t mx-3">
            <p className="px-1 pb-1 text-[10px] font-medium text-gray-400 uppercase tracking-wider">
              Services
            </p>
            <button
              onClick={() => handleAdminView("storage")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "storage" && "bg-gray-100 font-medium",
              )}
            >
              <HardDrive className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Storage
            </button>
            <button
              onClick={() => handleAdminView("webhooks")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "webhooks" && "bg-gray-100 font-medium",
              )}
            >
              <Webhook className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Webhooks
            </button>
          </div>

          {/* Messaging section */}
          <div className="mt-3 pt-3 border-t mx-3">
            <p className="px-1 pb-1 text-[10px] font-medium text-gray-400 uppercase tracking-wider">
              Messaging
            </p>
            <button
              onClick={() => handleAdminView("sms-health")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "sms-health" && "bg-gray-100 font-medium",
              )}
            >
              <MessageCircle className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              SMS Health
            </button>
            <button
              onClick={() => handleAdminView("sms-messages")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "sms-messages" && "bg-gray-100 font-medium",
              )}
            >
              <MessageSquare className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              SMS Messages
            </button>
            <button
              onClick={() => handleAdminView("email-templates")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "email-templates" && "bg-gray-100 font-medium",
              )}
            >
              <Mail className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Email Templates
            </button>
          </div>

          {/* Admin section */}
          <div className="mt-3 pt-3 border-t mx-3">
            <p className="px-1 pb-1 text-[10px] font-medium text-gray-400 uppercase tracking-wider">
              Admin
            </p>
            <button
              onClick={() => handleAdminView("users")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "users" && "bg-gray-100 font-medium",
              )}
            >
              <UsersIcon className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Users
            </button>
            <button
              onClick={() => handleAdminView("apps")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "apps" && "bg-gray-100 font-medium",
              )}
            >
              <Box className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Apps
            </button>
            <button
              onClick={() => handleAdminView("api-keys")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "api-keys" && "bg-gray-100 font-medium",
              )}
            >
              <KeyRound className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              API Keys
            </button>
            <button
              onClick={() => handleAdminView("oauth-clients")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "oauth-clients" && "bg-gray-100 font-medium",
              )}
            >
              <Fingerprint className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              OAuth Clients
            </button>
            <button
              onClick={() => handleAdminView("api-explorer")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "api-explorer" && "bg-gray-100 font-medium",
              )}
            >
              <Compass className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              API Explorer
            </button>
            <button
              onClick={() => handleAdminView("jobs")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "jobs" && "bg-gray-100 font-medium",
              )}
            >
              <ListTodo className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Jobs
            </button>
            <button
              onClick={() => handleAdminView("schedules")}
              className={cn(
                "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 rounded",
                view === "schedules" && "bg-gray-100 font-medium",
              )}
            >
              <CalendarClock className="w-3.5 h-3.5 text-gray-400 shrink-0" />
              Schedules
            </button>
          </div>
        </nav>

        <div className="border-t p-2 flex gap-1">
          <button
            onClick={onRefresh}
            className="p-2 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
            title="Refresh schema"
            aria-label="Refresh schema"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
          <button
            onClick={onLogout}
            className="p-2 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
            title="Log out"
            aria-label="Log out"
          >
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 flex flex-col overflow-hidden">
        {isAdminView ? (
          <div className="flex-1 overflow-auto">
            {view === "webhooks" ? (
              <Webhooks />
            ) : view === "storage" ? (
              <StorageBrowser />
            ) : view === "functions" ? (
              <FunctionBrowser functions={schema.functions || {}} />
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
            ) : view === "jobs" ? (
              <Jobs />
            ) : view === "schedules" ? (
              <Schedules />
            ) : view === "matviews" ? (
              <MatviewsAdmin schema={schema} />
            ) : (
              <Users />
            )}
          </div>
        ) : selected ? (
          <>
            <header className="border-b px-6 py-3 flex items-center gap-4">
              <h1 className="font-semibold">
                {selected.schema !== "public" && (
                  <span className="text-gray-400">{selected.schema}.</span>
                )}
                {selected.name}
              </h1>
              <span className="text-xs text-gray-400 bg-gray-100 rounded px-2 py-0.5">
                {selected.kind}
              </span>

              <div className="ml-auto flex gap-1 bg-gray-100 rounded p-0.5">
                <button
                  onClick={() => setView("data")}
                  className={cn(
                    "px-3 py-1 text-xs rounded font-medium",
                    view === "data"
                      ? "bg-white shadow-sm text-gray-900"
                      : "text-gray-500 hover:text-gray-700",
                  )}
                >
                  <TableIcon className="w-3.5 h-3.5 inline mr-1" />
                  Data
                </button>
                <button
                  onClick={() => setView("schema")}
                  className={cn(
                    "px-3 py-1 text-xs rounded font-medium",
                    view === "schema"
                      ? "bg-white shadow-sm text-gray-900"
                      : "text-gray-500 hover:text-gray-700",
                  )}
                >
                  <Columns3 className="w-3.5 h-3.5 inline mr-1" />
                  Schema
                </button>
                <button
                  onClick={() => setView("sql")}
                  className={cn(
                    "px-3 py-1 text-xs rounded font-medium",
                    view === "sql"
                      ? "bg-white shadow-sm text-gray-900"
                      : "text-gray-500 hover:text-gray-700",
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
          </>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center text-gray-400">
            <TableProperties className="w-12 h-12 text-gray-200 mb-3" />
            <p className="text-sm mb-1">Select a table from the sidebar</p>
            <p className="text-xs text-gray-300">
              or{" "}
              <button
                onClick={() => handleAdminView("sql-editor")}
                className="text-blue-500 hover:text-blue-600 underline"
              >
                open the SQL Editor
              </button>
              {" "}to create one
            </p>
          </div>
        )}
      </main>
    </div>
  );
}
