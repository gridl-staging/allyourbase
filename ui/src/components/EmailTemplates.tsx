import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  EmailTemplateEffective,
  EmailTemplateListResponse,
  PreviewEmailTemplateResponse,
} from "../types";
import {
  deleteEmailTemplate,
  getEmailTemplate,
  listEmailTemplates,
  previewEmailTemplate,
  sendTemplateEmail,
  setEmailTemplateEnabled,
  upsertEmailTemplate,
} from "../api";
import { AlertCircle, Loader2 } from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

const PREVIEW_DEBOUNCE_MS = 350;

function formatDate(iso?: string): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

function defaultVariableValue(name: string): string {
  if (name === "AppName") return "Allyourbase";
  if (name === "ActionURL") return "https://example.com/action";
  return "";
}

function defaultVarsJSON(variables: string[] | undefined): string {
  if (!variables || variables.length === 0) {
    return "{}";
  }

  const vars: Record<string, string> = {};
  for (const name of variables) {
    vars[name] = defaultVariableValue(name);
  }
  return JSON.stringify(vars, null, 2);
}

function parseVariablesJSON(input: string): { vars: Record<string, string> | null; error: string | null } {
  const raw = input.trim();
  if (raw === "") {
    return { vars: {}, error: null };
  }

  let decoded: unknown;
  try {
    decoded = JSON.parse(raw);
  } catch {
    return { vars: null, error: "Preview variables must be valid JSON." };
  }

  if (decoded === null || typeof decoded !== "object" || Array.isArray(decoded)) {
    return { vars: null, error: "Preview variables must be a JSON object." };
  }

  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(decoded)) {
    if (typeof v !== "string") {
      return { vars: null, error: `Variable ${k} must be a string.` };
    }
    out[k] = v;
  }

  return { vars: out, error: null };
}

