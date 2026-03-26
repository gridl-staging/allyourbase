import { useState } from "react";
import { purgeStorageCDN } from "../api";
import { useAppToast } from "./ToastProvider";

export function CdnPurgeSection() {
  const [urlText, setUrlText] = useState("");
  const [purging, setPurging] = useState(false);
  const [confirmingPurgeAll, setConfirmingPurgeAll] = useState(false);
  const { addToast } = useAppToast();

  const urls = urlText
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);

  async function handlePurgeUrls() {
    setPurging(true);
    try {
      const res = await purgeStorageCDN({ urls });
      addToast("success", `Purged ${res.submitted} URLs via ${res.provider}`);
      setUrlText("");
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Purge failed");
    } finally {
      setPurging(false);
    }
  }

  async function handlePurgeAll() {
    setPurging(true);
    try {
      await purgeStorageCDN({ purgeAll: true });
      addToast("success", "Full cache purge submitted");
      setConfirmingPurgeAll(false);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Purge failed");
    } finally {
      setPurging(false);
    }
  }

  return (
    <div className="border-t px-6 py-4">
      <h3 className="text-sm font-semibold mb-3">CDN Cache Purge</h3>

      <div className="flex gap-4">
        <div className="flex-1">
          <textarea
            value={urlText}
            onChange={(e) => setUrlText(e.target.value)}
            placeholder="Enter URLs to purge, one URL per line"
            rows={3}
            className="w-full px-3 py-2 text-sm border rounded font-mono resize-y focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <button
            onClick={handlePurgeUrls}
            disabled={purging || urls.length === 0}
            className="mt-2 inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-gray-900 rounded hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {purging ? "Purging…" : "Purge URLs"}
          </button>
        </div>

        <div className="flex flex-col items-start gap-2 min-w-[140px]">
          {confirmingPurgeAll ? (
            <>
              <p className="text-xs text-red-600 font-medium">
                Are you sure? This invalidates the entire CDN cache.
              </p>
              <div className="flex gap-2">
                <button
                  onClick={handlePurgeAll}
                  disabled={purging}
                  className="px-3 py-1.5 text-xs font-medium text-white bg-red-600 rounded hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Confirm
                </button>
                <button
                  onClick={() => setConfirmingPurgeAll(false)}
                  className="px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700 rounded border"
                >
                  Cancel
                </button>
              </div>
            </>
          ) : (
            <button
              onClick={() => setConfirmingPurgeAll(true)}
              disabled={purging}
              className="px-3 py-1.5 text-xs font-medium text-red-600 border border-red-300 rounded hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Purge All
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
