import { useEffect, useState } from "react";
import { getAuthHooks } from "../api_auth_hooks";
import type { AuthHooksConfig } from "../types/auth_hooks";

const HOOK_LABELS: { key: keyof AuthHooksConfig; label: string }[] = [
  { key: "before_sign_up", label: "Before Sign Up" },
  { key: "after_sign_up", label: "After Sign Up" },
  { key: "custom_access_token", label: "Custom Access Token" },
  { key: "before_password_reset", label: "Before Password Reset" },
  { key: "send_email", label: "Send Email" },
  { key: "send_sms", label: "Send SMS" },
];

export function AuthHooks() {
  const [hooks, setHooks] = useState<AuthHooksConfig | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getAuthHooks()
      .then(setHooks)
      .catch((err) =>
        setError(err instanceof Error ? err.message : "Failed to load"),
      );
  }, []);

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Auth Hooks
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  if (!hooks) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Auth Hooks
        </h2>
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Auth Hooks
      </h2>

      <div className="space-y-3 max-w-lg">
        {HOOK_LABELS.map(({ key, label }) => (
          <div
            key={key}
            data-testid={`auth-hook-card-${key}`}
            className="flex items-center justify-between p-3 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded"
          >
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
              {label}
            </span>
            <span
              data-testid={`auth-hook-value-${key}`}
              className={
                hooks[key]
                  ? "text-sm font-mono text-gray-900 dark:text-gray-100"
                  : "text-sm text-gray-400 dark:text-gray-500 italic"
              }
            >
              {hooks[key] || "Not configured"}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
