export interface FilterFieldOption {
  value: string;
  label: string;
}

export interface FilterField {
  name: string;
  label: string;
  type: "text" | "select" | "date";
  placeholder?: string;
  options?: FilterFieldOption[];
}

interface FilterBarProps {
  fields: FilterField[];
  values: Record<string, string>;
  onApply: (values: Record<string, string>) => void;
  onReset?: () => void;
  onChange?: (name: string, value: string) => void;
}

export function FilterBar({
  fields,
  values,
  onApply,
  onReset,
  onChange,
}: FilterBarProps) {
  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    onApply(values);
  };

  return (
    <form onSubmit={handleSubmit} className="mb-4 flex items-end gap-3 flex-wrap">
      {fields.map((field) => (
        <div key={field.name}>
          <label
            htmlFor={`filter-${field.name}`}
            className="block text-xs text-gray-600 dark:text-gray-300 mb-1"
          >
            {field.label}
          </label>
          {field.type === "select" ? (
            <select
              id={`filter-${field.name}`}
              aria-label={field.label}
              value={values[field.name] ?? ""}
              onChange={(e) => onChange?.(field.name, e.target.value)}
              className="border rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-800"
            >
              {field.options?.map((opt) => (
                <option key={opt.value || "__empty"} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          ) : (
            <input
              id={`filter-${field.name}`}
              aria-label={field.label}
              type={field.type}
              value={values[field.name] ?? ""}
              onChange={(e) => onChange?.(field.name, e.target.value)}
              placeholder={field.placeholder}
              className="border rounded px-2 py-1.5 text-sm"
            />
          )}
        </div>
      ))}
      <button
        type="submit"
        className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
      >
        Apply Filters
      </button>
      {onReset && (
        <button
          type="button"
          onClick={onReset}
          className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded border"
        >
          Reset
        </button>
      )}
    </form>
  );
}
