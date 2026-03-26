import { useState, useEffect, useCallback, useRef } from "react";
import type {
  DBTriggerResponse,
  CronTriggerResponse,
  StorageTriggerResponse,
} from "../types";
import {
  listDBTriggers,
  listCronTriggers,
  listStorageTriggers,
} from "../api";
import { cn } from "../lib/utils";
import type { TriggerTab } from "./EdgeFunctionTriggersShared";
import {
  normalizeDBTrigger,
  normalizeDBTriggers,
  normalizeCronTrigger,
  normalizeCronTriggers,
  normalizeStorageTrigger,
  normalizeStorageTriggers,
  upsertByID,
} from "./EdgeFunctionTriggersShared";
import { DBTriggerPanel } from "./EdgeFunctionTriggersDB";
import { CronTriggerPanel } from "./EdgeFunctionTriggersCron";
import { StorageTriggerPanel } from "./EdgeFunctionTriggersStorage";

interface Props {
  functionId: string;
  addToast: (type: "success" | "error", message: string) => void;
}

export function EdgeFunctionTriggers({ functionId, addToast }: Props) {
  const [tab, setTab] = useState<TriggerTab>("db");
  const refreshSeqRef = useRef(0);

  // DB triggers
  const [dbTriggers, setDbTriggers] = useState<DBTriggerResponse[]>([]);
  const [dbLoading, setDbLoading] = useState(true);

  // Cron triggers
  const [cronTriggers, setCronTriggers] = useState<CronTriggerResponse[]>([]);
  const [cronLoading, setCronLoading] = useState(true);

  // Storage triggers
  const [storageTriggers, setStorageTriggers] = useState<StorageTriggerResponse[]>([]);
  const [storageLoading, setStorageLoading] = useState(true);

  const fetchAll = useCallback(() => {
    const refreshSeq = ++refreshSeqRef.current;

    setDbLoading(true);
    setCronLoading(true);
    setStorageLoading(true);

    listDBTriggers(functionId)
      .then((data) => {
        if (refreshSeqRef.current !== refreshSeq) return;
        setDbTriggers(normalizeDBTriggers(data));
      })
      .catch(() => {
        if (refreshSeqRef.current !== refreshSeq) return;
        addToast("error", "Failed to load database triggers");
      })
      .finally(() => {
        if (refreshSeqRef.current !== refreshSeq) return;
        setDbLoading(false);
      });

    listCronTriggers(functionId)
      .then((data) => {
        if (refreshSeqRef.current !== refreshSeq) return;
        setCronTriggers(normalizeCronTriggers(data));
      })
      .catch(() => {
        if (refreshSeqRef.current !== refreshSeq) return;
        addToast("error", "Failed to load cron triggers");
      })
      .finally(() => {
        if (refreshSeqRef.current !== refreshSeq) return;
        setCronLoading(false);
      });

    listStorageTriggers(functionId)
      .then((data) => {
        if (refreshSeqRef.current !== refreshSeq) return;
        setStorageTriggers(normalizeStorageTriggers(data));
      })
      .catch(() => {
        if (refreshSeqRef.current !== refreshSeq) return;
        addToast("error", "Failed to load storage triggers");
      })
      .finally(() => {
        if (refreshSeqRef.current !== refreshSeq) return;
        setStorageLoading(false);
      });
  }, [functionId, addToast]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const handleDBCreated = useCallback((raw: unknown) => {
    setDbTriggers((prev) => upsertByID(prev, normalizeDBTrigger(raw)));
  }, []);

  const handleCronCreated = useCallback((raw: unknown) => {
    setCronTriggers((prev) => upsertByID(prev, normalizeCronTrigger(raw)));
  }, []);

  const handleStorageCreated = useCallback((raw: unknown) => {
    setStorageTriggers((prev) => upsertByID(prev, normalizeStorageTrigger(raw)));
  }, []);

  return (
    <div>
      {/* Tabs */}
      <div className="flex gap-1 bg-gray-100 dark:bg-gray-700 rounded p-0.5 mb-4 w-fit">
        {([
          { key: "db" as const, label: "Database" },
          { key: "cron" as const, label: "Cron" },
          { key: "storage" as const, label: "Storage" },
        ]).map((t) => (
          <button
            key={t.key}
            data-testid={`trigger-tab-${t.key}`}
            data-active={tab === t.key ? "true" : "false"}
            onClick={() => setTab(t.key)}
            className={cn(
              "px-3 py-1 text-xs rounded font-medium",
              tab === t.key
                ? "bg-white dark:bg-gray-800 shadow-sm text-gray-900 dark:text-gray-100"
                : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 dark:text-gray-200",
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "db" && (
        <DBTriggerPanel
          functionId={functionId}
          triggers={dbTriggers}
          loading={dbLoading}
          onRefresh={fetchAll}
          onCreateSuccess={handleDBCreated}
          addToast={addToast}
        />
      )}

      {tab === "cron" && (
        <CronTriggerPanel
          functionId={functionId}
          triggers={cronTriggers}
          loading={cronLoading}
          onRefresh={fetchAll}
          onCreateSuccess={handleCronCreated}
          addToast={addToast}
        />
      )}

      {tab === "storage" && (
        <StorageTriggerPanel
          functionId={functionId}
          triggers={storageTriggers}
          loading={storageLoading}
          onRefresh={fetchAll}
          onCreateSuccess={handleStorageCreated}
          addToast={addToast}
        />
      )}
    </div>
  );
}
