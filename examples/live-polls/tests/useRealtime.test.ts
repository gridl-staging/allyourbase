import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useRealtime } from "../src/hooks/useRealtime";
import type { RealtimeEvent } from "../src/hooks/useRealtime";

vi.mock("../src/lib/ayb", () => ({
  ayb: {
    realtime: {
      subscribe: vi.fn(() => vi.fn()),
    },
  },
}));

// Import *after* vi.mock so we get the mocked version.
const { ayb } = await import("../src/lib/ayb");
const mockSubscribe = ayb.realtime.subscribe as ReturnType<typeof vi.fn>;

describe("useRealtime", () => {
  beforeEach(() => {
    mockSubscribe.mockReset();
    mockSubscribe.mockReturnValue(vi.fn());
  });

  it("does not subscribe when tables is empty", () => {
    renderHook(() => useRealtime([], vi.fn()));
    expect(mockSubscribe).not.toHaveBeenCalled();
  });

  it("subscribes with the correct tables", () => {
    renderHook(() => useRealtime(["polls", "votes"], vi.fn()));
    expect(mockSubscribe).toHaveBeenCalledOnce();
    expect(mockSubscribe).toHaveBeenCalledWith(
      ["polls", "votes"],
      expect.any(Function),
    );
  });

  it("calls unsubscribe on unmount", () => {
    const unsub = vi.fn();
    mockSubscribe.mockReturnValue(unsub);
    const { unmount } = renderHook(() => useRealtime(["polls"], vi.fn()));
    expect(unsub).not.toHaveBeenCalled();
    unmount();
    expect(unsub).toHaveBeenCalledOnce();
  });

  it("re-subscribes when tables change (unsubs old first)", () => {
    const unsub1 = vi.fn();
    const unsub2 = vi.fn();
    mockSubscribe.mockReturnValueOnce(unsub1).mockReturnValueOnce(unsub2);

    const { rerender } = renderHook(
      ({ tables }) => useRealtime(tables, vi.fn()),
      { initialProps: { tables: ["polls"] } },
    );
    expect(mockSubscribe).toHaveBeenCalledTimes(1);

    rerender({ tables: ["polls", "votes"] });
    expect(unsub1).toHaveBeenCalledOnce();
    expect(mockSubscribe).toHaveBeenCalledTimes(2);
    expect(mockSubscribe).toHaveBeenLastCalledWith(
      ["polls", "votes"],
      expect.any(Function),
    );
  });

  it("does NOT re-subscribe when callback identity changes but tables stay the same", () => {
    type Props = { cb: (event: RealtimeEvent) => void };
    const { rerender } = renderHook(
      ({ cb }: Props) => useRealtime(["polls"], cb),
      { initialProps: { cb: vi.fn() as (event: RealtimeEvent) => void } },
    );
    expect(mockSubscribe).toHaveBeenCalledTimes(1);

    rerender({ cb: vi.fn() as (event: RealtimeEvent) => void }); // new function reference, same tables
    expect(mockSubscribe).toHaveBeenCalledTimes(1); // no new subscription
  });

  it("always dispatches to the latest callback (stale-closure guard)", () => {
    type Props = { cb: (event: RealtimeEvent) => void };
    let capturedHandler: ((event: RealtimeEvent) => void) | undefined;
    mockSubscribe.mockImplementation((_tables, handler) => {
      capturedHandler = handler;
      return vi.fn();
    });

    const cb1 = vi.fn() as (event: RealtimeEvent) => void;
    const cb2 = vi.fn() as (event: RealtimeEvent) => void;

    const { rerender } = renderHook(
      ({ cb }: Props) => useRealtime(["polls"], cb),
      { initialProps: { cb: cb1 } },
    );

    // Update the callback without changing tables — no re-subscription.
    rerender({ cb: cb2 });

    const event: RealtimeEvent = { action: "create", table: "polls", record: {} };
    capturedHandler!(event);

    expect(cb2).toHaveBeenCalledWith(event);
    expect(cb1).not.toHaveBeenCalled();
  });

  it("creates a subscription when transitioning from empty to populated tables (auth flow)", () => {
    const unsub = vi.fn();
    mockSubscribe.mockReturnValue(unsub);

    const { rerender } = renderHook(
      ({ tables }) => useRealtime(tables, vi.fn()),
      { initialProps: { tables: [] as string[] } },
    );
    expect(mockSubscribe).not.toHaveBeenCalled();

    rerender({ tables: ["polls", "poll_options", "votes"] });
    expect(mockSubscribe).toHaveBeenCalledOnce();
    expect(mockSubscribe).toHaveBeenCalledWith(
      ["polls", "poll_options", "votes"],
      expect.any(Function),
    );
  });
});
