import { useState } from "react";
import { X, Eye, EyeOff } from "lucide-react";

export interface EnvVar {
  key: string;
  value: string;
}

interface EnvVarEditorProps {
  envVars: EnvVar[];
  onChange: (envVars: EnvVar[]) => void;
}

export function EnvVarEditor({ envVars, onChange }: EnvVarEditorProps) {
  const [revealedIndices, setRevealedIndices] = useState<Set<number>>(new Set());

  const addEnvVar = () => onChange([...envVars, { key: "", value: "" }]);

  const removeEnvVar = (idx: number) => {
    onChange(envVars.filter((_, i) => i !== idx));
    setRevealedIndices((prev) => {
      const next = new Set<number>();
      for (const i of prev) {
        if (i < idx) next.add(i);
        else if (i > idx) next.add(i - 1);
      }
      return next;
    });
  };

  const updateEnvVar = (idx: number, field: "key" | "value", val: string) => {
    onChange(envVars.map((ev, i) => (i === idx ? { ...ev, [field]: val } : ev)));
  };

  const toggleReveal = (idx: number) => {
    setRevealedIndices((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  };

  const getDuplicateKeys = (): Set<number> => {
    const seen = new Map<string, number>();
    const dupes = new Set<number>();
    envVars.forEach((ev, idx) => {
      const k = ev.key.trim();
      if (!k) return;
      if (seen.has(k)) {
        dupes.add(seen.get(k)!);
        dupes.add(idx);
      } else {
        seen.set(k, idx);
      }
    });
    return dupes;
  };

  const duplicateIndices = getDuplicateKeys();

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <label className="text-sm font-medium text-gray-700 dark:text-gray-200">Environment Variables</label>
        <button
          onClick={addEnvVar}
          className="text-xs text-blue-600 hover:text-blue-700"
          data-testid="add-env-var"
        >
          + Add Variable
        </button>
      </div>
      {envVars.length === 0 && (
        <p className="text-xs text-gray-400 dark:text-gray-500" data-testid="env-empty">
          No environment variables configured.
        </p>
      )}
      {envVars.map((ev, idx) => {
        const isDuplicate = duplicateIndices.has(idx);
        return (
          <div key={idx} className="mb-2">
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="KEY"
                value={ev.key}
                onChange={(e) => updateEnvVar(idx, "key", e.target.value)}
                className={`flex-1 px-3 py-1.5 border rounded text-sm font-mono ${
                  isDuplicate ? "border-red-400" : ""
                }`}
                data-testid={`env-key-${idx}`}
              />
              <div className="flex-1 relative">
                <input
                  type={revealedIndices.has(idx) ? "text" : "password"}
                  placeholder="value"
                  value={ev.value}
                  onChange={(e) => updateEnvVar(idx, "value", e.target.value)}
                  className="w-full px-3 py-1.5 pr-8 border rounded text-sm font-mono"
                  data-testid={`env-value-${idx}`}
                />
                <button
                  type="button"
                  onClick={() => toggleReveal(idx)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300"
                  aria-label={revealedIndices.has(idx) ? "Hide value" : "Reveal value"}
                  data-testid={`env-reveal-${idx}`}
                >
                  {revealedIndices.has(idx) ? (
                    <EyeOff className="w-3.5 h-3.5" />
                  ) : (
                    <Eye className="w-3.5 h-3.5" />
                  )}
                </button>
              </div>
              <button
                onClick={() => removeEnvVar(idx)}
                className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-red-500"
                data-testid={`env-remove-${idx}`}
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            {isDuplicate && (
              <p className="text-xs text-red-500 mt-0.5" data-testid={`env-dupe-${idx}`}>
                Duplicate key
              </p>
            )}
          </div>
        );
      })}
    </div>
  );
}

export function envVarsToMap(envVars: EnvVar[]): Record<string, string> {
  const map: Record<string, string> = {};
  for (const { key, value } of envVars) {
    if (key.trim()) map[key.trim()] = value;
  }
  return map;
}

export function hasEnvVarErrors(envVars: EnvVar[]): boolean {
  const keys = envVars.map((ev) => ev.key.trim()).filter(Boolean);
  const uniqueKeys = new Set(keys);
  if (uniqueKeys.size < keys.length) return true;
  if (envVars.some((ev) => ev.value && !ev.key.trim())) return true;
  return false;
}
