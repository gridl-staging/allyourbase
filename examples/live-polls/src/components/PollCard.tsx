import { useState } from "react";
import { ayb } from "../lib/ayb";
import type { Poll, PollOption, Vote } from "../types";

interface Props {
  poll: Poll;
  options: PollOption[];
  votes: Vote[];
  currentUserId: string | null;
  onClose: (pollId: string) => void;
  onVote?: (vote: Vote) => void;
}

const BAR_COLORS = [
  "bg-blue-500",
  "bg-green-500",
  "bg-yellow-500",
  "bg-purple-500",
  "bg-pink-500",
  "bg-cyan-500",
  "bg-orange-500",
  "bg-red-500",
  "bg-indigo-500",
  "bg-teal-500",
];

export default function PollCard({ poll, options, votes, currentUserId, onClose, onVote }: Props) {
  const [voting, setVoting] = useState(false);
  const [error, setError] = useState("");

  const totalVotes = votes.length;
  const myVote = currentUserId ? votes.find((v) => v.user_id === currentUserId) : null;
  const isOwner = currentUserId === poll.user_id;

  // Count votes per option.
  const voteCounts = new Map<string, number>();
  for (const v of votes) {
    voteCounts.set(v.option_id, (voteCounts.get(v.option_id) ?? 0) + 1);
  }

  // Sort options by position.
  const sorted = [...options].sort((a, b) => a.position - b.position);

  async function handleVote(optionId: string) {
    if (poll.is_closed || voting || !currentUserId) return;
    setError("");
    setVoting(true);
    try {
      // Use the records API (not the cast_vote RPC) so the server publishes an
      // SSE event that other connected users receive in realtime.
      let vote: Vote;
      if (myVote) {
        vote = await ayb.records.update<Vote>("votes", myVote.id, { option_id: optionId });
      } else {
        vote = await ayb.records.create<Vote>("votes", {
          poll_id: poll.id,
          option_id: optionId,
          user_id: currentUserId,
        });
      }
      onVote?.(vote);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Vote failed");
    } finally {
      setVoting(false);
    }
  }

  async function handleClose() {
    try {
      await ayb.records.update("polls", poll.id, { is_closed: true });
      onClose(poll.id);
    } catch {
      // Ignore — realtime will sync.
    }
  }

  return (
    <div data-testid="poll-card" className="bg-gray-900 border border-gray-700 rounded-xl p-5">
      <div className="flex justify-between items-start mb-3">
        <h3 className="text-lg font-semibold">{poll.question}</h3>
        <div className="flex items-center gap-2 flex-shrink-0">
          {poll.is_closed && (
            <span className="text-xs bg-gray-700 text-gray-300 px-2 py-0.5 rounded">Closed</span>
          )}
          {isOwner && !poll.is_closed && (
            <button
              onClick={handleClose}
              className="text-xs text-gray-400 hover:text-red-400"
            >
              Close poll
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        {sorted.map((opt, i) => {
          const count = voteCounts.get(opt.id) ?? 0;
          const pct = totalVotes > 0 ? (count / totalVotes) * 100 : 0;
          const isMyVote = myVote?.option_id === opt.id;
          const color = BAR_COLORS[i % BAR_COLORS.length];

          return (
            <button
              key={opt.id}
              onClick={() => handleVote(opt.id)}
              disabled={poll.is_closed || voting}
              className={`relative text-left rounded-lg overflow-hidden border transition-colors ${
                isMyVote
                  ? "border-blue-500 bg-gray-800"
                  : "border-gray-700 bg-gray-800 hover:border-gray-500"
              } ${poll.is_closed ? "cursor-default" : "cursor-pointer"}`}
            >
              <div
                className={`absolute inset-y-0 left-0 ${color} opacity-20 transition-all duration-500`}
                style={{ width: `${pct}%` }}
              />
              <div className="relative px-3 py-2 flex justify-between items-center">
                <span className="text-sm">
                  {isMyVote && <span className="mr-1">&#10003;</span>}
                  {opt.label}
                </span>
                <span className="text-xs text-gray-400 tabular-nums">
                  {count} {count === 1 ? "vote" : "votes"} ({Math.round(pct)}%)
                </span>
              </div>
            </button>
          );
        })}
      </div>

      <div className="mt-2 flex justify-between items-center">
        <span className="text-xs text-gray-500">
          {totalVotes} total {totalVotes === 1 ? "vote" : "votes"}
        </span>
        {error && <span className="text-xs text-red-400">{error}</span>}
      </div>
    </div>
  );
}
