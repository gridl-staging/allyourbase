import { useCallback, useEffect, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import type { CallLogListResponse, DailyUsage, UsageSummary, Prompt } from "../types/ai";
import { deletePrompt, getAIUsage, getDailyUsage, listAILogs, listPrompts } from "../api_ai";
import { useDraftFilters } from "../hooks/useDraftFilters";
import { useAppToast } from "./ToastProvider";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { AILogsTab } from "./ai/AILogsTab";
import { AIUsageTab } from "./ai/AIUsageTab";
import { AIAssistantTab } from "./ai/AIAssistantTab";
import { AIPromptsTab } from "./ai/AIPromptsTab";
import { cn } from "../lib/utils";

type Tab = "logs" | "usage" | "assistant" | "prompts";

const TAB_CLASS =
  "px-4 py-2 text-sm font-medium rounded-t border-b-2 transition-colors";
const TAB_ACTIVE =
  "border-blue-600 text-blue-600 dark:text-blue-400 dark:border-blue-400";
const TAB_INACTIVE =
  "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400";

const TABS: { value: Tab; label: string }[] = [
  { value: "logs", label: "Logs" },
  { value: "usage", label: "Usage" },
  { value: "assistant", label: "Assistant" },
  { value: "prompts", label: "Prompts" },
];

const INITIAL_LOG_FILTER_VALUES = {
  provider: "",
  status: "",
  from: "",
  to: "",
};

function normalizeDateFilter(value: string, boundary: "start" | "end"): string | undefined {
  if (!value) return undefined;
  const [year, month, day] = value.split("-").map((part) => Number.parseInt(part, 10));
  if ([year, month, day].some(Number.isNaN)) {
    return undefined;
  }

  const localDate = boundary === "start"
    ? new Date(year, month - 1, day, 0, 0, 0, 0)
    : new Date(year, month - 1, day, 23, 59, 59, 999);

  return localDate.toISOString();
}

export function AIAssistant() {
  const [tab, setTab] = useState<Tab>("logs");
  const [logsData, setLogsData] = useState<CallLogListResponse | null>(null);
  const [usageData, setUsageData] = useState<UsageSummary | null>(null);
  const [dailyUsageData, setDailyUsageData] = useState<DailyUsage[]>([]);
  const [promptsData, setPromptsData] = useState<Prompt[]>([]);
  const [loading, setLoading] = useState(true);
  const [errorByTab, setErrorByTab] = useState<Record<Tab, string | null>>({
    logs: null,
    usage: null,
    assistant: null,
    prompts: null,
  });
  const [deleteTarget, setDeleteTarget] = useState<Prompt | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const {
    draftValues: draftFilterValues,
    appliedValues: appliedFilterValues,
    setDraftValue: setDraftFilterValue,
    applyValues: applyFilterValues,
    resetValues: resetFilterValues,
  } = useDraftFilters(INITIAL_LOG_FILTER_VALUES);

  const { addToast } = useAppToast();

  const loadLogs = useCallback(async (filters: Record<string, string>) => {
    try {
      setErrorByTab((prev) => ({ ...prev, logs: null }));
      const result = await listAILogs({
        provider: filters.provider || undefined,
        status: filters.status || undefined,
        from: normalizeDateFilter(filters.from ?? "", "start"),
        to: normalizeDateFilter(filters.to ?? "", "end"),
      });
      setLogsData(result);
    } catch (e) {
      setErrorByTab((prev) => ({
        ...prev,
        logs: e instanceof Error ? e.message : "Failed to load AI logs",
      }));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadUsage = useCallback(async () => {
    try {
      setErrorByTab((prev) => ({ ...prev, usage: null }));
      const [usageSummary, dailyUsage] = await Promise.all([
        getAIUsage(),
        getDailyUsage(),
      ]);
      setUsageData(usageSummary);
      setDailyUsageData(dailyUsage);
    } catch (e) {
      setErrorByTab((prev) => ({
        ...prev,
        usage: e instanceof Error ? e.message : "Failed to load usage",
      }));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadPrompts = useCallback(async () => {
    try {
      setErrorByTab((prev) => ({ ...prev, prompts: null }));
      const result = await listPrompts();
      setPromptsData(result.prompts);
    } catch (e) {
      setErrorByTab((prev) => ({
        ...prev,
        prompts: e instanceof Error ? e.message : "Failed to load prompts",
      }));
    } finally {
      setLoading(false);
    }
  }, []);

  const retryActiveTab = useCallback(() => {
    setLoading(true);
    if (tab === "logs") {
      void loadLogs(appliedFilterValues);
      return;
    }
    if (tab === "usage") {
      void loadUsage();
      return;
    }
    if (tab === "prompts") {
      void loadPrompts();
      return;
    }
    setLoading(false);
  }, [appliedFilterValues, loadLogs, loadPrompts, loadUsage, tab]);

  useEffect(() => {
    retryActiveTab();
  }, [retryActiveTab]);

  const handleDeletePrompt = async () => {
    if (!deleteTarget) return;
    setDeleting(deleteTarget.id);
    try {
      await deletePrompt(deleteTarget.id);
      addToast("success", `Prompt "${deleteTarget.name}" deleted`);
      setDeleteTarget(null);
      await loadPrompts();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to delete prompt");
    } finally {
      setDeleting(null);
    }
  };

  if (loading && !logsData && !usageData && promptsData.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading AI data...
      </div>
    );
  }

  const activeTabError = errorByTab[tab];
  if (activeTabError) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{activeTabError}</p>
          <button
            onClick={retryActiveTab}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">AI Assistant</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          AI logs, usage analytics, interactive assistant, and prompt management
        </p>
      </div>

        <div className="flex gap-1 mb-4 border-b">
        {TABS.map((t) => (
          <button
            key={t.value}
            onClick={() => { setTab(t.value); setLoading(true); }}
            className={cn(TAB_CLASS, tab === t.value ? TAB_ACTIVE : TAB_INACTIVE)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "logs" && (
        <AILogsTab
          data={logsData}
          filterValues={draftFilterValues}
          onFilterChange={setDraftFilterValue}
          onApply={(vals) => {
            setLoading(true);
            applyFilterValues(vals);
          }}
          onReset={() => {
            resetFilterValues();
            setLoading(true);
          }}
        />
      )}

      {tab === "usage" && <AIUsageTab usage={usageData} dailyUsage={dailyUsageData} />}

      {tab === "assistant" && <AIAssistantTab />}

      {tab === "prompts" && (
        <AIPromptsTab
          prompts={promptsData}
          deleting={deleting}
          onPromptsChanged={loadPrompts}
          onDelete={setDeleteTarget}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Prompt"
        message={deleteTarget ? `Delete prompt "${deleteTarget.name}"? This action cannot be undone.` : ""}
        confirmLabel="Delete"
        destructive
        loading={deleting !== null}
        onConfirm={handleDeletePrompt}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
