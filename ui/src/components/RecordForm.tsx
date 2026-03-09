import { useState, useCallback, useEffect, type FormEvent } from "react";
import type { Column } from "../types";
import { X, Dices } from "lucide-react";
import { cn } from "../lib/utils";

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

interface RecordFormProps {
  columns: Column[];
  primaryKey: string[];
  initialData?: Record<string, unknown>;
  onSubmit: (data: Record<string, unknown>) => Promise<void>;
  onClose: () => void;
  mode: "create" | "edit";
}

export function RecordForm({
  columns,
  primaryKey,
  initialData,
  onSubmit,
  onClose,
  mode,
}: RecordFormProps) {
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const col of columns) {
      if (initialData && initialData[col.name] !== undefined && initialData[col.name] !== null) {
        const v = initialData[col.name];
        if (col.type.startsWith("timestamp") && typeof v === "string") {
          // datetime-local inputs need YYYY-MM-DDTHH:mm:ss format (no timezone)
          const d = new Date(v);
          if (!isNaN(d.getTime())) {
            const pad = (n: number) => String(n).padStart(2, "0");
            init[col.name] = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
          } else {
            init[col.name] = String(v);
          }
        } else {
          init[col.name] = typeof v === "object" ? JSON.stringify(v, null, 2) : String(v);
        }
      } else {
        init[col.name] = "";
      }
    }
    return init;
  });
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [hints, setHints] = useState<Record<string, string>>({});

  // Close on Escape key
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  const setHint = useCallback((name: string, hint: string) => {
    setHints((prev) => ({ ...prev, [name]: hint }));
  }, []);

  const clearHint = useCallback((name: string) => {
    setHints((prev) => {
      const next = { ...prev };
      delete next[name];
      return next;
    });
  }, []);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      setError(null);
      setSaving(true);
      try {
        const data: Record<string, unknown> = {};
        for (const col of columns) {
          // Skip PK fields on create if they have a default (likely auto-generated).
          if (mode === "create" && col.isPrimaryKey && col.default && values[col.name] === "") {
            continue;
          }
          // Skip PK fields on edit (can't change PKs).
          if (mode === "edit" && col.isPrimaryKey) {
            continue;
          }
          const raw = values[col.name];
          // Skip empty optional fields.
          if (raw === "" && col.nullable) {
            if (mode === "create") continue;
            // On edit, only send null if the original value was not already null.
            if (initialData && (initialData[col.name] === null || initialData[col.name] === undefined)) continue;
            data[col.name] = null;
            continue;
          }
          if (raw === "" && mode === "edit") continue;
          if (raw === "" && col.default) continue;
          data[col.name] = coerceValue(col, raw);
        }
        await onSubmit(data);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to save");
      } finally {
        setSaving(false);
      }
    },
    [columns, values, mode, initialData, onSubmit],
  );

  const setValue = useCallback((name: string, value: string) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  }, []);

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/20" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-white shadow-lg overflow-y-auto flex flex-col">
        <div className="px-6 py-4 border-b flex items-center justify-between sticky top-0 bg-white z-10">
          <h2 className="font-semibold">
            {mode === "create" ? "New Record" : "Edit Record"}
          </h2>
          <button onClick={onClose} className="p-1 hover:bg-gray-100 rounded" aria-label="Close">
            <X className="w-4 h-4" />
          </button>
        </div>

        {error && (
          <div className="mx-6 mt-4 bg-red-50 border border-red-200 text-red-700 rounded px-3 py-2 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="p-6 space-y-4 flex-1">
          {columns.map((col) => {
            const isPK = primaryKey.includes(col.name);
            const disabled = mode === "edit" && isPK;
            return (
              <FieldInput
                key={col.name}
                column={col}
                value={values[col.name]}
                onChange={(v) => setValue(col.name, v)}
                disabled={disabled}
                hint={hints[col.name]}
                onHint={(h) => (h ? setHint(col.name, h) : clearHint(col.name))}
              />
            );
          })}

          <div className="flex gap-2 pt-4 border-t sticky bottom-0 bg-white py-4">
            <button
              type="submit"
              disabled={saving}
              className="flex-1 bg-gray-900 text-white rounded px-4 py-2 text-sm font-medium hover:bg-gray-800 disabled:opacity-50"
            >
              {saving ? "Saving..." : mode === "create" ? "Create" : "Save Changes"}
            </button>
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 rounded"
            >
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function FieldInput({
  column,
  value,
  onChange,
  disabled,
  hint,
  onHint,
}: {
  column: Column;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  hint?: string;
  onHint?: (h: string | null) => void;
}) {
  const isJson = column.jsonType === "object" || column.jsonType === "array" || column.type === "jsonb" || column.type === "json";
  const isBoolean = column.type === "boolean" || column.type === "bool";
  const isEnum = column.enumValues && column.enumValues.length > 0;
  const isUuid = column.type === "uuid";
  const isText = column.type === "text" || isJson;

  const validateField = (val: string) => {
    if (!val || !onHint) return;
    if (isUuid && !UUID_RE.test(val)) {
      onHint("Must be a UUID — click the dice to generate one");
      return;
    }
    if (isJson) {
      try {
        JSON.parse(val);
      } catch {
        onHint('Invalid JSON — e.g. {"key": "value"} or [1, 2, 3]');
        return;
      }
    }
    onHint(null);
  };

  const handleBlur = () => validateField(value);

  const handleChange = (v: string) => {
    onChange(v);
    if (hint) onHint?.(null);
  };

  const generateUuid = () => {
    const uuid = crypto.randomUUID();
    onChange(uuid);
    onHint?.(null);
  };

  const placeholder = isUuid
    ? column.default
      ? `default: ${column.default}`
      : "e.g. 550e8400-e29b-41d4-a716-446655440000"
    : column.default
      ? `default: ${column.default}`
      : column.nullable
        ? "null"
        : "";

  return (
    <div>
      <label htmlFor={`field-${column.name}`} className="text-xs font-medium text-gray-600 block mb-1">
        {column.name}
        <span className="text-gray-300 font-normal ml-1.5">{column.type}</span>
        {!column.nullable && !column.default && (
          <span className="text-red-400 ml-0.5">*</span>
        )}
        {column.isPrimaryKey && (
          <span className="text-blue-500 ml-1.5 text-xs">PK</span>
        )}
      </label>

      {isBoolean ? (
        <select
          id={`field-${column.name}`}
          value={value}
          onChange={(e) => handleChange(e.target.value)}
          disabled={disabled}
          className={cn(
            "w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500",
            disabled && "bg-gray-50 text-gray-400",
          )}
        >
          {column.nullable && <option value="">null</option>}
          <option value="true">true</option>
          <option value="false">false</option>
        </select>
      ) : isEnum ? (
        <select
          id={`field-${column.name}`}
          value={value}
          onChange={(e) => handleChange(e.target.value)}
          disabled={disabled}
          className={cn(
            "w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500",
            disabled && "bg-gray-50 text-gray-400",
          )}
        >
          <option value="">-- select --</option>
          {column.enumValues!.map((v) => (
            <option key={v} value={v}>
              {v}
            </option>
          ))}
        </select>
      ) : isUuid ? (
        <div className="flex gap-1.5">
          <input
            id={`field-${column.name}`}
            type="text"
            value={value}
            onChange={(e) => handleChange(e.target.value)}
            onBlur={handleBlur}
            disabled={disabled}
            className={cn(
              "flex-1 border rounded px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500",
              disabled && "bg-gray-50 text-gray-400",
              hint && "border-amber-400",
            )}
            placeholder={placeholder}
          />
          {!disabled && (
            <button
              type="button"
              onClick={generateUuid}
              title="Generate random UUID"
              aria-label="Generate random UUID"
              className="px-2 py-2 border rounded text-gray-500 hover:bg-gray-50 hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <Dices className="w-4 h-4" />
            </button>
          )}
        </div>
      ) : isText ? (
        <textarea
          id={`field-${column.name}`}
          value={value}
          onChange={(e) => handleChange(e.target.value)}
          onBlur={handleBlur}
          disabled={disabled}
          rows={isJson ? 5 : 3}
          className={cn(
            "w-full border rounded px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 resize-y",
            disabled && "bg-gray-50 text-gray-400",
            hint && "border-amber-400",
          )}
          placeholder={placeholder}
        />
      ) : (
        <input
          id={`field-${column.name}`}
          type={inputType(column.type)}
          value={value}
          onChange={(e) => handleChange(e.target.value)}
          disabled={disabled}
          className={cn(
            "w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500",
            disabled && "bg-gray-50 text-gray-400",
          )}
          placeholder={placeholder}
        />
      )}

      {hint && (
        <p className="mt-1 text-xs text-amber-600">{hint}</p>
      )}
    </div>
  );
}

function inputType(pgType: string): string {
  if (pgType.startsWith("int") || pgType === "smallint" || pgType === "bigint" || pgType === "serial" || pgType === "bigserial") return "number";
  if (pgType === "numeric" || pgType === "decimal" || pgType === "real" || pgType === "double precision" || pgType === "float4" || pgType === "float8") return "number";
  if (pgType === "date") return "date";
  if (pgType.startsWith("timestamp")) return "datetime-local";
  if (pgType === "time" || pgType.startsWith("time ")) return "time";
  return "text";
}

function coerceValue(col: Column, raw: string): unknown {
  if (raw === "") return null;
  const t = col.type;
  if (t === "boolean" || t === "bool") return raw === "true";
  if (
    t.startsWith("int") || t === "smallint" || t === "bigint" ||
    t === "serial" || t === "bigserial" || t === "oid"
  ) {
    const n = parseInt(raw, 10);
    return isNaN(n) ? raw : n;
  }
  if (
    t === "numeric" || t === "decimal" || t === "real" ||
    t === "double precision" || t === "float4" || t === "float8"
  ) {
    const n = parseFloat(raw);
    return isNaN(n) ? raw : n;
  }
  if (t === "jsonb" || t === "json" || col.jsonType === "object" || col.jsonType === "array") {
    try {
      return JSON.parse(raw);
    } catch {
      return raw;
    }
  }
  return raw;
}
