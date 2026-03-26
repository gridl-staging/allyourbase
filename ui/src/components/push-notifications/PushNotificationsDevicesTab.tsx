import { Plus } from "lucide-react";
import type { PushDeviceToken } from "../../types";
import { cn } from "../../lib/utils";
import type { DeviceFilters } from "./models";
import { formatDate, previewDeviceToken, providerBadgeClass } from "./helpers";

interface PushNotificationsDevicesTabProps {
  deviceFilters: DeviceFilters;
  setDeviceFilters: React.Dispatch<React.SetStateAction<DeviceFilters>>;
  devices: PushDeviceToken[] | null;
  onApplyDeviceFilters: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  onOpenRegister: () => void;
  onRevokeDevice: (id: string) => Promise<void>;
}

export function PushNotificationsDevicesTab({
  deviceFilters,
  setDeviceFilters,
  devices,
  onApplyDeviceFilters,
  onOpenRegister,
  onRevokeDevice,
}: PushNotificationsDevicesTabProps) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <form onSubmit={onApplyDeviceFilters} className="flex items-end gap-3">
          <div>
            <label htmlFor="push-devices-app-id" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
              Filter App ID
            </label>
            <input
              id="push-devices-app-id"
              aria-label="Filter App ID"
              value={deviceFilters.app_id}
              onChange={(event) => setDeviceFilters((prev) => ({ ...prev, app_id: event.target.value }))}
              className="border rounded px-2 py-1.5 text-sm"
            />
          </div>
          <div>
            <label htmlFor="push-devices-user-id" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
              Filter User ID
            </label>
            <input
              id="push-devices-user-id"
              aria-label="Filter User ID"
              value={deviceFilters.user_id}
              onChange={(event) => setDeviceFilters((prev) => ({ ...prev, user_id: event.target.value }))}
              className="border rounded px-2 py-1.5 text-sm"
            />
          </div>
          <label className="inline-flex items-center gap-2 text-sm mb-1.5">
            <input
              type="checkbox"
              checked={deviceFilters.include_inactive}
              onChange={(event) => setDeviceFilters((prev) => ({ ...prev, include_inactive: event.target.checked }))}
            />
            Include inactive
          </label>
          <button
            type="submit"
            className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
          >
            Apply Filters
          </button>
        </form>

        <button
          onClick={onOpenRegister}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Register Device
        </button>
      </div>

      {devices && devices.length === 0 ? (
        <div className="text-center py-10 border rounded bg-gray-50 dark:bg-gray-800 text-sm text-gray-500 dark:text-gray-400">
          No push devices found
        </div>
      ) : devices ? (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Token</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Provider</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Platform</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">User</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Device Name</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Active</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Last Refreshed</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Last Used</th>
                <th className="text-right px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((item) => (
                <tr key={item.id} className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800">
                  <td className="px-4 py-2.5 font-mono text-xs text-gray-700 dark:text-gray-200">{previewDeviceToken(item.token)}</td>
                  <td className="px-4 py-2.5">
                    <span className={cn("inline-block px-2 py-0.5 rounded text-xs", providerBadgeClass(item.provider))}>
                      {item.provider}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.platform}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.user_id}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.device_name || "-"}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.is_active ? "yes" : "no"}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{formatDate(item.last_refreshed_at)}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{formatDate(item.last_used)}</td>
                  <td className="px-4 py-2.5 text-right">
                    <button
                      onClick={() => onRevokeDevice(item.id)}
                      className="px-2.5 py-1 text-xs border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
                      aria-label={`Revoke device ${item.id}`}
                    >
                      Revoke
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
