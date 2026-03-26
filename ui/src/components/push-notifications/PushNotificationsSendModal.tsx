import type { SendFormState } from "./models";

interface PushNotificationsSendModalProps {
  open: boolean;
  sending: boolean;
  sendForm: SendFormState;
  setSendForm: React.Dispatch<React.SetStateAction<SendFormState>>;
  onClose: () => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function PushNotificationsSendModal({
  open,
  sending,
  sendForm,
  setSendForm,
  onClose,
  onSubmit,
}: PushNotificationsSendModalProps) {
  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
      <div className="w-full max-w-lg rounded-lg bg-white dark:bg-gray-800 border shadow-lg p-4">
        <h2 className="text-base font-semibold mb-3">Send Test Push</h2>
        <form onSubmit={onSubmit} className="space-y-3">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label htmlFor="push-send-app-id" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                App ID
              </label>
              <input
                id="push-send-app-id"
                aria-label="App ID"
                value={sendForm.app_id}
                onChange={(event) => setSendForm((prev) => ({ ...prev, app_id: event.target.value }))}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label htmlFor="push-send-user-id" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                User ID
              </label>
              <input
                id="push-send-user-id"
                aria-label="User ID"
                value={sendForm.user_id}
                onChange={(event) => setSendForm((prev) => ({ ...prev, user_id: event.target.value }))}
                className="w-full border rounded px-3 py-2 text-sm"
              />
            </div>
          </div>

          <div>
            <label htmlFor="push-send-title" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Title
            </label>
            <input
              id="push-send-title"
              aria-label="Title"
              value={sendForm.title}
              onChange={(event) => setSendForm((prev) => ({ ...prev, title: event.target.value }))}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="push-send-body" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Body
            </label>
            <textarea
              id="push-send-body"
              aria-label="Body"
              value={sendForm.body}
              onChange={(event) => setSendForm((prev) => ({ ...prev, body: event.target.value }))}
              rows={3}
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="push-send-data" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Data (JSON)
            </label>
            <textarea
              id="push-send-data"
              aria-label="Data (JSON)"
              value={sendForm.dataJSON}
              onChange={(event) => setSendForm((prev) => ({ ...prev, dataJSON: event.target.value }))}
              rows={4}
              className="w-full border rounded px-3 py-2 text-sm font-mono"
            />
          </div>

          <div className="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={sending}
              className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-70"
            >
              Send Push
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
