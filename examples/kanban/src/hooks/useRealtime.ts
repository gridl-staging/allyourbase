import { useEffect } from "react";
import type { RealtimeEvent } from "@allyourbase/js";
import { ayb } from "../lib/ayb";

export function useRealtime(
  tables: string[],
  callback: (event: RealtimeEvent) => void,
) {
  useEffect(() => {
    if (tables.length === 0) return;
    const unsub = ayb.realtime.subscribe(tables, callback);
    return unsub;
  }, [tables.join(",")]); // eslint-disable-line react-hooks/exhaustive-deps
}
