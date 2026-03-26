import type { AuthSettings as AuthSettingsType } from "../types";

const TOGGLE_FIELDS: { key: keyof AuthSettingsType; label: string }[] = [
  { key: "totp_enabled", label: "TOTP MFA" },
  { key: "anonymous_auth_enabled", label: "Anonymous Auth" },
  { key: "email_mfa_enabled", label: "Email MFA" },
  { key: "sms_enabled", label: "SMS Auth" },
  { key: "magic_link_enabled", label: "Magic Link" },
];

interface AuthSettingsTogglesProps {
  settings: AuthSettingsType;
  onToggle: (key: keyof AuthSettingsType) => void;
}

export function AuthSettingsToggles({ settings, onToggle }: AuthSettingsTogglesProps) {
  return (
    <div className="space-y-3">
      {TOGGLE_FIELDS.map(({ key, label }) => (
        <label key={key} className="flex items-center justify-between p-3 border rounded-lg">
          <span className="text-sm font-medium text-gray-700 dark:text-gray-200">{label}</span>
          <input
            type="checkbox"
            data-testid={`toggle-${key}`}
            checked={settings[key]}
            onChange={() => onToggle(key)}
            className="h-4 w-4 rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
          />
        </label>
      ))}
    </div>
  );
}
