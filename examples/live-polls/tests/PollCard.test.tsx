import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import PollCard from "../src/components/PollCard";
import { ayb } from "../src/lib/ayb";
import type { Poll, PollOption, Vote } from "../src/types";

// Mock the ayb module — include both create (first vote) and update (vote change).
vi.mock("../src/lib/ayb", () => ({
  ayb: {
    records: {
      create: vi.fn(),
      update: vi.fn(),
    },
  },
}));

const mockCreate = vi.mocked(ayb.records.create);
const mockUpdate = vi.mocked(ayb.records.update);

const poll: Poll = {
  id: "poll-1",
  user_id: "user-owner",
  question: "What is the best language?",
  is_closed: false,
  created_at: "2026-02-09T00:00:00Z",
};

const options: PollOption[] = [
  { id: "opt-a", poll_id: "poll-1", label: "TypeScript", position: 0 },
  { id: "opt-b", poll_id: "poll-1", label: "Go", position: 1 },
  { id: "opt-c", poll_id: "poll-1", label: "Rust", position: 2 },
];

function makeVotes(counts: Record<string, number>): Vote[] {
  const votes: Vote[] = [];
  let userIdx = 0;
  for (const [optionId, count] of Object.entries(counts)) {
    for (let i = 0; i < count; i++) {
      votes.push({
        id: `vote-${userIdx}`,
        poll_id: "poll-1",
        option_id: optionId,
        user_id: `user-${userIdx}`,
        created_at: "2026-02-09T00:00:00Z",
      });
      userIdx++;
    }
  }
  return votes;
}

