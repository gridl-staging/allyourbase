import { useState } from "react";
import { createNotification } from "../api_notifications";

export function Notifications() {
  const [userId, setUserId] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [channel, setChannel] = useState("");
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const formValid = userId.length > 0 && title.length > 0 && channel.length > 0;

  const handleSubmit = async () => {
    setLoading(true);
    setSuccess(false);
    setSubmitError(null);
    try {
      await createNotification({
        user_id: userId,
        title,
        body: body || undefined,
        channel,
      });
      setSuccess(true);
      setUserId("");
      setTitle("");
      setBody("");
      setChannel("");
    } catch {
      setSubmitError("Failed to send notification.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Notifications
      </h2>

      <div className="max-w-md space-y-3">
        <label className="block text-xs text-gray-600 dark:text-gray-400">
          User ID
          <input
            type="text"
            value={userId}
            onChange={(e) => setUserId(e.target.value)}
            className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          />
        </label>
        <label className="block text-xs text-gray-600 dark:text-gray-400">
          Title
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          />
        </label>
        <label className="block text-xs text-gray-600 dark:text-gray-400">
          Body
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={3}
            className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          />
        </label>
        <label className="block text-xs text-gray-600 dark:text-gray-400">
          Channel
          <input
            type="text"
            value={channel}
            onChange={(e) => setChannel(e.target.value)}
            placeholder="email, push, sms"
            className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          />
        </label>

        <button
          onClick={handleSubmit}
          disabled={!formValid || loading}
          className="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
        >
          Send Notification
        </button>

        {success && (
          <p role="status" className="text-sm text-green-600 dark:text-green-400">
            Notification sent successfully.
          </p>
        )}
        {submitError && (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            {submitError}
          </p>
        )}
      </div>
    </div>
  );
}
