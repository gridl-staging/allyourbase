import { useState, useEffect, useCallback } from "react";
import type { SMSMessage } from "../types";
import { listAdminSMSMessages } from "../api";
import { cn } from "../lib/utils";
import { Loader2, AlertCircle, MessageSquare } from "lucide-react";
import { SMSSendTester } from "./SMSSendTester";

function statusBadgeClass(status: string): string {
  if (["delivered", "sent"].includes(status)) {
    return "bg-green-100 text-green-700";
  }
  if (["failed", "undelivered", "canceled"].includes(status)) {
    return "bg-red-100 text-red-700";
  }
  return "bg-yellow-100 text-yellow-700";
}

function truncateBody(body: string, max = 60): string {
  return body.length > max ? body.slice(0, max) + "…" : body;
}

export function SMSMessages() {
  const [messages, setMessages] = useState<SMSMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [showSendModal, setShowSendModal] = useState(false);

  const fetchMessages = useCallback(async (p: number) => {
    try {
      setError(null);
      setLoading(true);
      const res = await listAdminSMSMessages({ page: p });
      setMessages(res.items);
      setPage(res.page);
      setTotalPages(res.totalPages);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load messages");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchMessages(1);
  }, [fetchMessages]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => fetchMessages(page)}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (messages.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">SMS Messages</h2>
          <button
            data-testid="open-send-modal"
            onClick={() => setShowSendModal(true)}
            className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Send SMS
          </button>
        </div>
        <div className="flex flex-col items-center justify-center h-48 text-gray-400">
          <MessageSquare className="w-8 h-8 mb-2" />
          No messages sent yet
        </div>
        {showSendModal && (
          <SMSSendTester
            onClose={() => setShowSendModal(false)}
            onSent={() => fetchMessages(page)}
          />
        )}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">SMS Messages</h2>
        <button
          data-testid="open-send-modal"
          onClick={() => setShowSendModal(true)}
          className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          Send SMS
        </button>
      </div>

      <table data-testid="sms-messages-table" className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-gray-500">
            <th className="pb-2 font-medium">To</th>
            <th className="pb-2 font-medium">Body</th>
            <th className="pb-2 font-medium">Provider</th>
            <th className="pb-2 font-medium">Status</th>
            <th className="pb-2 font-medium">Sent At</th>
            <th className="pb-2 font-medium">Error</th>
          </tr>
        </thead>
        <tbody>
          {messages.map((msg) => (
            <tr
              key={msg.id}
              data-testid={`sms-row-${msg.id}`}
              className="border-b"
            >
              <td className="py-2">{msg.to}</td>
              <td className="py-2">{truncateBody(msg.body)}</td>
              <td className="py-2">{msg.provider}</td>
              <td className="py-2">
                <span
                  data-testid={`status-badge-${msg.status}`}
                  className={cn(
                    "px-2 py-0.5 rounded text-xs font-medium",
                    statusBadgeClass(msg.status),
                  )}
                >
                  {msg.status}
                </span>
              </td>
              <td className="py-2">
                {new Date(msg.created_at).toLocaleString()}
              </td>
              <td className="py-2 text-red-600">
                {msg.error_message || ""}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-4 pt-2">
          <button
            data-testid="pagination-prev"
            onClick={() => fetchMessages(page - 1)}
            disabled={page === 1}
            className="px-3 py-1 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Prev
          </button>
          <span className="text-sm text-gray-600">
            Page {page} of {totalPages}
          </span>
          <button
            data-testid="pagination-next"
            onClick={() => fetchMessages(page + 1)}
            disabled={page === totalPages}
            className="px-3 py-1 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Next
          </button>
        </div>
      )}

      {showSendModal && (
          <SMSSendTester
            onClose={() => setShowSendModal(false)}
            onSent={() => fetchMessages(page)}
          />
        )}
    </div>
  );
}
