import { useCallback, useEffect, useState } from "react";
import AuthForm from "./components/AuthForm";
import CreatePoll from "./components/CreatePoll";
import PollCard from "./components/PollCard";
import { useRealtime, type RealtimeEvent } from "./hooks/useRealtime";
import { ayb, isLoggedIn, clearPersistedTokens } from "./lib/ayb";
import type { Poll, PollOption, Vote } from "./types";

/** Fetch all records from a collection, paginating through server limits. */
async function fetchAll<T>(table: string, opts: Record<string, unknown> = {}): Promise<T[]> {
  const PAGE_SIZE = 500; // server max
  const items: T[] = [];
  let page = 1;
  for (;;) {
    const res = await ayb.records.list<T>(table, { ...opts, page, perPage: PAGE_SIZE });
    items.push(...res.items);
    if (res.items.length < PAGE_SIZE) break;
    page++;
  }
  return items;
}

export default function App() {
  const [authed, setAuthed] = useState(isLoggedIn());
  const [userId, setUserId] = useState<string | null>(null);
  const [userEmail, setUserEmail] = useState<string | null>(null);
  const [polls, setPolls] = useState<Poll[]>([]);
  const [optionsMap, setOptionsMap] = useState<Map<string, PollOption[]>>(new Map());
  const [votesMap, setVotesMap] = useState<Map<string, Vote[]>>(new Map());
  const [showCreate, setShowCreate] = useState(false);

  // Load current user on mount.
  useEffect(() => {
    if (authed) {
      ayb.auth.me().then((u) => {
        setUserId(u.id);
        setUserEmail(u.email ?? null);
      }).catch(() => {});
    }
  }, [authed]);

  // Load all polls, options, and votes.
  useEffect(() => {
    if (!authed) return;
    async function load() {
      try {
        const [pollRes, allOpts, allVotes] = await Promise.all([
          ayb.records.list<Poll>("polls", { sort: "-created_at", perPage: 100 }),
          fetchAll<PollOption>("poll_options"),
          fetchAll<Vote>("votes"),
        ]);
        setPolls(pollRes.items);

        const om = new Map<string, PollOption[]>();
        for (const o of allOpts) {
          const arr = om.get(o.poll_id) ?? [];
          arr.push(o);
          om.set(o.poll_id, arr);
        }
        // Merge with existing state to avoid discarding SSE-delivered data.
        setOptionsMap((prev) => {
          const next = new Map(prev);
          for (const [pollId, opts] of om) {
            next.set(pollId, opts);
          }
          return next;
        });

        const vm = new Map<string, Vote[]>();
        for (const v of allVotes) {
          const arr = vm.get(v.poll_id) ?? [];
          arr.push(v);
          vm.set(v.poll_id, arr);
        }
        setVotesMap((prev) => {
          const next = new Map(prev);
          for (const [pollId, votes] of vm) {
            next.set(pollId, votes);
          }
          return next;
        });
      } catch {
        // Polls load empty if server unavailable.
      }
    }
    load();
  }, [authed]);

  // Realtime: update votes, polls, options on changes.
  const handleRealtime = useCallback((event: RealtimeEvent) => {
    if (event.table === "votes") {
      const vote = event.record as unknown as Vote;
      setVotesMap((prev) => {
        const next = new Map(prev);
        const arr = [...(next.get(vote.poll_id) ?? [])];
        if (event.action === "create") {
          // Dedup against the optimistic update from handleVoteCast, which may
          // have added this vote to local state before the SSE "create" arrives.
          // (Vote changes fire "update", not "create", so this only deduplicates
          // the first-vote optimistic path.)
          const idx = arr.findIndex((v) => v.user_id === vote.user_id);
          if (idx >= 0) {
            arr[idx] = vote;
          } else {
            arr.push(vote);
          }
        } else if (event.action === "update") {
          const idx = arr.findIndex((v) => v.id === vote.id);
          if (idx >= 0) {
            arr[idx] = vote;
          } else {
            // Vote wasn't in local state yet (e.g. load() hadn't included it).
            arr.push(vote);
          }
        } else if (event.action === "delete") {
          const filtered = arr.filter((v) => v.id !== vote.id);
          next.set(vote.poll_id, filtered);
          return next;
        }
        next.set(vote.poll_id, arr);
        return next;
      });
    }

    if (event.table === "polls") {
      const poll = event.record as unknown as Poll;
      if (event.action === "create") {
        setPolls((prev) => (prev.find((p) => p.id === poll.id) ? prev : [poll, ...prev]));
      } else if (event.action === "update") {
        setPolls((prev) => prev.map((p) => (p.id === poll.id ? poll : p)));
      } else if (event.action === "delete") {
        setPolls((prev) => prev.filter((p) => p.id !== poll.id));
      }
    }

    if (event.table === "poll_options") {
      const opt = event.record as unknown as PollOption;
      if (event.action === "create") {
        setOptionsMap((prev) => {
          const next = new Map(prev);
          const arr = [...(next.get(opt.poll_id) ?? [])];
          if (!arr.find((o) => o.id === opt.id)) arr.push(opt);
          next.set(opt.poll_id, arr);
          return next;
        });
      }
    }
  }, []);

  // Only subscribe to realtime when authenticated — the EventSource URL includes
  // the JWT token, which is not available until after login. Passing an empty
  // array when logged out prevents creating an unauthenticated SSE connection
  // that would get a 401 and never be replaced after login (effect deps unchanged).
  useRealtime(authed ? ["polls", "poll_options", "votes"] : [], handleRealtime);

  function handlePollCreated(poll: Poll, options: PollOption[]) {
    // Guard against the SSE create event arriving before this callback — if the
    // poll was already added via realtime, skip the duplicate rather than
    // prepending it again.
    setPolls((prev) => (prev.find((p) => p.id === poll.id) ? prev : [poll, ...prev]));
    setOptionsMap((prev) => {
      const next = new Map(prev);
      next.set(poll.id, options);
      return next;
    });
    setShowCreate(false);
  }

  function handleClosePoll(pollId: string) {
    setPolls((prev) => prev.map((p) => (p.id === pollId ? { ...p, is_closed: true } : p)));
  }

  function handleVoteCast(vote: Vote) {
    setVotesMap((prev) => {
      const next = new Map(prev);
      const arr = [...(next.get(vote.poll_id) ?? [])];
      const idx = arr.findIndex((v) => v.user_id === vote.user_id);
      if (idx >= 0) {
        arr[idx] = vote;
      } else {
        arr.push(vote);
      }
      next.set(vote.poll_id, arr);
      return next;
    });
  }

  function handleLogout() {
    clearPersistedTokens();
    setAuthed(false);
    setUserId(null);
    setUserEmail(null);
    setPolls([]);
    setOptionsMap(new Map());
    setVotesMap(new Map());
  }

  if (!authed) {
    return <AuthForm onAuth={() => setAuthed(true)} />;
  }

  return (
    <div className="min-h-screen">
      <header className="border-b border-gray-800 px-4 py-3 flex justify-between items-center">
        <h1 className="text-xl font-bold">Live Polls</h1>
        <div className="flex gap-3 items-center">
          {userEmail && (
            <span data-testid="user-email" className="text-xs text-gray-500 hidden sm:block">
              {userEmail}
            </span>
          )}
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="bg-blue-600 hover:bg-blue-500 rounded px-3 py-1.5 text-sm font-semibold"
          >
            {showCreate ? "Cancel" : "+ New Poll"}
          </button>
          <button
            onClick={handleLogout}
            className="text-gray-400 hover:text-white text-sm"
          >
            Sign out
          </button>
        </div>
      </header>

      <main className="max-w-2xl mx-auto p-4 flex flex-col gap-4">
        {showCreate && userId && <CreatePoll userId={userId} onCreated={handlePollCreated} />}

        {polls.length === 0 && !showCreate && (
          <div className="text-center text-gray-500 py-12">
            <p className="text-lg mb-2">No polls yet</p>
            <p className="text-sm">Create the first one!</p>
          </div>
        )}

        {polls.map((poll) => (
          <PollCard
            key={poll.id}
            poll={poll}
            options={optionsMap.get(poll.id) ?? []}
            votes={votesMap.get(poll.id) ?? []}
            currentUserId={userId}
            onClose={handleClosePoll}
            onVote={handleVoteCast}
          />
        ))}
      </main>
    </div>
  );
}
