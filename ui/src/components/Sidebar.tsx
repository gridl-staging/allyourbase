import type { Table } from "../types";
import { CommandPaletteHint } from "./CommandPalette";
import type { AdminView, View } from "./layout-types";
import {
  Table as TableIcon,
  Columns3,
  Code,
  LogOut,
  Moon,
  RefreshCw,
  Sun,
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
  Bell,
  Settings,
  ShieldCheck,
  Link,
  GitBranch,
  Activity,
  ShieldAlert,
  Gauge,
  Archive,
  BarChart3,
  Server,
  Sparkles,
  ScrollText,
  FileText,
  Lock,
  Globe,
  Puzzle,
  Database,
  ArrowDownToLine,
  LineChart,
  ShieldPlus,
  Anchor,
  BellRing,
  Cable,
  AlertTriangle,
  LifeBuoy,
  Building2,
} from "lucide-react";
import { cn } from "../lib/utils";

const SIDEBAR_ICON_CLASS = "w-3.5 h-3.5 text-gray-400 dark:text-gray-500 shrink-0";
const SIDEBAR_ITEM_BASE_CLASS = "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 rounded text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800";
const SIDEBAR_ITEM_ACTIVE_CLASS = "bg-gray-100 dark:bg-gray-800 font-medium text-gray-900 dark:text-gray-100";
const SIDEBAR_SECTION_CLASS = "mt-3 pt-3 border-t border-gray-200 dark:border-gray-700 mx-3";
const SIDEBAR_SECTION_TITLE_CLASS = "px-1 pb-1 text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase tracking-wider";
const SIDEBAR_ACTION_BUTTON_CLASS = "p-2 text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 rounded hover:bg-gray-100 dark:hover:bg-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60";

function sidebarItemClass(active: boolean) {
  return cn(SIDEBAR_ITEM_BASE_CLASS, active && SIDEBAR_ITEM_ACTIVE_CLASS);
}

interface SidebarProps {
  tables: Table[];
  selected: Table | null;
  view: View;
  isAdminView: boolean;
  onSelectTable: (table: Table) => void;
  onSelectAdminView: (view: AdminView) => void;
  onOpenCommandPalette: () => void;
  onRefresh: () => void | Promise<void>;
  onToggleTheme: () => void;
  onLogout: () => void;
  theme: "dark" | "light";
  themeToggleLabel: string;
}

