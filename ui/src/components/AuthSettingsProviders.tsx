import type { OAuthProviderInfo, TestProviderResult } from "../types";
import {
  PROVIDER_SETUP_INFO,
  type ProviderFormState,
} from "./auth-settings-helpers";

interface AuthSettingsProvidersProps {
  providers: OAuthProviderInfo[];
  providerError: string | null;
  providerSuccess: string | null;
  providerFormError: string | null;
  providerEditingName: string | null;
  providerForm: ProviderFormState | null;
  providerSaving: boolean;
  providerTesting: boolean;
  testResult: TestProviderResult | null;
  onOpenProviderForm: (provider: OAuthProviderInfo) => void;
  onCloseProviderForm: () => void;
  onProviderFieldChange: <K extends keyof ProviderFormState>(
    key: K,
    value: ProviderFormState[K],
  ) => void;
  onProviderSave: () => void;
  onTestConnection: () => void;
  onRequestProviderDelete: (provider: OAuthProviderInfo) => void;
}

export function AuthSettingsProviders({
  providers,
  providerError,
  providerSuccess,
  providerFormError,
  providerEditingName,
  providerForm,
  providerSaving,
  providerTesting,
  testResult,
  onOpenProviderForm,
  onCloseProviderForm,
  onProviderFieldChange,
  onProviderSave,
  onTestConnection,
  onRequestProviderDelete,
}: AuthSettingsProvidersProps) {
  return (
    <div className="space-y-3 pt-2 border-t">
      <h3 className="text-base font-semibold">OAuth Providers</h3>
      {providerError && (
        <div className="px-4 py-2 bg-yellow-50 border border-yellow-200 rounded-lg text-yellow-900 text-sm">
          {providerError}
        </div>
      )}
      {providerSuccess && (
        <div className="px-4 py-2 bg-green-50 border border-green-200 rounded-lg text-green-800 text-sm">
          {providerSuccess}
        </div>
      )}

      {providers.length === 0 ? (
        !providerError && <p className="text-sm text-gray-500 dark:text-gray-400">No OAuth providers configured.</p>
      ) : (
        <div className="space-y-2">
          {providers.map((provider) => (
            <div key={provider.name} className="space-y-2">
              <div
                data-testid={`provider-row-${provider.name}`}
                className="rounded-lg border px-4 py-3 flex items-center justify-between gap-2"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900 dark:text-gray-100">{provider.name}</p>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {provider.type === "oidc" ? "OIDC" : "Built-in"}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-2 text-xs">
                    <span
                      data-testid={`provider-enabled-${provider.name}`}
                      className={provider.enabled ? "text-green-700" : "text-gray-600 dark:text-gray-300"}
                    >
                      {provider.enabled ? "Enabled" : "Disabled"}
                    </span>
                    <span className="text-gray-300 dark:text-gray-500">|</span>
                    <span
                      data-testid={`provider-client-${provider.name}`}
                      className={provider.client_id_configured ? "text-green-700" : "text-orange-700"}
                    >
                      {provider.client_id_configured ? "Client ID configured" : "Client ID missing"}
                    </span>
                  </div>
                  <button
                    type="button"
                    data-testid={`provider-edit-${provider.name}`}
                    onClick={() => onOpenProviderForm(provider)}
                    className="text-xs px-2 py-1 rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
                  >
                    {provider.client_id_configured ? "Edit" : "Configure"}
                  </button>
                  {provider.type === "oidc" && (
                    <button
                      type="button"
                      data-testid={`provider-delete-${provider.name}`}
                      onClick={() => onRequestProviderDelete(provider)}
                      className="text-xs px-2 py-1 rounded border border-red-300 text-red-700 hover:bg-red-50 dark:border-red-700 dark:text-red-400 dark:hover:bg-red-900/20"
                    >
                      Delete
                    </button>
                  )}
                </div>
              </div>

              {providerEditingName === provider.name && providerForm && (
                <div className="rounded-lg border border-blue-200 bg-blue-50/40 px-4 py-3 space-y-3">
                  {providerFormError && (
                    <div className="px-3 py-2 bg-red-50 border border-red-200 rounded text-red-700 text-xs">
                      {providerFormError}
                    </div>
                  )}
                  {testResult && (
                    <div
                      className={`px-3 py-2 rounded text-xs ${
                        testResult.success
                          ? "bg-green-50 border border-green-200 text-green-700"
                          : "bg-red-50 border border-red-200 text-red-700"
                      }`}
                    >
                      {testResult.success ? testResult.message : testResult.error}
                    </div>
                  )}

                  {provider.type === "builtin" && PROVIDER_SETUP_INFO[provider.name] && (
                    <div data-testid="provider-setup-instructions" className="text-xs text-gray-600 dark:text-gray-300 space-y-1">
                      <p>
                        Set up credentials at{" "}
                        <a
                          href={PROVIDER_SETUP_INFO[provider.name].consoleUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-blue-600 hover:underline"
                        >
                          {PROVIDER_SETUP_INFO[provider.name].consoleLabel} ({PROVIDER_SETUP_INFO[provider.name].consoleUrl})
                        </a>
                      </p>
                      <p className="font-mono text-gray-500 dark:text-gray-400">
                        Redirect URI: <code>{`{your-domain}/oauth/${provider.name}/callback`}</code>
                      </p>
                    </div>
                  )}

                  <label className="flex items-center gap-2 text-sm text-gray-800 dark:text-gray-200">
                    <input
                      type="checkbox"
                      data-testid="provider-form-enabled"
                      checked={providerForm.enabled}
                      onChange={(event) => onProviderFieldChange("enabled", event.target.checked)}
                    />
                    Enable provider
                  </label>

                  <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                    <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                      <span>Client ID</span>
                      <input
                        type="text"
                        data-testid="provider-form-client-id"
                        value={providerForm.client_id}
                        onChange={(event) => onProviderFieldChange("client_id", event.target.value)}
                        className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                      />
                    </label>

                    {provider.name !== "apple" && (
                      <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                        <span>Client Secret</span>
                        <input
                          type="password"
                          data-testid="provider-form-client-secret"
                          value={providerForm.client_secret}
                          onChange={(event) => onProviderFieldChange("client_secret", event.target.value)}
                          className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                        />
                      </label>
                    )}

                    {provider.name === "microsoft" && (
                      <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                        <span>Tenant ID</span>
                        <input
                          type="text"
                          data-testid="provider-form-tenant-id"
                          value={providerForm.tenant_id}
                          onChange={(event) => onProviderFieldChange("tenant_id", event.target.value)}
                          className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                        />
                      </label>
                    )}

                    {provider.name === "apple" && (
                      <>
                        <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                          <span>Apple Team ID</span>
                          <input
                            type="text"
                            data-testid="provider-form-team-id"
                            value={providerForm.team_id}
                            onChange={(event) => onProviderFieldChange("team_id", event.target.value)}
                            className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                          />
                        </label>
                        <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                          <span>Apple Key ID</span>
                          <input
                            type="text"
                            data-testid="provider-form-key-id"
                            value={providerForm.key_id}
                            onChange={(event) => onProviderFieldChange("key_id", event.target.value)}
                            className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                          />
                        </label>
                        <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1 md:col-span-2">
                          <span>Apple Private Key</span>
                          <textarea
                            data-testid="provider-form-private-key"
                            value={providerForm.private_key}
                            onChange={(event) => onProviderFieldChange("private_key", event.target.value)}
                            className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm min-h-24"
                          />
                        </label>
                      </>
                    )}

                    {provider.type === "oidc" && (
                      <>
                        <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1 md:col-span-2">
                          <span>Issuer URL</span>
                          <input
                            type="text"
                            data-testid="provider-form-issuer-url"
                            value={providerForm.issuer_url}
                            onChange={(event) => onProviderFieldChange("issuer_url", event.target.value)}
                            className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                          />
                        </label>
                        <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                          <span>Display Name (optional)</span>
                          <input
                            type="text"
                            data-testid="provider-form-display-name"
                            value={providerForm.display_name}
                            onChange={(event) => onProviderFieldChange("display_name", event.target.value)}
                            className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                          />
                        </label>
                        <label className="text-xs text-gray-700 dark:text-gray-200 space-y-1">
                          <span>Scopes (space-separated)</span>
                          <input
                            type="text"
                            data-testid="provider-form-scopes"
                            value={providerForm.scopes}
                            onChange={(event) => onProviderFieldChange("scopes", event.target.value)}
                            className="w-full rounded border border-gray-300 dark:border-gray-600 px-2 py-1.5 text-sm"
                          />
                        </label>
                      </>
                    )}
                  </div>

                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      data-testid="provider-form-save"
                      onClick={onProviderSave}
                      disabled={providerSaving}
                      className="px-3 py-1.5 rounded bg-blue-600 text-white text-xs hover:bg-blue-700 disabled:opacity-70"
                    >
                      {providerSaving ? "Saving..." : "Save Provider"}
                    </button>
                    <button
                      type="button"
                      data-testid="provider-form-test"
                      onClick={onTestConnection}
                      disabled={providerTesting}
                      className="px-3 py-1.5 rounded border border-gray-300 dark:border-gray-600 text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800 disabled:opacity-70"
                    >
                      {providerTesting ? "Testing..." : "Test Connection"}
                    </button>
                    <button
                      type="button"
                      data-testid="provider-form-cancel"
                      onClick={onCloseProviderForm}
                      disabled={providerSaving}
                      className="px-3 py-1.5 rounded border border-gray-300 dark:border-gray-600 text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
