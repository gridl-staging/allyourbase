import { describe, it, expect } from "vitest";
import type { Vote, PollOption } from "../src/types";

/** Count votes per option_id, same logic used in PollCard. */
function countVotes(votes: Vote[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const v of votes) {
    counts.set(v.option_id, (counts.get(v.option_id) ?? 0) + 1);
  }
  return counts;
}

/** Percentage with zero-division safety. */
function pct(count: number, total: number): number {
  return total > 0 ? Math.round((count / total) * 100) : 0;
}

/** Sort options by position. */
function sortOptions(options: PollOption[]): PollOption[] {
  return [...options].sort((a, b) => a.position - b.position);
}

function vote(optionId: string, userId: string): Vote {
  return {
    id: crypto.randomUUID(),
    poll_id: "poll-1",
    option_id: optionId,
    user_id: userId,
    created_at: new Date().toISOString(),
  };
}

describe("countVotes", () => {
  it("returns empty map for no votes", () => {
    const counts = countVotes([]);
    expect(counts.size).toBe(0);
  });

  it("counts single vote per option", () => {
    const votes = [vote("opt-a", "user-1"), vote("opt-b", "user-2")];
    const counts = countVotes(votes);
    expect(counts.get("opt-a")).toBe(1);
    expect(counts.get("opt-b")).toBe(1);
  });

  it("counts multiple votes for same option", () => {
    const votes = [
      vote("opt-a", "user-1"),
      vote("opt-a", "user-2"),
      vote("opt-a", "user-3"),
      vote("opt-b", "user-4"),
    ];
    const counts = countVotes(votes);
    expect(counts.get("opt-a")).toBe(3);
    expect(counts.get("opt-b")).toBe(1);
  });

  it("handles option with zero votes", () => {
    const votes = [vote("opt-a", "user-1")];
    const counts = countVotes(votes);
    expect(counts.get("opt-a")).toBe(1);
    expect(counts.get("opt-b")).toBeUndefined();
  });
});

describe("pct", () => {
  it("returns 0 when total is 0", () => {
    expect(pct(0, 0)).toBe(0);
  });

  it("returns 100 when count equals total", () => {
    expect(pct(5, 5)).toBe(100);
  });

  it("returns 50 for half", () => {
    expect(pct(1, 2)).toBe(50);
  });

  it("rounds to nearest integer", () => {
    expect(pct(1, 3)).toBe(33);
    expect(pct(2, 3)).toBe(67);
  });

  it("handles single vote of many (small percentage)", () => {
    expect(pct(1, 100)).toBe(1);
    expect(pct(1, 1000)).toBe(0); // rounds to 0
  });

  it("three-way split rounds without exceeding 100", () => {
    // 1/3 + 1/3 + 1/3 = 33 + 33 + 33 = 99 (not 100, but that's expected with rounding)
    const parts = [pct(1, 3), pct(1, 3), pct(1, 3)];
    expect(parts).toEqual([33, 33, 33]);
  });

  it("all votes on one option yields 100%", () => {
    expect(pct(10, 10)).toBe(100);
    expect(pct(1, 1)).toBe(100);
  });
});

describe("sortOptions", () => {
  it("sorts by position ascending", () => {
    const opts: PollOption[] = [
      { id: "c", poll_id: "p", label: "Third", position: 2 },
      { id: "a", poll_id: "p", label: "First", position: 0 },
      { id: "b", poll_id: "p", label: "Second", position: 1 },
    ];
    const sorted = sortOptions(opts);
    expect(sorted.map((o) => o.label)).toEqual(["First", "Second", "Third"]);
  });

  it("does not mutate original array", () => {
    const opts: PollOption[] = [
      { id: "b", poll_id: "p", label: "B", position: 1 },
      { id: "a", poll_id: "p", label: "A", position: 0 },
    ];
    const sorted = sortOptions(opts);
    expect(sorted[0].id).toBe("a");
    expect(opts[0].id).toBe("b"); // original unchanged
  });
});
