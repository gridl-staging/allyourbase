import { useState } from "react";
import {
  adminListTickets,
  adminGetTicket,
  adminUpdateTicket,
  adminAddMessage,
} from "../api_support";
import type { Ticket, TicketWithMessages } from "../types/support";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { FilterBar, type FilterField } from "./shared/FilterBar";
import { StatusBadge } from "./shared/StatusBadge";
import { useDraftFilters } from "../hooks/useDraftFilters";

const TICKET_STATUSES = [
  "open",
  "in_progress",
  "waiting_on_customer",
  "resolved",
  "closed",
] as const;

const PRIORITIES = ["low", "normal", "high", "urgent"] as const;

const ticketStatusMap: Record<
  string,
  "warning" | "info" | "success" | "default"
> = {
  open: "warning",
  in_progress: "info",
  waiting_on_customer: "warning",
  resolved: "success",
  closed: "default",
};

const senderColors: Record<string, string> = {
  customer: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  support:
    "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  system: "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300",
};

type TicketUpdate = { status?: string; priority?: string };

const filterFields: FilterField[] = [
  {
    name: "status",
    label: "Status",
    type: "select",
    options: [
      { value: "", label: "All" },
      ...TICKET_STATUSES.map((s) => ({ value: s, label: s.replace(/_/g, " ") })),
    ],
  },
  {
    name: "priority",
    label: "Priority",
    type: "select",
    options: [
      { value: "", label: "All" },
      ...PRIORITIES.map((p) => ({ value: p, label: p })),
    ],
  },
];

export function SupportTickets() {
  const { draftValues, appliedValues, setDraftValue, applyValues, resetValues } =
    useDraftFilters({ status: "", priority: "" });

  const { data, loading, error, setError, actionLoading, runAction, refresh } = useAdminResource(
    () =>
      adminListTickets({
        status: appliedValues.status || undefined,
        priority: appliedValues.priority || undefined,
      }),
  );

  const [expandedTicket, setExpandedTicket] =
    useState<TicketWithMessages | null>(null);
  const [replyBody, setReplyBody] = useState("");
  const queueRefresh = () => setTimeout(() => refresh(), 0);

  const handleExpand = async (ticket: Ticket) => {
    if (expandedTicket?.ticket.id === ticket.id) {
      setExpandedTicket(null);
      return;
    }
    try {
      const detail = await adminGetTicket(ticket.id);
      setExpandedTicket(detail);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load ticket");
    }
  };

  const handleUpdateTicket = (ticketId: string, update: TicketUpdate) =>
    runAction(async () => {
      await adminUpdateTicket(ticketId, update);
      if (expandedTicket?.ticket.id === ticketId) {
        const detail = await adminGetTicket(ticketId);
        setExpandedTicket(detail);
      }
    });

  const handleReply = (ticketId: string) =>
    runAction(async () => {
      await adminAddMessage(ticketId, { body: replyBody.trim() });
      setReplyBody("");
      const detail = await adminGetTicket(ticketId);
      setExpandedTicket(detail);
    });

  const columns: Column<Ticket>[] = [
    { key: "subject", header: "Subject" },
    {
      key: "status",
      header: "Status",
      render: (row) => (
        <StatusBadge status={row.status} variantMap={ticketStatusMap} />
      ),
    },
    {
      key: "priority",
      header: "Priority",
      render: (row) => (
        <StatusBadge status={row.priority} />
      ),
    },
    { key: "tenant_id", header: "Tenant" },
    {
      key: "created_at",
      header: "Created",
      render: (row) => new Date(row.created_at).toLocaleDateString(),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <button
          onClick={() => handleExpand(row)}
          className="text-xs text-blue-500 hover:text-blue-600"
        >
          Details
        </button>
      ),
    },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Support Tickets
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Support Tickets
      </h2>

      <FilterBar
        fields={filterFields}
        values={draftValues}
        onChange={(name, value) => setDraftValue(name, value)}
        onApply={(values) => {
          applyValues(values);
          queueRefresh();
        }}
        onReset={() => {
          resetValues();
          queueRefresh();
        }}
      />

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-4">
          Loading...
        </p>
      ) : (
        <AdminTable
          columns={columns}
          rows={data ?? []}
          rowKey="id"
          page={1}
          totalPages={1}
          onPageChange={() => {}}
          emptyMessage="No support tickets"
        />
      )}

      {expandedTicket && (
        <TicketDetail
          detail={expandedTicket}
          onUpdateTicket={handleUpdateTicket}
          onReply={handleReply}
          replyBody={replyBody}
          setReplyBody={setReplyBody}
          actionLoading={actionLoading}
        />
      )}
    </div>
  );
}

function TicketDetail({
  detail,
  onUpdateTicket,
  onReply,
  replyBody,
  setReplyBody,
  actionLoading,
}: {
  detail: TicketWithMessages;
  onUpdateTicket: (id: string, update: TicketUpdate) => void;
  onReply: (id: string) => void;
  replyBody: string;
  setReplyBody: (v: string) => void;
  actionLoading: boolean;
}) {
  const { ticket, messages } = detail;

  return (
    <div className="mt-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-gray-50 dark:bg-gray-900">
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
          {ticket.subject}
        </h4>
        <div className="flex gap-2">
          <TicketSelect
            value={ticket.status}
            options={TICKET_STATUSES}
            formatOptionLabel={(option) => option.replace(/_/g, " ")}
            ariaLabel="Ticket status"
            onChange={(nextStatus) =>
              onUpdateTicket(ticket.id, { status: nextStatus })
            }
          />
          <TicketSelect
            value={ticket.priority}
            options={PRIORITIES}
            ariaLabel="Ticket priority"
            onChange={(nextPriority) =>
              onUpdateTicket(ticket.id, { priority: nextPriority })
            }
          />
        </div>
      </div>

      <div className="space-y-3 mb-4">
        {messages.map((msg) => (
          <div key={msg.id} className="flex gap-2 text-sm">
            <span
              className={`shrink-0 px-1.5 py-0.5 rounded text-[10px] font-medium ${senderColors[msg.sender_type] || senderColors.system}`}
            >
              {msg.sender_type}
            </span>
            <div className="flex-1">
              <p className="text-gray-800 dark:text-gray-200">{msg.body}</p>
              <p className="text-[10px] text-gray-400 mt-0.5">
                {new Date(msg.created_at).toLocaleString()}
              </p>
            </div>
          </div>
        ))}
      </div>

      <div className="flex gap-2">
        <textarea
          placeholder="Type a reply..."
          value={replyBody}
          onChange={(e) => setReplyBody(e.target.value)}
          rows={2}
          className="flex-1 px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
        />
        <button
          onClick={() => onReply(ticket.id)}
          disabled={!replyBody.trim() || actionLoading}
          className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50 self-end"
        >
          Send
        </button>
      </div>
    </div>
  );
}

function TicketSelect({
  value,
  options,
  onChange,
  formatOptionLabel = (option) => option,
  ariaLabel,
}: {
  value: string;
  options: readonly string[];
  onChange: (value: string) => void;
  formatOptionLabel?: (option: string) => string;
  ariaLabel?: string;
}) {
  return (
    <select
      aria-label={ariaLabel}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="px-2 py-1 text-xs border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
    >
      {options.map((option) => (
        <option key={option} value={option}>
          {formatOptionLabel(option)}
        </option>
      ))}
    </select>
  );
}
