import { useEffect, useRef } from "react";
import { ayb } from "../lib/ayb";
import type { RealtimeEvent } from "@allyourbase/js";

export type { RealtimeEvent };

/**
 * Subscribe to realtime changes on the given tables.
 * Calls `callback` for every create/update/delete event.
 *
 * The callback is captured via a ref so that callers can use an inline function
 * or a useCallback with dependencies without triggering re-subscription. The
 * subscription is only re-created when `tables` changes.
 */
export function useRealtime(
  tables: string[],
  callback: (event: RealtimeEvent) => void,
) {
  // Always hold the latest callback in a ref so the subscription closure never
  // goes stale, even if the caller passes a new function identity each render.
  const callbackRef = useRef(callback);
  callbackRef.current = callback;

  useEffect(() => {
    if (tables.length === 0) return;
    const unsub = ayb.realtime.subscribe(
      tables,
      (event) => callbackRef.current(event),
    );
    return unsub;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tables.join(",")]);
}
