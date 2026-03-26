import type { CallLogListResponse } from "../../types/ai";
import { AdminTable, type Column } from "../shared/AdminTable";
import { StatusBadge } from "../shared/StatusBadge";
import { FilterBar, type FilterField } from "../shared/FilterBar";

type LogRow = CallLogListResponse["logs"][number];

const LOG_COLUMNS: Column<LogRow>[] = [
  { key: "provider", header: "Provider" },
  { key: "model", header: "Model" },
  {
    key: "input_tokens",
    header: "Tokens (in/out)",
    render: (row) => `${row.input_tokens} / ${row.output_tokens}`,
  },
  {
    key: "cost_usd",
    header: "Cost",
    render: (row) => `$${row.cost_usd.toFixed(4)}`,
  },
  {
    key: "duration_ms",
    header: "Duration",
    render: (row) => `${row.duration_ms}ms`,
  },
  {
    key: "status",
    header: "Status",
    render: (row) => <StatusBadge status={row.status} />,
  },
  {
    key: "created_at",
    header: "Time",
    render: (row) => new Date(row.created_at).toLocaleString(),
  },
];

const FILTER_FIELDS: FilterField[] = [
  {
    name: "provider",
    label: "Provider",
    type: "select",
    options: [
      { value: "", label: "All providers" },
      { value: "openai", label: "OpenAI" },
      { value: "anthropic", label: "Anthropic" },
    ],
  },
  {
    name: "status",
    label: "Status",
    type: "select",
    options: [
      { value: "", label: "All statuses" },
      { value: "success", label: "Success" },
      { value: "error", label: "Error" },
    ],
  },
  { name: "from", label: "From", type: "date" },
  { name: "to", label: "To", type: "date" },
];

interface AILogsTabProps {
  data: CallLogListResponse | null;
  filterValues: Record<string, string>;
  onFilterChange: (name: string, value: string) => void;
  onApply: (values: Record<string, string>) => void;
  onReset: () => void;
}

export function AILogsTab({
  data,
  filterValues,
  onFilterChange,
  onApply,
  onReset,
}: AILogsTabProps) {
  return (
    <>
      <FilterBar
        fields={FILTER_FIELDS}
        values={filterValues}
        onChange={onFilterChange}
        onApply={onApply}
        onReset={onReset}
      />
      <AdminTable
        columns={LOG_COLUMNS}
        rows={data?.logs ?? []}
        rowKey="id"
        emptyMessage="No AI logs found"
      />
    </>
  );
}
