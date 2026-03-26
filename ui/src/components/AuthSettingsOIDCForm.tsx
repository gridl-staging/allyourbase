import type { OIDCFormState } from "./auth-settings-helpers";

interface AuthSettingsOIDCFormProps {
  showOIDCForm: boolean;
  oidcForm: OIDCFormState;
  oidcFormError: string | null;
  oidcSaving: boolean;
  onShowForm: () => void;
  onCancel: () => void;
  onFieldChange: <K extends keyof OIDCFormState>(key: K, value: string) => void;
  onSave: () => void;
}

export function AuthSettingsOIDCForm({
  showOIDCForm,
  oidcForm,
  oidcFormError,
  oidcSaving,
  onShowForm,
  onCancel,
  onFieldChange,
  onSave,
}: AuthSettingsOIDCFormProps) {
  return (
    <div className="pt-2">
      {!showOIDCForm ? (
        <button
          type="button"
          data-testid="add-oidc-provider"
          onClick={onShowForm}
          className="text-xs px-3 py-1.5 rounded border border-dashed border-gray-400 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
        >
          + Add OIDC Provider
        </button>
      ) : (
        <div className="rounded-lg border border-purple-200 bg-purple-50/40 px-4 py-3 space-y-3">
          <h4 className="text-sm font-medium text-gray-800 dark:text-gray-200">Add Custom OIDC Provider</h4>
          {oidcFormError && (
            <div className="px-3 py-2 bg-red-50 border border-red-200 rounded text-red-700 text-xs">
              {oidcFormError}
            </div>
          )}

          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
              <span>Provider Name</span>
              <input
                type="text"
                data-testid="oidc-form-provider-name"
                value={oidcForm.provider_name}
                onChange={(event) => onFieldChange("provider_name", event.target.value)}
                placeholder="e.g. my-keycloak"
                className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
              />
            </label>

            <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
              <span>Issuer URL</span>
              <input
                type="text"
                data-testid="oidc-form-issuer-url"
                value={oidcForm.issuer_url}
                onChange={(event) => onFieldChange("issuer_url", event.target.value)}
                placeholder="https://auth.example.com/realms/main"
                className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
              />
            </label>

            <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
              <span>Client ID</span>
              <input
                type="text"
                data-testid="oidc-form-client-id"
                value={oidcForm.client_id}
                onChange={(event) => onFieldChange("client_id", event.target.value)}
                className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
              />
            </label>

            <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
              <span>Client Secret</span>
              <input
                type="password"
                data-testid="oidc-form-client-secret"
                value={oidcForm.client_secret}
                onChange={(event) => onFieldChange("client_secret", event.target.value)}
                className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
              />
            </label>

            <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
              <span>Display Name</span>
              <input
                type="text"
                data-testid="oidc-form-display-name"
                value={oidcForm.display_name}
                onChange={(event) => onFieldChange("display_name", event.target.value)}
                placeholder="e.g. Keycloak"
                className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
              />
            </label>

            <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
              <span>Scopes</span>
              <input
                type="text"
                data-testid="oidc-form-scopes"
                value={oidcForm.scopes}
                onChange={(event) => onFieldChange("scopes", event.target.value)}
                placeholder="openid profile email"
                className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
              />
            </label>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              data-testid="oidc-form-save"
              onClick={onSave}
              disabled={oidcSaving}
              className="px-3 py-1.5 rounded bg-purple-600 text-white text-xs hover:bg-purple-700 disabled:opacity-70"
            >
              {oidcSaving ? "Adding..." : "Add Provider"}
            </button>
            <button
              type="button"
              data-testid="oidc-form-cancel"
              onClick={onCancel}
              disabled={oidcSaving}
              className="px-3 py-1.5 rounded border border-gray-300 dark:border-gray-600 text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
