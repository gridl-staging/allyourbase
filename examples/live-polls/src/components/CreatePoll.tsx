import { FormEvent, useState } from "react";
import { ayb } from "../lib/ayb";
import type { Poll, PollOption } from "../types";

interface Props {
  userId: string;
  onCreated: (poll: Poll, options: PollOption[]) => void;
}

export default function CreatePoll({ userId, onCreated }: Props) {
  const [question, setQuestion] = useState("");
  const [options, setOptions] = useState(["", ""]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  function addOption() {
    if (options.length < 10) {
      setOptions([...options, ""]);
    }
  }

  function removeOption(index: number) {
    if (options.length > 2) {
      setOptions(options.filter((_, i) => i !== index));
    }
  }

  function updateOption(index: number, value: string) {
    setOptions(options.map((o, i) => (i === index ? value : o)));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");

    const trimmedOptions = options.map((o) => o.trim()).filter(Boolean);
    if (trimmedOptions.length < 2) {
      setError("At least 2 options required");
      return;
    }

    setLoading(true);
    try {
      const poll = await ayb.records.create<Poll>("polls", {
        question: question.trim(),
        user_id: userId,
      });

      const createdOptions = await Promise.all(
        trimmedOptions.map((label, i) =>
          ayb.records.create<PollOption>("poll_options", {
            poll_id: poll.id,
            label,
            position: i,
          }),
        ),
      );

      setQuestion("");
      setOptions(["", ""]);
      onCreated(poll, createdOptions);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create poll");
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} aria-label="New Poll" className="bg-gray-900 border border-gray-700 rounded-xl p-5">
      <h2 className="text-lg font-bold mb-3">New Poll</h2>
      <input
        type="text"
        placeholder="Ask a question..."
        value={question}
        onChange={(e) => setQuestion(e.target.value)}
        className="w-full bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm mb-3 focus:outline-none focus:border-blue-500"
        required
      />
      <div className="flex flex-col gap-2 mb-3">
        {options.map((opt, i) => (
          <div key={i} className="flex gap-2">
            <input
              type="text"
              placeholder={`Option ${i + 1}`}
              value={opt}
              onChange={(e) => updateOption(i, e.target.value)}
              className="flex-1 bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-sm focus:outline-none focus:border-blue-500"
            />
            {options.length > 2 && (
              <button
                type="button"
                onClick={() => removeOption(i)}
                className="text-gray-500 hover:text-red-400 text-sm px-2"
              >
                x
              </button>
            )}
          </div>
        ))}
      </div>
      {options.length < 10 && (
        <button
          type="button"
          onClick={addOption}
          className="text-blue-400 text-sm hover:underline mb-3 block"
        >
          + Add option
        </button>
      )}
      {error && <p className="text-red-400 text-xs mb-2">{error}</p>}
      <button
        type="submit"
        disabled={loading}
        className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 rounded py-2 px-4 text-sm font-semibold"
      >
        {loading ? "Creating..." : "Create Poll"}
      </button>
    </form>
  );
}