export function EmailTemplates() {
  const [list, setList] = useState<EmailTemplateListResponse | null>(null);
  const [loadingList, setLoadingList] = useState(true);
  const [loadingEffective, setLoadingEffective] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [effective, setEffective] = useState<EmailTemplateEffective | null>(null);

  const [subjectTemplate, setSubjectTemplate] = useState("");
  const [htmlTemplate, setHTMLTemplate] = useState("");
  const [previewVarsInput, setPreviewVarsInput] = useState("{}");
  const [previewResult, setPreviewResult] = useState<PreviewEmailTemplateResponse | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [toggling, setToggling] = useState(false);
  const [sendTo, setSendTo] = useState("");
  const [sending, setSending] = useState(false);

  const { toasts, addToast, removeToast } = useToast();

  const selectedItem = useMemo(() => {
    if (!list || !selectedKey) return null;
    return list.items.find((item) => item.templateKey === selectedKey) ?? null;
  }, [list, selectedKey]);

  const isSystemKey = selectedKey?.startsWith("auth.") ?? false;
  const hasCustomOverride = selectedItem?.source === "custom";

  const loadList = useCallback(async () => {
    try {
      setError(null);
      const res = await listEmailTemplates();
      setList(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load email templates");
      setList(null);
    } finally {
      setLoadingList(false);
    }
  }, []);

  const loadEffective = useCallback(async (key: string) => {
    setLoadingEffective(true);
    try {
      const res = await getEmailTemplate(key);
      setEffective(res);
      setSubjectTemplate(res.subjectTemplate);
      setHTMLTemplate(res.htmlTemplate);
      setPreviewVarsInput(defaultVarsJSON(res.variables));
      setPreviewResult(null);
      setPreviewError(null);
      setSendTo("");
    } catch (e) {
      setEffective(null);
      setPreviewResult(null);
      setPreviewError(e instanceof Error ? e.message : "Failed to load template");
    } finally {
      setLoadingEffective(false);
    }
  }, []);

  useEffect(() => {
    loadList();
  }, [loadList]);

  useEffect(() => {
    if (!list || list.items.length === 0) {
      setSelectedKey(null);
      return;
    }

    if (!selectedKey) {
      setSelectedKey(list.items[0].templateKey);
      return;
    }

    const stillExists = list.items.some((item) => item.templateKey === selectedKey);
    if (!stillExists) {
      setSelectedKey(list.items[0].templateKey);
    }
  }, [list, selectedKey]);

  useEffect(() => {
    if (!selectedKey) return;
    loadEffective(selectedKey);
  }, [selectedKey, loadEffective]);

  useEffect(() => {
    if (!selectedKey || loadingEffective) return;
    if (subjectTemplate.trim() === "" || htmlTemplate.trim() === "") return;

    const parsed = parseVariablesJSON(previewVarsInput);
    if (parsed.error) {
      setPreviewError(parsed.error);
      setPreviewResult(null);
      return;
    }

    const timer = window.setTimeout(async () => {
      setPreviewLoading(true);
      try {
        const rendered = await previewEmailTemplate(selectedKey, {
          subjectTemplate,
          htmlTemplate,
          variables: parsed.vars ?? {},
        });
        setPreviewResult(rendered);
        setPreviewError(null);
      } catch (e) {
        setPreviewResult(null);
        setPreviewError(e instanceof Error ? e.message : "Preview failed");
      } finally {
        setPreviewLoading(false);
      }
    }, PREVIEW_DEBOUNCE_MS);

    return () => window.clearTimeout(timer);
  }, [selectedKey, loadingEffective, subjectTemplate, htmlTemplate, previewVarsInput]);

  const handleSave = async () => {
    if (!selectedKey) return;
    setSaving(true);
    try {
      await upsertEmailTemplate(selectedKey, {
        subjectTemplate,
        htmlTemplate,
      });
      addToast("success", `Saved ${selectedKey}`);
      await Promise.all([loadList(), loadEffective(selectedKey)]);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to save template");
    } finally {
      setSaving(false);
    }
  };

  const handleToggle = async () => {
    if (!selectedKey || !selectedItem || selectedItem.source !== "custom") return;
    setToggling(true);
    try {
      await setEmailTemplateEnabled(selectedKey, !selectedItem.enabled);
      addToast("success", `${!selectedItem.enabled ? "Enabled" : "Disabled"} ${selectedKey}`);
      await Promise.all([loadList(), loadEffective(selectedKey)]);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to update template status");
    } finally {
      setToggling(false);
    }
  };

  const handleDeleteOrReset = async () => {
    if (!selectedKey || !selectedItem || selectedItem.source !== "custom") return;

    setDeleting(true);
    try {
      await deleteEmailTemplate(selectedKey);
      addToast("success", isSystemKey ? `Reset ${selectedKey} to default` : `Deleted ${selectedKey}`);
      await loadList();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to delete template");
    } finally {
      setDeleting(false);
    }
  };

  const handleSendTest = async () => {
    if (!selectedKey) return;

    const recipient = sendTo.trim();
    if (!recipient) {
      addToast("error", "Test recipient is required");
      return;
    }

    const parsed = parseVariablesJSON(previewVarsInput);
    if (parsed.error) {
      addToast("error", parsed.error);
      return;
    }

    setSending(true);
    try {
      await sendTemplateEmail({
        templateKey: selectedKey,
        to: recipient,
        variables: parsed.vars ?? {},
      });
      addToast("success", `Sent test email to ${recipient}`);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to send test email");
    } finally {
      setSending(false);
    }
  };

  if (loadingList && !list) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading email templates...
      </div>
    );
  }

  if (error && !list) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => {
              setLoadingList(true);
              loadList();
            }}
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
        <h1 className="text-lg font-semibold">Email Templates</h1>
        <p className="text-sm text-gray-500 mt-0.5">
          Customize built-in auth emails and manage app-specific templates
        </p>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[320px_1fr] gap-6">
        <section className="border rounded-lg overflow-hidden">
          <div className="bg-gray-50 border-b px-4 py-2 text-xs font-medium text-gray-600 uppercase tracking-wider">
            Template Keys
          </div>
          {list && list.items.length > 0 ? (
            <ul>
              {list.items.map((item) => (
                <li key={item.templateKey} className="border-b last:border-0">
                  <button
                    onClick={() => setSelectedKey(item.templateKey)}
                    className={cn(
                      "w-full text-left px-4 py-2.5 hover:bg-gray-50",
                      selectedKey === item.templateKey && "bg-gray-100",
                    )}
                  >
                    <div className="font-mono text-xs text-gray-800">{item.templateKey}</div>
                    <div className="mt-1 flex items-center gap-2 text-[11px] text-gray-500">
                      <span
                        className={cn(
                          "px-1.5 py-0.5 rounded",
                          item.source === "custom" ? "bg-blue-100 text-blue-700" : "bg-gray-200 text-gray-700",
                        )}
                      >
                        {item.source}
                      </span>
                      <span>{item.enabled ? "enabled" : "disabled"}</span>
                      <span>updated {formatDate(item.updatedAt)}</span>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <div className="px-4 py-8 text-sm text-gray-500">No templates found.</div>
          )}
        </section>

        <section className="border rounded-lg p-4">
          {!selectedKey ? (
            <div className="text-sm text-gray-500">Select a template key to edit.</div>
          ) : loadingEffective ? (
            <div className="flex items-center text-sm text-gray-400">
              <Loader2 className="w-4 h-4 animate-spin mr-2" />
              Loading template...
            </div>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h2 className="text-base font-semibold">{selectedKey}</h2>
                  <p className="text-xs text-gray-500">
                    Editing {effective?.source ?? selectedItem?.source ?? "template"} template
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {hasCustomOverride && selectedItem ? (
                    <button
                      onClick={handleToggle}
                      disabled={toggling}
                      className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 disabled:opacity-60"
                    >
                      {selectedItem.enabled ? "Disable Override" : "Enable Override"}
                    </button>
                  ) : null}

                  {hasCustomOverride ? (
                    <button
                      onClick={handleDeleteOrReset}
                      disabled={deleting}
                      className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 disabled:opacity-60"
                    >
                      {isSystemKey ? "Reset to Default" : "Delete Template"}
                    </button>
                  ) : null}

                  <button
                    onClick={handleSave}
                    disabled={saving || !selectedKey}
                    className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-70"
                  >
                    Save Template
                  </button>
                </div>
              </div>

              <div>
                <label htmlFor="email-template-subject" className="block text-sm text-gray-700 mb-1">
                  Subject Template
                </label>
                <input
                  id="email-template-subject"
                  aria-label="Subject Template"
                  value={subjectTemplate}
                  onChange={(e) => setSubjectTemplate(e.target.value)}
                  className="w-full border rounded px-3 py-2 text-sm font-mono"
                />
              </div>

              <div>
                <label htmlFor="email-template-html" className="block text-sm text-gray-700 mb-1">
                  HTML Template
                </label>
                <textarea
                  id="email-template-html"
                  aria-label="HTML Template"
                  value={htmlTemplate}
                  onChange={(e) => setHTMLTemplate(e.target.value)}
                  rows={8}
                  className="w-full border rounded px-3 py-2 text-sm font-mono"
                />
              </div>

              <div>
                <label htmlFor="email-template-vars" className="block text-sm text-gray-700 mb-1">
                  Preview Variables (JSON)
                </label>
                <textarea
                  id="email-template-vars"
                  aria-label="Preview Variables (JSON)"
                  value={previewVarsInput}
                  onChange={(e) => setPreviewVarsInput(e.target.value)}
                  rows={5}
                  className="w-full border rounded px-3 py-2 text-sm font-mono"
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-[1fr_auto] gap-2 items-end">
                <div>
                  <label htmlFor="email-template-send-to" className="block text-sm text-gray-700 mb-1">
                    Test Recipient
                  </label>
                  <input
                    id="email-template-send-to"
                    aria-label="Test Recipient"
                    value={sendTo}
                    onChange={(e) => setSendTo(e.target.value)}
                    className="w-full border rounded px-3 py-2 text-sm"
                    placeholder="user@example.com"
                  />
                </div>
                <button
                  onClick={handleSendTest}
                  disabled={sending}
                  className="px-3 py-2 text-sm border rounded hover:bg-gray-50 disabled:opacity-60"
                >
                  Send Test Email
                </button>
              </div>

              <div className="border rounded-lg p-3 bg-gray-50 space-y-2">
                <h3 className="text-sm font-medium">Preview</h3>
                {previewLoading ? (
                  <p className="text-xs text-gray-500">Rendering preview...</p>
                ) : previewError ? (
                  <p className="text-xs text-red-600">{previewError}</p>
                ) : previewResult ? (
                  <div className="space-y-2 text-xs">
                    <div>
                      <p className="font-medium text-gray-700 mb-1">Subject</p>
                      <pre className="whitespace-pre-wrap border rounded bg-white p-2">{previewResult.subject}</pre>
                    </div>
                    <div>
                      <p className="font-medium text-gray-700 mb-1">HTML</p>
                      <pre className="whitespace-pre-wrap border rounded bg-white p-2 max-h-36 overflow-auto">
                        {previewResult.html}
                      </pre>
                    </div>
                    <div>
                      <p className="font-medium text-gray-700 mb-1">Plaintext</p>
                      <pre className="whitespace-pre-wrap border rounded bg-white p-2">{previewResult.text}</pre>
                    </div>
                  </div>
                ) : (
                  <p className="text-xs text-gray-500">Preview will appear after template or variables change.</p>
                )}
              </div>
            </div>
          )}
        </section>
      </div>

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
