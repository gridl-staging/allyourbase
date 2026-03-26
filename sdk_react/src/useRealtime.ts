/**
 * @module React hook for subscribing to real-time database updates on specified tables. Automatically manages subscription lifecycle and cleanup.
 */
import { useEffect, useMemo, useRef } from "react";
import { useAYBClient } from "./provider";

/**
 * Subscribes to real-time database updates on the specified tables. Automatically manages the subscription lifecycle, with the callback invoked for each incoming event and cleanup performed on unmount or when table subscriptions change.
 * @param tables - Table names to subscribe to
 * @param callback - Invoked for each real-time event on the subscribed tables
 */
export function useRealtime(
  tables: string[],
  callback: (event: Record<string, unknown>) => void,
): void {
  const client = useAYBClient();
  const callbackRef = useRef(callback);
  callbackRef.current = callback;

  const tablesKey = useMemo(() => tables.join(","), [tables]);

  useEffect(() => {
    const unsubscribe = client.realtime.subscribe(tables, (event) => {
      callbackRef.current(event);
    });

    return () => {
      unsubscribe();
    };
  }, [client, tables, tablesKey]);
}