export function Sidebar({
  tables,
  selected,
  view,
  isAdminView,
  onSelectTable,
  onSelectAdminView,
  onOpenCommandPalette,
  onRefresh,
  onToggleTheme,
  onLogout,
  theme,
  themeToggleLabel,
}: SidebarProps) {
  return (
    <aside className="w-60 border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900 flex flex-col">
      <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex items-center gap-2">
        <span className="text-base">👾</span>
        <span className="font-semibold text-sm text-gray-900 dark:text-gray-100">Allyourbase</span>
      </div>

      <CommandPaletteHint onClick={onOpenCommandPalette} />

      <nav className="flex-1 overflow-y-auto py-2">
        <div className="px-4 pb-1 flex items-center justify-between">
          <p className="text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase tracking-wider">
            Tables
          </p>
          <button
            onClick={() => onSelectAdminView("sql-editor")}
            className="text-[10px] text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 flex items-center gap-0.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60 rounded"
            title="New Table (opens SQL Editor)"
            aria-label="New Table"
          >
            <Plus className="w-3 h-3" />
            New Table
          </button>
        </div>

        {tables.length === 0 ? (
          <div className="px-4 py-6 text-center">
            <TableProperties className="w-8 h-8 text-gray-300 dark:text-gray-600 mx-auto mb-2" />
            <p className="text-xs text-gray-500 dark:text-gray-400 mb-1">No tables yet</p>
            <p className="text-[11px] text-gray-400 dark:text-gray-500 mb-3">
              Create your first table to get started.
            </p>
            <button
              onClick={() => onSelectAdminView("sql-editor")}
              className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/60"
            >
              Open SQL Editor
            </button>
          </div>
        ) : (
          tables.map((table) => {
            const key = `${table.schema}.${table.name}`;
            const isSelected =
              !isAdminView &&
              selected &&
              selected.schema === table.schema &&
              selected.name === table.name;

            return (
              <button
                key={key}
                onClick={() => onSelectTable(table)}
                className={cn(SIDEBAR_ITEM_BASE_CLASS, isSelected && SIDEBAR_ITEM_ACTIVE_CLASS)}
              >
                <TableIcon className={SIDEBAR_ICON_CLASS} />
                <span className="truncate">
                  {table.schema !== "public" && (
                    <span className="text-gray-400 dark:text-gray-500">{table.schema}.</span>
                  )}
                  {table.name}
                </span>
              </button>
            );
          })
        )}

        <div className={SIDEBAR_SECTION_CLASS}>
          <p className={SIDEBAR_SECTION_TITLE_CLASS}>Database</p>
          <button onClick={() => onSelectAdminView("sql-editor")} className={sidebarItemClass(view === "sql-editor")}>
            <Code className={SIDEBAR_ICON_CLASS} />
            SQL Editor
          </button>
          <button onClick={() => onSelectAdminView("functions")} className={sidebarItemClass(view === "functions")}>
            <Zap className={SIDEBAR_ICON_CLASS} />
            Functions
          </button>
          <button onClick={() => onSelectAdminView("rls")} className={sidebarItemClass(view === "rls")}>
            <Shield className={SIDEBAR_ICON_CLASS} />
            RLS Policies
          </button>
          <button onClick={() => onSelectAdminView("matviews")} className={sidebarItemClass(view === "matviews")}>
            <Layers className={SIDEBAR_ICON_CLASS} />
            Matviews
          </button>
          <button
            onClick={() => onSelectAdminView("schema-designer")}
            className={sidebarItemClass(view === "schema-designer")}
            data-testid="nav-schema-designer"
          >
            <Columns3 className={SIDEBAR_ICON_CLASS} />
            Schema Designer
          </button>
          <button onClick={() => onSelectAdminView("fdw")} className={sidebarItemClass(view === "fdw")}>
            <Cable className={SIDEBAR_ICON_CLASS} />
            FDW
          </button>
        </div>

        <div className={SIDEBAR_SECTION_CLASS}>
          <p className={SIDEBAR_SECTION_TITLE_CLASS}>Services</p>
          <button onClick={() => onSelectAdminView("storage")} className={sidebarItemClass(view === "storage")}>
            <HardDrive className={SIDEBAR_ICON_CLASS} />
            Storage
          </button>
          <button onClick={() => onSelectAdminView("sites")} className={sidebarItemClass(view === "sites")}>
            <Globe className={SIDEBAR_ICON_CLASS} />
            Sites
          </button>
          <button onClick={() => onSelectAdminView("edge-functions")} className={sidebarItemClass(view === "edge-functions")}>
            <Zap className={SIDEBAR_ICON_CLASS} />
            Edge Functions
          </button>
          <button onClick={() => onSelectAdminView("webhooks")} className={sidebarItemClass(view === "webhooks")}>
            <Webhook className={SIDEBAR_ICON_CLASS} />
            Webhooks
          </button>
        </div>

        <div className={SIDEBAR_SECTION_CLASS}>
          <p className={SIDEBAR_SECTION_TITLE_CLASS}>Messaging</p>
          <button onClick={() => onSelectAdminView("sms-health")} className={sidebarItemClass(view === "sms-health")}>
            <MessageCircle className={SIDEBAR_ICON_CLASS} />
            SMS Health
          </button>
          <button onClick={() => onSelectAdminView("sms-messages")} className={sidebarItemClass(view === "sms-messages")}>
            <MessageSquare className={SIDEBAR_ICON_CLASS} />
            SMS Messages
          </button>
          <button onClick={() => onSelectAdminView("email-templates")} className={sidebarItemClass(view === "email-templates")}>
            <Mail className={SIDEBAR_ICON_CLASS} />
            Email Templates
          </button>
          <button onClick={() => onSelectAdminView("push")} className={sidebarItemClass(view === "push")}>
            <Bell className={SIDEBAR_ICON_CLASS} />
            Push Notifications
          </button>
        </div>

        <div className={SIDEBAR_SECTION_CLASS}>
          <p className={SIDEBAR_SECTION_TITLE_CLASS}>Admin</p>
          <button onClick={() => onSelectAdminView("users")} className={sidebarItemClass(view === "users")}>
            <UsersIcon className={SIDEBAR_ICON_CLASS} />
            Users
          </button>
          <button onClick={() => onSelectAdminView("apps")} className={sidebarItemClass(view === "apps")}>
            <Box className={SIDEBAR_ICON_CLASS} />
            Apps
          </button>
          <button onClick={() => onSelectAdminView("api-keys")} className={sidebarItemClass(view === "api-keys")}>
            <KeyRound className={SIDEBAR_ICON_CLASS} />
            API Keys
          </button>
          <button onClick={() => onSelectAdminView("oauth-clients")} className={sidebarItemClass(view === "oauth-clients")}>
            <Fingerprint className={SIDEBAR_ICON_CLASS} />
            OAuth Clients
          </button>
          <button onClick={() => onSelectAdminView("api-explorer")} className={sidebarItemClass(view === "api-explorer")}>
            <Compass className={SIDEBAR_ICON_CLASS} />
            API Explorer
          </button>
          <button onClick={() => onSelectAdminView("jobs")} className={sidebarItemClass(view === "jobs")}>
            <ListTodo className={SIDEBAR_ICON_CLASS} />
            Jobs
          </button>
          <button onClick={() => onSelectAdminView("schedules")} className={sidebarItemClass(view === "schedules")}>
            <CalendarClock className={SIDEBAR_ICON_CLASS} />
            Schedules
          </button>
          <button onClick={() => onSelectAdminView("realtime-inspector")} className={sidebarItemClass(view === "realtime-inspector")}>
            <Activity className={SIDEBAR_ICON_CLASS} />
            Realtime Inspector
          </button>
          <button onClick={() => onSelectAdminView("security-advisor")} className={sidebarItemClass(view === "security-advisor")}>
            <ShieldAlert className={SIDEBAR_ICON_CLASS} />
            Security Advisor
          </button>
          <button onClick={() => onSelectAdminView("performance-advisor")} className={sidebarItemClass(view === "performance-advisor")}>
            <Gauge className={SIDEBAR_ICON_CLASS} />
            Performance Advisor
          </button>
          <button onClick={() => onSelectAdminView("backups")} className={sidebarItemClass(view === "backups")}>
            <Archive className={SIDEBAR_ICON_CLASS} />
            Backups
          </button>
          <button onClick={() => onSelectAdminView("analytics")} className={sidebarItemClass(view === "analytics")}>
            <BarChart3 className={SIDEBAR_ICON_CLASS} />
            Analytics
          </button>
          <button onClick={() => onSelectAdminView("usage")} className={sidebarItemClass(view === "usage")}>
            <LineChart className={SIDEBAR_ICON_CLASS} />
            Usage
          </button>
          <button onClick={() => onSelectAdminView("replicas")} className={sidebarItemClass(view === "replicas")}>
            <Server className={SIDEBAR_ICON_CLASS} />
            Replicas
          </button>
          <button onClick={() => onSelectAdminView("branches")} className={sidebarItemClass(view === "branches")}>
            <GitBranch className={SIDEBAR_ICON_CLASS} />
            Branches
          </button>
          <button onClick={() => onSelectAdminView("audit-logs")} className={sidebarItemClass(view === "audit-logs")}>
            <ScrollText className={SIDEBAR_ICON_CLASS} />
            Audit Logs
          </button>
          <button onClick={() => onSelectAdminView("admin-logs")} className={sidebarItemClass(view === "admin-logs")}>
            <FileText className={SIDEBAR_ICON_CLASS} />
            Admin Logs
          </button>
          <button onClick={() => onSelectAdminView("secrets")} className={sidebarItemClass(view === "secrets")}>
            <Lock className={SIDEBAR_ICON_CLASS} />
            Secrets
          </button>
          <button onClick={() => onSelectAdminView("custom-domains")} className={sidebarItemClass(view === "custom-domains")}>
            <Globe className={SIDEBAR_ICON_CLASS} />
            Custom Domains
          </button>
          <button onClick={() => onSelectAdminView("extensions")} className={sidebarItemClass(view === "extensions")}>
            <Puzzle className={SIDEBAR_ICON_CLASS} />
            Extensions
          </button>
          <button onClick={() => onSelectAdminView("vector-indexes")} className={sidebarItemClass(view === "vector-indexes")}>
            <Database className={SIDEBAR_ICON_CLASS} />
            Vector Indexes
          </button>
          <button onClick={() => onSelectAdminView("log-drains")} className={sidebarItemClass(view === "log-drains")}>
            <ArrowDownToLine className={SIDEBAR_ICON_CLASS} />
            Log Drains
          </button>
          <button onClick={() => onSelectAdminView("stats")} className={sidebarItemClass(view === "stats")}>
            <LineChart className={SIDEBAR_ICON_CLASS} />
            Stats
          </button>
          <button onClick={() => onSelectAdminView("notifications")} className={sidebarItemClass(view === "notifications")}>
            <BellRing className={SIDEBAR_ICON_CLASS} />
            Notifications
          </button>
          <button onClick={() => onSelectAdminView("incidents")} className={sidebarItemClass(view === "incidents")}>
            <AlertTriangle className={SIDEBAR_ICON_CLASS} />
            Incidents
          </button>
          <button onClick={() => onSelectAdminView("support-tickets")} className={sidebarItemClass(view === "support-tickets")}>
            <LifeBuoy className={SIDEBAR_ICON_CLASS} />
            Support Tickets
          </button>
          <button onClick={() => onSelectAdminView("tenants")} className={sidebarItemClass(view === "tenants")}>
            <Building2 className={SIDEBAR_ICON_CLASS} />
            Tenants
          </button>
          <button onClick={() => onSelectAdminView("organizations")} className={sidebarItemClass(view === "organizations")}>
            <Building2 className={SIDEBAR_ICON_CLASS} />
            Organizations
          </button>
        </div>

        <div className={SIDEBAR_SECTION_CLASS}>
          <p className={SIDEBAR_SECTION_TITLE_CLASS}>AI</p>
          <button onClick={() => onSelectAdminView("ai-assistant")} className={sidebarItemClass(view === "ai-assistant")}>
            <Sparkles className={SIDEBAR_ICON_CLASS} />
            AI Assistant
          </button>
        </div>

        <div className={SIDEBAR_SECTION_CLASS}>
          <p className={SIDEBAR_SECTION_TITLE_CLASS}>Auth</p>
          <button onClick={() => onSelectAdminView("auth-settings")} className={sidebarItemClass(view === "auth-settings")}>
            <Settings className={SIDEBAR_ICON_CLASS} />
            Auth Settings
          </button>
          <button onClick={() => onSelectAdminView("mfa-management")} className={sidebarItemClass(view === "mfa-management")}>
            <ShieldCheck className={SIDEBAR_ICON_CLASS} />
            MFA Management
          </button>
          <button onClick={() => onSelectAdminView("account-linking")} className={sidebarItemClass(view === "account-linking")}>
            <Link className={SIDEBAR_ICON_CLASS} />
            Account Linking
          </button>
          <button onClick={() => onSelectAdminView("saml")} className={sidebarItemClass(view === "saml")}>
            <ShieldPlus className={SIDEBAR_ICON_CLASS} />
            SAML
          </button>
          <button onClick={() => onSelectAdminView("auth-hooks")} className={sidebarItemClass(view === "auth-hooks")}>
            <Anchor className={SIDEBAR_ICON_CLASS} />
            Auth Hooks
          </button>
        </div>
      </nav>

      <div className="border-t border-gray-200 dark:border-gray-700 p-2 flex gap-1">
        <button
          onClick={onRefresh}
          className={SIDEBAR_ACTION_BUTTON_CLASS}
          title="Refresh schema"
          aria-label="Refresh schema"
        >
          <RefreshCw className="w-4 h-4" />
        </button>
        <button
          onClick={onToggleTheme}
          className={SIDEBAR_ACTION_BUTTON_CLASS}
          title={themeToggleLabel}
          aria-label={themeToggleLabel}
        >
          {theme === "dark" ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
        </button>
        <button
          onClick={onLogout}
          className={SIDEBAR_ACTION_BUTTON_CLASS}
          title="Log out"
          aria-label="Log out"
        >
          <LogOut className="w-4 h-4" />
        </button>
      </div>
    </aside>
  );
}
