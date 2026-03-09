/**
 * Unit tests for the App component's handleRealtime callback.
 *
 * Strategy: render App with all API calls mocked to return empty lists, capture
 * the realtime subscription callback from the subscribe mock, then fire events
 * directly into it and assert on the resulting DOM output.
 *
 * This covers the deduplication logic which has no other unit-level coverage —
 * the e2e tests exercise it end-to-end but can't test edge cases like the
 * optimistic-update dedup race in the votes 'create' branch.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act, waitFor, fireEvent } from "@testing-library/react";
import App from "../src/App";
import { ayb } from "../src/lib/ayb";
import type { RealtimeEvent } from "../src/hooks/useRealtime";
import type { Poll, PollOption, Vote } from "../src/types";

// ── Mock setup ───────────────────────────────────────────────────────────────

let capturedRealtimeCallback: ((event: RealtimeEvent) => void) | undefined;

vi.mock("../src/lib/ayb", () => ({
  ayb: {
    auth: {
      me: vi.fn().mockResolvedValue({ id: "user-current", email: "me@test.com" }),
    },
    records: {
      list: vi.fn().mockResolvedValue({ items: [], page: 1, perPage: 500, totalItems: 0, totalPages: 0 }),
      // PollCard's handleClose calls records.update when the owner closes a poll.
      // The mock must exist so the component doesn't throw, even in tests that
      // don't exercise that path.
      update: vi.fn().mockResolvedValue({}),
      // CreatePoll calls records.create when the user submits a new poll.
      // Required by the handlePollCreated dedup test; other tests don't use it.
      create: vi.fn().mockResolvedValue({}),
    },
    realtime: {
      subscribe: vi.fn((_tables, cb) => {
        capturedRealtimeCallback = cb;
        return vi.fn(); // unsub
      }),
    },
  },
  isLoggedIn: vi.fn().mockReturnValue(true),
  clearPersistedTokens: vi.fn(),
  persistTokens: vi.fn(),
}));

const mockList = vi.mocked(ayb.records.list);
const mockCreate = vi.mocked(ayb.records.create);

// ── Fixtures ─────────────────────────────────────────────────────────────────

const POLL: Poll = {
  id: "poll-1",
  user_id: "user-owner",
  question: "Best language?",
  is_closed: false,
  created_at: "2026-01-01T00:00:00Z",
};

const OPT_A: PollOption = { id: "opt-a", poll_id: "poll-1", label: "TypeScript", position: 0 };
const OPT_B: PollOption = { id: "opt-b", poll_id: "poll-1", label: "Go", position: 1 };

function vote(id: string, userId: string, optionId: string): Vote {
  return {
    id,
    poll_id: "poll-1",
    option_id: optionId,
    user_id: userId,
    created_at: "2026-01-01T00:00:00Z",
  };
}

// Helper: fire a realtime event into the captured subscription callback.
// Uses async act() so React 18 fully flushes all batched state updates and
// pending effects before the next assertion runs.
async function fire(event: RealtimeEvent) {
  await act(async () => {
    capturedRealtimeCallback!(event);
  });
}

// Helper: alias for the Record cast required by the event type.
function rec<T>(obj: T): Record<string, unknown> {
  return obj as unknown as Record<string, unknown>;
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("App handleRealtime", () => {
  beforeEach(() => {
    // vi.clearAllMocks() resets call history and removes any per-test
    // mockResolvedValueOnce queues so they don't bleed into the next test.
    // It does NOT reset mock implementations set with mockReturnValue /
    // mockResolvedValue, so we re-establish defaults explicitly below.
    vi.clearAllMocks();
    capturedRealtimeCallback = undefined;

    // Re-apply default implementations after clearing.
    mockList.mockResolvedValue({ items: [], page: 1, perPage: 500, totalItems: 0, totalPages: 0 });
    vi.mocked(ayb.auth.me).mockResolvedValue({ id: "user-current", email: "me@test.com" });
    vi.mocked(ayb.records.update).mockResolvedValue({});
    // Default for create: return an empty object. Tests that exercise the
    // handlePollCreated path override this with mockResolvedValueOnce.
    mockCreate.mockResolvedValue({} as never);
    vi.mocked(ayb.realtime.subscribe).mockImplementation((_tables, cb) => {
      capturedRealtimeCallback = cb;
      return vi.fn();
    });
  });

  // ── polls table ──────────────────────────────────────────────────────────

  it("create event adds a new poll to the list", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });

    expect(await screen.findByText("Best language?")).toBeInTheDocument();
  });

  it("create event does not duplicate a poll that already exists", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "polls", record: rec(POLL) });

    await screen.findByText("Best language?");
    // Only one poll card should be rendered.
    expect(screen.getAllByText("Best language?")).toHaveLength(1);
  });

  it("update event replaces the poll in the list", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await screen.findByText("Best language?");

    await fire({ action: "update", table: "polls", record: rec({ ...POLL, question: "Changed?" }) });

    expect(await screen.findByText("Changed?")).toBeInTheDocument();
    expect(screen.queryByText("Best language?")).not.toBeInTheDocument();
  });

  it("delete event removes the poll from the list", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await screen.findByText("Best language?");

    await fire({ action: "delete", table: "polls", record: rec(POLL) });

    await waitFor(() =>
      expect(screen.queryByText("Best language?")).not.toBeInTheDocument(),
    );
  });

  it("update event marks a poll as closed (is_closed=true) via SSE", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });
    await screen.findByText("Best language?");

    await fire({ action: "update", table: "polls", record: rec({ ...POLL, is_closed: true }) });

    expect(await screen.findByText("Closed")).toBeInTheDocument();
  });

  // ── poll_options table ───────────────────────────────────────────────────

  it("create event adds an option to the poll card", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });

    expect(await screen.findByText("TypeScript")).toBeInTheDocument();
  });

  it("create event does not duplicate an option with the same id", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) }); // duplicate

    await screen.findByText("TypeScript");
    expect(screen.getAllByText("TypeScript")).toHaveLength(1);
  });

  // ── votes table ──────────────────────────────────────────────────────────

  it("vote create event increments the total vote count", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });
    await screen.findByText("0 total votes");

    await fire({ action: "create", table: "votes", record: rec(vote("v1", "user-other", "opt-a")) });

    expect(await screen.findByText("1 total vote")).toBeInTheDocument();
  });

  it("vote create event deduplicates against optimistic update (same user_id → no double-count)", async () => {
    // This is the critical race-condition guard in handleRealtime:
    // When the current user votes, handleVoteCast runs an optimistic update (adds
    // the vote to local state). Shortly after, the SSE 'create' event arrives for
    // the same vote. The dedup by user_id must REPLACE the local copy rather than
    // pushing a second entry — keeping the total at 1, not 2.
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });
    await screen.findByText("0 total votes");

    // Simulate the optimistic local copy (id="v-local").
    await fire({ action: "create", table: "votes", record: rec(vote("v-local", "user-current", "opt-a")) });
    expect(await screen.findByText("1 total vote")).toBeInTheDocument();

    // SSE 'create' arrives — same user, server-assigned id.
    await fire({ action: "create", table: "votes", record: rec(vote("v-server", "user-current", "opt-a")) });

    // Positive assertion first: count is still 1.
    expect(screen.getByText("1 total vote")).toBeInTheDocument();
    // Negative constraint: "2 total votes" must never appear.
    expect(screen.queryByText("2 total votes")).not.toBeInTheDocument();
  });

  it("vote update event moves a vote from one option to another", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_B) });
    await screen.findByText("0 total votes");

    // User casts vote for TypeScript (opt-a).
    const initialVote = vote("v1", "user-other", "opt-a");
    await fire({ action: "create", table: "votes", record: rec(initialVote) });
    await screen.findByText("1 total vote");

    // User changes vote to Go (opt-b) — SSE fires 'update' with same id but new option_id.
    await fire({ action: "update", table: "votes", record: rec({ ...initialVote, option_id: "opt-b" }) });

    // TypeScript (opt-a) should now show 0 votes, Go (opt-b) should show 1.
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /TypeScript/ }).textContent).toContain("0 votes");
    });
    expect(screen.getByRole("button", { name: /Go/ }).textContent).toContain("1 vote");
    // Total stays at 1 — it's a change, not a new vote.
    expect(screen.getByText("1 total vote")).toBeInTheDocument();
  });

  it("vote delete event removes the vote from the count", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await fire({ action: "create", table: "poll_options", record: rec(OPT_A) });
    await screen.findByText("0 total votes");

    const v = vote("v1", "user-other", "opt-a");
    await fire({ action: "create", table: "votes", record: rec(v) });
    await screen.findByText("1 total vote");

    await fire({ action: "delete", table: "votes", record: rec(v) });

    await waitFor(() =>
      expect(screen.getByText("0 total votes")).toBeInTheDocument(),
    );
  });

  // ── handleLogout ─────────────────────────────────────────────────────────
  // The session-193 data-leak fix clears polls, optionsMap, and votesMap on
  // logout so that a second user on the same browser session never briefly
  // sees the first user's poll data.
  // Bug history: prior to session-193 the logout only called clearPersistedTokens()
  // and set authed=false — the Map state was NOT cleared, causing state to linger.

  // ── handlePollCreated ─────────────────────────────────────────────────────
  // handlePollCreated is called by CreatePoll's onCreated callback after the
  // user submits a new poll through the UI.  The same dedup guard that
  // protects the realtime "create" path (`prev.find(p => p.id === poll.id) ?
  // prev : [poll, ...prev]`) also protects this path, because the SSE event
  // for the new poll can arrive BEFORE handlePollCreated fires (network timing).
  // Without the guard both paths would push the same poll and produce a
  // duplicate card.

  it("handlePollCreated does not add a duplicate when SSE already delivered the poll", async () => {
    // Mock records.create: first call creates the poll, subsequent calls create options.
    mockCreate
      .mockResolvedValueOnce(POLL as never)
      .mockResolvedValueOnce(OPT_A as never)
      .mockResolvedValueOnce(OPT_B as never);

    render(<App />);

    // Wait for auth.me to resolve — userId must be set before CreatePoll renders.
    // The user-email span appears only after setUserId/setUserEmail settle.
    await screen.findByTestId("user-email");
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    // Simulate the SSE "create" event arriving FIRST (before the form submit).
    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await screen.findByText("Best language?");

    // Now the user also submits the same poll through the UI (simulating the
    // race where the SSE beat the onCreated callback).
    fireEvent.click(screen.getByRole("button", { name: "+ New Poll" }));
    await screen.findByRole("heading", { name: "New Poll" });

    fireEvent.change(screen.getByPlaceholderText("Ask a question..."), {
      target: { value: POLL.question },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 1"), {
      target: { value: OPT_A.label },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 2"), {
      target: { value: OPT_B.label },
    });
    fireEvent.submit(
      screen.getByRole("button", { name: "Create Poll" }).closest("form")!,
    );

    // handlePollCreated sets showCreate=false — wait for the form to close.
    await waitFor(() =>
      expect(
        screen.queryByRole("heading", { name: "New Poll" }),
      ).not.toBeInTheDocument(),
    );

    // Only ONE poll card — the dedup guard prevented a double-add.
    expect(screen.getAllByText("Best language?")).toHaveLength(1);
  });

  it("sign out clears all poll state and renders the auth form", async () => {
    render(<App />);
    await waitFor(() => expect(capturedRealtimeCallback).toBeDefined());

    // Add a poll via SSE so there is visible data to be cleared on logout.
    await fire({ action: "create", table: "polls", record: rec(POLL) });
    await screen.findByText("Best language?");

    // Click the "Sign out" button.
    fireEvent.click(screen.getByText("Sign out"));

    // Poll list must be cleared immediately (synchronous state reset).
    await waitFor(() =>
      expect(screen.queryByText("Best language?")).not.toBeInTheDocument(),
    );

    // AuthForm renders when authed=false — the email input is the smoke-test.
    expect(screen.getByPlaceholderText("Email")).toBeInTheDocument();
  });
});
