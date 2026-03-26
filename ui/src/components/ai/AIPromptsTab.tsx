import { useMemo, useState } from "react";
import { Loader2, Trash2 } from "lucide-react";
import { createPrompt, getPromptVersions, renderPrompt, updatePrompt } from "../../api_ai";
import type { Prompt, PromptVersion } from "../../types/ai";
import { useAppToast } from "../ToastProvider";

interface AIPromptsTabProps {
  prompts: Prompt[];
  deleting: string | null;
  onPromptsChanged: () => Promise<void>;
  onDelete: (prompt: Prompt) => void;
}

export function AIPromptsTab({
  prompts,
  deleting,
  onPromptsChanged,
  onDelete,
}: AIPromptsTabProps) {
  const [creating, setCreating] = useState(false);
  const [editingPromptId, setEditingPromptId] = useState<string | null>(null);
  const [promptName, setPromptName] = useState("");
  const [promptTemplate, setPromptTemplate] = useState("");
  const [saving, setSaving] = useState(false);
  const [renderVariablesText, setRenderVariablesText] = useState("{}");
  const [renderedOutput, setRenderedOutput] = useState("");
  const [selectedPromptId, setSelectedPromptId] = useState<string | null>(null);
  const [versions, setVersions] = useState<PromptVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const { addToast } = useAppToast();

  const selectedPrompt = useMemo(() => {
    if (!selectedPromptId) return prompts[0] ?? null;
    return prompts.find((prompt) => prompt.id === selectedPromptId) ?? prompts[0] ?? null;
  }, [prompts, selectedPromptId]);

  const resetEditor = () => {
    setCreating(false);
    setEditingPromptId(null);
    setPromptName("");
    setPromptTemplate("");
  };

  const handleCreatePrompt = async () => {
    if (!promptName.trim() || !promptTemplate.trim()) return;
    setSaving(true);
    try {
      await createPrompt({
        name: promptName.trim(),
        template: promptTemplate.trim(),
        variables: [],
      });
      await onPromptsChanged();
      addToast("success", "Prompt created");
      resetEditor();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to create prompt");
    } finally {
      setSaving(false);
    }
  };

  const handleUpdatePrompt = async () => {
    if (!editingPromptId || !promptTemplate.trim()) return;
    setSaving(true);
    try {
      await updatePrompt(editingPromptId, { template: promptTemplate.trim() });
      await onPromptsChanged();
      addToast("success", "Prompt updated");
      resetEditor();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to update prompt");
    } finally {
      setSaving(false);
    }
  };

  const handleViewVersions = async (promptId: string) => {
    setLoadingVersions(true);
    try {
      const result = await getPromptVersions(promptId);
      setVersions(result);
      setSelectedPromptId(promptId);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to load versions");
      setVersions([]);
    } finally {
      setLoadingVersions(false);
    }
  };

  const handleRenderTemplate = async () => {
    if (!selectedPrompt) return;
    let variables: Record<string, unknown>;
    try {
      const parsed = JSON.parse(renderVariablesText);
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
        throw new Error("variables must be a JSON object");
      }
      variables = parsed as Record<string, unknown>;
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Invalid variables JSON");
      return;
    }

    try {
      const result = await renderPrompt(selectedPrompt.id, variables);
      setRenderedOutput(result.rendered);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to render prompt");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">Prompts</h3>
        <button
          className="px-3 py-1.5 text-sm border rounded hover:bg-gray-100"
          onClick={() => {
            setCreating(true);
            setEditingPromptId(null);
            setPromptName("");
            setPromptTemplate("");
          }}
        >
          Create Prompt
        </button>
      </div>

      {(creating || editingPromptId) && (
        <div className="border rounded-lg p-4 bg-gray-50 dark:bg-gray-800/40">
          <div className="grid gap-3 md:grid-cols-2">
            <div>
              <label htmlFor="prompt-name" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
                Prompt Name
              </label>
              <input
                id="prompt-name"
                aria-label="Prompt Name"
                value={promptName}
                disabled={!creating}
                onChange={(e) => setPromptName(e.target.value)}
                className="w-full border rounded px-2 py-1.5 text-sm"
              />
            </div>
            <div className="md:col-span-2">
              <label htmlFor="prompt-template" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
                Template
              </label>
              <textarea
                id="prompt-template"
                aria-label="Template"
                value={promptTemplate}
                onChange={(e) => setPromptTemplate(e.target.value)}
                className="w-full border rounded px-2 py-1.5 text-sm min-h-[84px]"
              />
            </div>
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <button
              onClick={resetEditor}
              className="px-3 py-1.5 text-sm border rounded hover:bg-gray-100"
            >
              Cancel
            </button>
            {creating ? (
              <button
                onClick={handleCreatePrompt}
                disabled={saving || !promptName.trim() || !promptTemplate.trim()}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                Save Prompt
              </button>
            ) : (
              <button
                onClick={handleUpdatePrompt}
                disabled={saving || !promptTemplate.trim()}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                Update Prompt
              </button>
            )}
          </div>
        </div>
      )}

      <div className="border rounded-lg overflow-hidden">
        {prompts.length === 0 ? (
          <div className="text-center py-12 bg-gray-50 dark:bg-gray-800 text-gray-500 text-sm">
            No prompts configured
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Name</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Version</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Model</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Provider</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Variables</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Updated</th>
                <th className="text-right px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
              </tr>
            </thead>
            <tbody>
              {prompts.map((prompt) => (
                <tr key={prompt.id} className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800">
                  <td className="px-4 py-2.5 font-medium">{prompt.name}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">v{prompt.version}</td>
                  <td className="px-4 py-2.5 text-xs">
                    {prompt.model ? (
                      <code className="bg-gray-100 dark:bg-gray-700 px-1.5 py-0.5 rounded">{prompt.model}</code>
                    ) : "-"}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">{prompt.provider || "-"}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">
                    {prompt.variables.length > 0
                      ? prompt.variables.map((v) => v.name).join(", ")
                      : "-"}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">
                    {new Date(prompt.updated_at).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex justify-end gap-1">
                      <button
                        onClick={() => {
                          setCreating(false);
                          setEditingPromptId(prompt.id);
                          setPromptName(prompt.name);
                          setPromptTemplate(prompt.template);
                          setSelectedPromptId(prompt.id);
                        }}
                        className="px-2 py-1 text-xs border rounded hover:bg-gray-100"
                        aria-label={`Edit prompt ${prompt.name}`}
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => { void handleViewVersions(prompt.id); }}
                        className="px-2 py-1 text-xs border rounded hover:bg-gray-100"
                        aria-label={`View versions ${prompt.name}`}
                      >
                        Versions
                      </button>
                      <button
                        onClick={() => setSelectedPromptId(prompt.id)}
                        className="px-2 py-1 text-xs border rounded hover:bg-gray-100"
                      >
                        Render
                      </button>
                      <button
                        onClick={() => onDelete(prompt)}
                        disabled={deleting === prompt.id}
                        className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700"
                        aria-label={`Delete prompt ${prompt.name}`}
                      >
                        {deleting === prompt.id ? (
                          <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        ) : (
                          <Trash2 className="w-3.5 h-3.5" />
                        )}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="border rounded-lg p-4">
        <h4 className="text-sm font-medium mb-2">Template Renderer</h4>
        <textarea
          value={renderVariablesText}
          onChange={(e) => setRenderVariablesText(e.target.value)}
          className="w-full border rounded px-2 py-1.5 text-sm min-h-[88px]"
          aria-label="Render Variables"
        />
        <button
          onClick={() => { void handleRenderTemplate(); }}
          disabled={!selectedPrompt}
          className="mt-2 px-3 py-1.5 text-sm border rounded hover:bg-gray-100 disabled:opacity-50"
        >
          Render Template
        </button>
        {renderedOutput && (
          <pre className="mt-2 bg-gray-50 dark:bg-gray-800 p-3 rounded text-sm whitespace-pre-wrap">
            <code>{renderedOutput}</code>
          </pre>
        )}
      </div>

      <div className="border rounded-lg p-4">
        <h4 className="text-sm font-medium mb-2">Version History</h4>
        {loadingVersions ? (
          <p className="text-xs text-gray-500">Loading versions...</p>
        ) : versions.length === 0 ? (
          <p className="text-xs text-gray-500">No versions loaded</p>
        ) : (
          <ul className="space-y-1 text-xs text-gray-600 dark:text-gray-300">
            {versions.map((version) => (
              <li key={version.id}>
                v{version.version} ({new Date(version.created_at).toLocaleString()})
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
