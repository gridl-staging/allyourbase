import { useState, useCallback, useEffect } from "react";
import type { SchemaCache, Table } from "../types";
import { CommandPalette } from "./CommandPalette";
import type { CommandAction } from "./CommandPalette";
import { useTheme } from "./ThemeProvider";
import { Sidebar } from "./Sidebar";
import { ContentRouter } from "./ContentRouter";
import {
  type View,
  type AdminView,
  isAdminView,
} from "./layout-types";

interface LayoutProps {
  schema: SchemaCache;
  onLogout: () => void;
  onRefresh: () => void | Promise<void>;
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
  const { theme, toggleTheme } = useTheme();

  const handleSelect = useCallback((table: Table) => {
    setSelected(table);
    setView("data");
  }, []);

  const handleAdminView = useCallback((nextView: AdminView) => {
    setSelected(null);
    setView(nextView);
  }, []);

  const handleCommand = useCallback(
    (action: CommandAction) => {
      if (action.kind === "table") {
        handleSelect(action.table);
      } else {
        handleAdminView(action.view as AdminView);
      }
    },
    [handleSelect, handleAdminView],
  );

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "k") {
        event.preventDefault();
        setCmdOpen((current) => !current);
      }
    };

    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const themeToggleLabel =
    theme === "dark" ? "Switch to light mode" : "Switch to dark mode";

  return (
    <div className="flex h-screen bg-gray-50 text-gray-900 dark:bg-gray-950 dark:text-gray-100">
      <CommandPalette
        open={cmdOpen}
        onClose={() => setCmdOpen(false)}
        onSelect={handleCommand}
        tables={tables}
      />

      <Sidebar
        tables={tables}
        selected={selected}
        view={view}
        isAdminView={isAdminView(view)}
        onSelectTable={handleSelect}
        onSelectAdminView={handleAdminView}
        onOpenCommandPalette={() => setCmdOpen(true)}
        onRefresh={onRefresh}
        onToggleTheme={toggleTheme}
        onLogout={onLogout}
        theme={theme as "dark" | "light"}
        themeToggleLabel={themeToggleLabel}
      />

      <ContentRouter
        schema={schema}
        view={view}
        isAdminView={isAdminView(view)}
        selected={selected}
        onRefresh={onRefresh}
        onSetView={setView}
        onSelectAdminView={handleAdminView}
      />
    </div>
  );
}
