import type { RegisterFormState } from "./models";

interface PushNotificationsRegisterModalProps {
  open: boolean;
  registering: boolean;
  registerForm: RegisterFormState;
  setRegisterForm: React.Dispatch<React.SetStateAction<RegisterFormState>>;
  onClose: () => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function PushNotificationsRegisterModal({
  open,
  registering,
  registerForm,
  setRegisterForm,
  onClose,
  onSubmit,
}: PushNotificationsRegisterModalProps) {
  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
      <div className="w-full max-w-lg rounded-lg bg-white dark:bg-gray-800 border shadow-lg p-4">
        <h2 className="text-base font-semibold mb-3">Register Device</h2>
        <form onSubmit={onSubmit} className="space-y-3">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label htmlFor="push-register-app-id" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                App ID
              </label>
              <input
                id="push-register-app-id"
                aria-label="App ID"
                value={registerForm.app_id}
                onChange={(event) => setRegisterForm((prev) => ({ ...prev, app_id: event.target.value }))}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label htmlFor="push-register-user-id" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                User ID
              </label>
              <input
                id="push-register-user-id"
                aria-label="User ID"
                value={registerForm.user_id}
                onChange={(event) => setRegisterForm((prev) => ({ ...prev, user_id: event.target.value }))}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label htmlFor="push-register-provider" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                Provider
              </label>
              <select
                id="push-register-provider"
                aria-label="Provider"
                value={registerForm.provider}
                onChange={(event) =>
                  setRegisterForm((prev) => ({
                    ...prev,
                    provider: event.target.value as "fcm" | "apns",
                  }))
                }
                className="w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-800"
              >
                <option value="fcm">fcm</option>
                <option value="apns">apns</option>
              </select>
            </div>
            <div>
              <label htmlFor="push-register-platform" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                Platform
              </label>
              <select
                id="push-register-platform"
                aria-label="Platform"
                value={registerForm.platform}
                onChange={(event) =>
                  setRegisterForm((prev) => ({
                    ...prev,
                    platform: event.target.value as "android" | "ios",
                  }))
                }
                className="w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-800"
              >
                <option value="android">android</option>
                <option value="ios">ios</option>
              </select>
            </div>
          </div>

          <div>
            <label htmlFor="push-register-token" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Token
            </label>
            <textarea
              id="push-register-token"
              aria-label="Token"
              value={registerForm.token}
              onChange={(event) => setRegisterForm((prev) => ({ ...prev, token: event.target.value }))}
              rows={3}
              className="w-full border rounded px-3 py-2 text-sm font-mono"
            />
          </div>

          <div>
            <label htmlFor="push-register-device-name" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Device Name
            </label>
            <input
              id="push-register-device-name"
              aria-label="Device Name"
              value={registerForm.device_name}
              onChange={(event) => setRegisterForm((prev) => ({ ...prev, device_name: event.target.value }))}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>

          <div className="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={registering}
              className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-70"
            >
              Save Device
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
