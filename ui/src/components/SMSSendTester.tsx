import { useState } from "react";
import type { SMSSendResponse } from "../types";
import { adminSendSMS } from "../api";
import { X } from "lucide-react";

interface SMSSendTesterProps {
  onClose: () => void;
  onSent?: () => void;
}

export function SMSSendTester({ onClose, onSent }: SMSSendTesterProps) {
  const [phone, setPhone] = useState("");
  const [body, setBody] = useState("");
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<SMSSendResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  const canSend = phone.trim() !== "" && body.trim() !== "" && !sending;

  async function handleSend() {
    try {
      setError(null);
      setSending(true);
      const res = await adminSendSMS(phone, body);
      setResult(res);
      setPhone("");
      setBody("");
      onSent?.();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to send SMS");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div className="bg-white rounded-lg shadow-lg w-full max-w-md p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold">Send Test SMS</h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <label htmlFor="sms-phone" className="block text-sm font-medium text-gray-700 mb-1">
              To (phone number)
            </label>
            <input
              id="sms-phone"
              type="tel"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              placeholder="+1234567890"
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="sms-body" className="block text-sm font-medium text-gray-700 mb-1">
              Message body
            </label>
            <textarea
              id="sms-body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder="Enter message text..."
              rows={3}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>

          {result && (
            <div
              data-testid="send-result"
              className="bg-green-50 border border-green-200 rounded p-3 text-sm"
            >
              <p className="font-medium text-green-800">Message sent</p>
              {result.id && <p className="text-green-700">ID: {result.id}</p>}
              <p className="text-green-700">To: {result.to}</p>
              <p className="text-green-700">Status: {result.status}</p>
            </div>
          )}

          {error && (
            <div
              data-testid="send-error"
              className="bg-red-50 border border-red-200 rounded p-3 text-sm text-red-700"
            >
              {error}
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
            >
              Cancel
            </button>
            <button
              onClick={handleSend}
              disabled={!canSend}
              className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {sending ? "Sending..." : "Send"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