describe("PollCard", () => {
  beforeEach(() => {
    mockCreate.mockReset();
    mockUpdate.mockReset();
  });

  // ── Rendering ──────────────────────────────────────────────────────────────

  it("renders the poll question", () => {
    render(
      <PollCard poll={poll} options={options} votes={[]} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("What is the best language?")).toBeInTheDocument();
  });

  it("renders all options", () => {
    render(
      <PollCard poll={poll} options={options} votes={[]} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("TypeScript")).toBeInTheDocument();
    expect(screen.getByText("Go")).toBeInTheDocument();
    expect(screen.getByText("Rust")).toBeInTheDocument();
  });

  it("shows vote counts and percentages", () => {
    const votes = makeVotes({ "opt-a": 3, "opt-b": 1 });
    render(
      <PollCard poll={poll} options={options} votes={votes} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("3 votes (75%)")).toBeInTheDocument();
    expect(screen.getByText("1 vote (25%)")).toBeInTheDocument();
    expect(screen.getByText("0 votes (0%)")).toBeInTheDocument();
    expect(screen.getByText("4 total votes")).toBeInTheDocument();
  });

  it("shows zero votes for empty poll", () => {
    render(
      <PollCard poll={poll} options={options} votes={[]} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("0 total votes")).toBeInTheDocument();
  });

  it("shows checkmark on user's voted option", () => {
    const votes = makeVotes({ "opt-a": 1 });
    // user-0 voted for opt-a (from makeVotes).
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={votes}
        currentUserId="user-0"
        onClose={vi.fn()}
      />,
    );
    // The checkmark character is rendered before the label.
    const tsOption = screen.getByText(/TypeScript/);
    expect(tsOption.textContent).toContain("\u2713");
  });

  it("shows Closed badge when poll is closed", () => {
    const closedPoll = { ...poll, is_closed: true };
    render(
      <PollCard
        poll={closedPoll}
        options={options}
        votes={[]}
        currentUserId={null}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByText("Closed")).toBeInTheDocument();
  });

  it("shows close button for poll owner", () => {
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-owner"
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByText("Close poll")).toBeInTheDocument();
  });

  it("hides close button for non-owners", () => {
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-other"
        onClose={vi.fn()}
      />,
    );
    expect(screen.queryByText("Close poll")).not.toBeInTheDocument();
  });

  it("options are sorted by position", () => {
    const reversed: PollOption[] = [
      { id: "opt-c", poll_id: "poll-1", label: "Rust", position: 2 },
      { id: "opt-a", poll_id: "poll-1", label: "TypeScript", position: 0 },
      { id: "opt-b", poll_id: "poll-1", label: "Go", position: 1 },
    ];
    render(
      <PollCard
        poll={poll}
        options={reversed}
        votes={[]}
        currentUserId={null}
        onClose={vi.fn()}
      />,
    );
    const buttons = screen.getAllByRole("button").filter((b) => b.textContent?.match(/votes/));
    expect(buttons[0].textContent).toContain("TypeScript");
    expect(buttons[1].textContent).toContain("Go");
    expect(buttons[2].textContent).toContain("Rust");
  });

  // ── Vote interactions ──────────────────────────────────────────────────────

  it("calls records.create and onVote when casting a first vote", async () => {
    const newVote: Vote = {
      id: "vote-new",
      poll_id: "poll-1",
      option_id: "opt-a",
      user_id: "user-current",
      created_at: "2026-02-09T00:00:00Z",
    };
    mockCreate.mockResolvedValueOnce(newVote);
    const onVote = vi.fn();

    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-current"
        onClose={vi.fn()}
        onVote={onVote}
      />,
    );

    // Click the TypeScript option (first vote — no existing vote to update).
    fireEvent.click(screen.getByRole("button", { name: /TypeScript/ }));

    await waitFor(() => expect(onVote).toHaveBeenCalledOnce());
    expect(onVote).toHaveBeenCalledWith(newVote);
    expect(mockCreate).toHaveBeenCalledWith("votes", {
      poll_id: "poll-1",
      option_id: "opt-a",
      user_id: "user-current",
    });
    // update should NOT have been called.
    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("calls records.update and onVote when changing an existing vote", async () => {
    const existingVote: Vote = {
      id: "vote-existing",
      poll_id: "poll-1",
      option_id: "opt-a",
      user_id: "user-current",
      created_at: "2026-02-09T00:00:00Z",
    };
    const updatedVote: Vote = { ...existingVote, option_id: "opt-b" };
    mockUpdate.mockResolvedValueOnce(updatedVote);
    const onVote = vi.fn();

    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[existingVote]}
        currentUserId="user-current"
        onClose={vi.fn()}
        onVote={onVote}
      />,
    );

    // Click "Go" to change vote from TypeScript → Go.
    fireEvent.click(screen.getByRole("button", { name: /Go/ }));

    await waitFor(() => expect(onVote).toHaveBeenCalledOnce());
    expect(onVote).toHaveBeenCalledWith(updatedVote);
    expect(mockUpdate).toHaveBeenCalledWith("votes", "vote-existing", {
      option_id: "opt-b",
    });
    // create should NOT have been called.
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("does not call any API when voting on a closed poll", () => {
    const closedPoll = { ...poll, is_closed: true };

    render(
      <PollCard
        poll={closedPoll}
        options={options}
        votes={[]}
        currentUserId="user-current"
        onClose={vi.fn()}
        onVote={vi.fn()}
      />,
    );

    // All vote buttons are disabled — clicks must not trigger any API calls.
    for (const btn of screen
      .getAllByRole("button")
      .filter((b) => b.textContent?.match(/votes/))) {
      fireEvent.click(btn);
    }

    expect(mockCreate).not.toHaveBeenCalled();
    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("does not call any API when currentUserId is null (logged-out viewer)", () => {
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId={null}
        onClose={vi.fn()}
        onVote={vi.fn()}
      />,
    );

    // Buttons are enabled visually (not disabled) but the handler guards against null userId.
    for (const btn of screen
      .getAllByRole("button")
      .filter((b) => b.textContent?.match(/votes/))) {
      fireEvent.click(btn);
    }

    expect(mockCreate).not.toHaveBeenCalled();
    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("clicking close poll calls records.update then onClose with the poll id", async () => {
    const onClose = vi.fn();
    mockUpdate.mockResolvedValueOnce({ ...poll, is_closed: true });

    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-owner" // owner sees the Close poll button
        onClose={onClose}
      />,
    );

    fireEvent.click(screen.getByText("Close poll"));

    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(onClose).toHaveBeenCalledWith("poll-1");
    expect(mockUpdate).toHaveBeenCalledWith("polls", "poll-1", { is_closed: true });
    // Voting API must not have been triggered.
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("shows an error message when the vote API call fails", async () => {
    mockCreate.mockRejectedValueOnce(new Error("Network error"));

    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-current"
        onClose={vi.fn()}
        onVote={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /TypeScript/ }));

    expect(await screen.findByText("Network error")).toBeInTheDocument();
  });
});
