import { useState, useEffect, useCallback } from "react";
import type {
  AuthSettings as AuthSettingsType,
  OAuthProviderInfo,
  TestProviderResult,
} from "../types";
import {
  deleteAuthProvider,
  getAuthProviders,
  getAuthSettings,
  updateAuthProvider,
  updateAuthSettings,
  testAuthProvider,
} from "../api";
import { Loader2, AlertCircle } from "lucide-react";
import {
  buildOIDCUpdatePayload,
  buildProviderUpdatePayload,
  createOIDCForm,
  createProviderForm,
  type OIDCFormState,
  type ProviderFormState,
} from "./auth-settings-helpers";
import { AuthSettingsToggles } from "./AuthSettingsToggles";
import { AuthSettingsProviders } from "./AuthSettingsProviders";
import { AuthSettingsOIDCForm } from "./AuthSettingsOIDCForm";
import { ConfirmDialog } from "./shared/ConfirmDialog";

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function AuthSettings() {
  const [settings, setSettings] = useState<AuthSettingsType | null>(null);
  const [providers, setProviders] = useState<OAuthProviderInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [providerError, setProviderError] = useState<string | null>(null);
  const [providerSuccess, setProviderSuccess] = useState<string | null>(null);
  const [providerFormError, setProviderFormError] = useState<string | null>(null);
  const [providerEditingName, setProviderEditingName] = useState<string | null>(null);
  const [providerForm, setProviderForm] = useState<ProviderFormState | null>(null);
  const [providerSaving, setProviderSaving] = useState(false);
  const [providerTesting, setProviderTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showOIDCForm, setShowOIDCForm] = useState(false);
  const [oidcForm, setOIDCForm] = useState<OIDCFormState>(createOIDCForm);
  const [oidcFormError, setOIDCFormError] = useState<string | null>(null);
  const [oidcSaving, setOIDCSaving] = useState(false);
  const [pendingDeleteProvider, setPendingDeleteProvider] = useState<OAuthProviderInfo | null>(null);
  const [providerDeleting, setProviderDeleting] = useState(false);

  const reloadProviders = useCallback(async (): Promise<boolean> => {
    try {
      const providersRes = await getAuthProviders();
      setProviderError(null);
      setProviders(providersRes.providers);
      return true;
    } catch (requestError) {
      setProviderError(getErrorMessage(requestError, "Failed to load OAuth providers"));
      return false;
    }
  }, []);

  const fetchSettings = useCallback(async () => {
    try {
      setError(null);
      setProviderError(null);
      setLoading(true);
      const [settingsRes, providersRes] = await Promise.all([
        getAuthSettings(),
        getAuthProviders().catch((requestError) => {
          setProviderError(getErrorMessage(requestError, "Failed to load OAuth providers"));
          return { providers: [] };
        }),
      ]);
      setSettings(settingsRes);
      setProviders(providersRes.providers);
    } catch (requestError) {
      setError(getErrorMessage(requestError, "Failed to load auth settings"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  const handleToggle = async (key: keyof AuthSettingsType) => {
    if (!settings) return;
    setError(null);
    setSuccess(null);
    const updated = { ...settings, [key]: !settings[key] };
    try {
      const res = await updateAuthSettings(updated);
      setSettings(res);
      setSuccess("Settings updated");
    } catch (requestError) {
      setError(getErrorMessage(requestError, "Failed to update settings"));
    }
  };

  const openProviderForm = (provider: OAuthProviderInfo) => {
    setProviderEditingName(provider.name);
    setProviderForm(createProviderForm(provider));
    setProviderFormError(null);
    setProviderSuccess(null);
    setTestResult(null);
  };

  const closeProviderForm = () => {
    setProviderEditingName(null);
    setProviderForm(null);
    setProviderFormError(null);
    setTestResult(null);
  };

  const handleProviderFieldChange = <K extends keyof ProviderFormState>(
    key: K,
    value: ProviderFormState[K],
  ) => {
    setProviderForm((current) => {
      if (!current) return current;
      return { ...current, [key]: value };
    });
  };

  const handleProviderSave = async () => {
    if (!providerEditingName || !providerForm) return;
    setProviderFormError(null);
    setProviderSuccess(null);
    setProviderSaving(true);
    try {
      const payload = buildProviderUpdatePayload(providerEditingName, providerForm);
      const updatedProvider = await updateAuthProvider(providerEditingName, payload);
      setProviders((current) =>
        current.map((provider) =>
          provider.name === updatedProvider.name ? updatedProvider : provider,
        ),
      );
      closeProviderForm();
      setProviderSuccess(`Provider "${updatedProvider.name}" updated.`);
    } catch (requestError) {
      setProviderFormError(getErrorMessage(requestError, "Failed to update OAuth provider"));
    } finally {
      setProviderSaving(false);
    }
  };

  const handleTestConnection = async () => {
    if (!providerEditingName) return;
    setTestResult(null);
    setProviderTesting(true);
    try {
      const result = await testAuthProvider(providerEditingName);
      setTestResult(result);
    } catch (requestError) {
      setTestResult({
        success: false,
        provider: providerEditingName,
        error: getErrorMessage(requestError, "Test connection failed"),
      });
    } finally {
      setProviderTesting(false);
    }
  };

  const handleOIDCFieldChange = <K extends keyof OIDCFormState>(
    key: K,
    value: string,
  ) => {
    setOIDCForm((current) => ({ ...current, [key]: value }));
  };

  const openOIDCForm = () => {
    setShowOIDCForm(true);
    setOIDCForm(createOIDCForm());
    setOIDCFormError(null);
  };

  const closeOIDCForm = () => {
    setShowOIDCForm(false);
    setOIDCFormError(null);
  };

  const handleOIDCSave = async () => {
    setOIDCFormError(null);
    const name = oidcForm.provider_name.trim();
    if (!name) {
      setOIDCFormError("Provider name is required");
      return;
    }
    setOIDCSaving(true);
    try {
      const payload = buildOIDCUpdatePayload(oidcForm);
      await updateAuthProvider(name, payload);
      setShowOIDCForm(false);
      setOIDCForm(createOIDCForm());
      const providersReloaded = await reloadProviders();
      if (!providersReloaded) {
        return;
      }
      setProviderSuccess(`OIDC provider "${name}" added.`);
    } catch (requestError) {
      setOIDCFormError(getErrorMessage(requestError, "Failed to add OIDC provider"));
    } finally {
      setOIDCSaving(false);
    }
  };

  const requestProviderDelete = (provider: OAuthProviderInfo) => {
    setPendingDeleteProvider(provider);
  };

  const cancelProviderDelete = () => {
    setPendingDeleteProvider(null);
  };

  const confirmProviderDelete = async () => {
    if (!pendingDeleteProvider) return;
    const name = pendingDeleteProvider.name;
    setProviderDeleting(true);
    setProviderError(null);
    setProviderSuccess(null);
    try {
      await deleteAuthProvider(name);
      setPendingDeleteProvider(null);
      if (providerEditingName === name) {
        closeProviderForm();
      }
      const providersReloaded = await reloadProviders();
      if (!providersReloaded) {
        return;
      }
      setProviderSuccess(`Provider "${name}" deleted.`);
    } catch (requestError) {
      setPendingDeleteProvider(null);
      setProviderError(getErrorMessage(requestError, "Failed to delete provider"));
    } finally {
      setProviderDeleting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading...
      </div>
    );
  }

  if (error && !settings) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button onClick={fetchSettings} className="mt-2 text-sm text-blue-600 hover:underline">
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!settings) {
    return null;
  }

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold">Auth Settings</h2>
      {error && (
        <div className="px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-red-800 text-sm">
          {error}
        </div>
      )}
      {success && (
        <div className="px-4 py-2 bg-green-50 border border-green-200 rounded-lg text-green-800 text-sm">
          {success}
        </div>
      )}

      <AuthSettingsToggles settings={settings} onToggle={handleToggle} />

      <AuthSettingsProviders
        providers={providers}
        providerError={providerError}
        providerSuccess={providerSuccess}
        providerFormError={providerFormError}
        providerEditingName={providerEditingName}
        providerForm={providerForm}
        providerSaving={providerSaving}
        providerTesting={providerTesting}
        testResult={testResult}
        onOpenProviderForm={openProviderForm}
        onCloseProviderForm={closeProviderForm}
        onProviderFieldChange={handleProviderFieldChange}
        onProviderSave={handleProviderSave}
        onTestConnection={handleTestConnection}
        onRequestProviderDelete={requestProviderDelete}
      />

      <ConfirmDialog
        open={pendingDeleteProvider !== null}
        title="Delete Provider"
        message={`Are you sure you want to delete provider "${pendingDeleteProvider?.name}"? This cannot be undone.`}
        confirmLabel="Delete"
        destructive
        loading={providerDeleting}
        onConfirm={confirmProviderDelete}
        onCancel={cancelProviderDelete}
      />

      <AuthSettingsOIDCForm
        showOIDCForm={showOIDCForm}
        oidcForm={oidcForm}
        oidcFormError={oidcFormError}
        oidcSaving={oidcSaving}
        onShowForm={openOIDCForm}
        onCancel={closeOIDCForm}
        onFieldChange={handleOIDCFieldChange}
        onSave={handleOIDCSave}
      />
    </div>
  );
}
