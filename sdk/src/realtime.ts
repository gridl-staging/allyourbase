/**
 * @module Provides a RealtimeClient class for subscribing to server-sent realtime event updates from specified database tables.
 */
import type { RealtimeEvent } from "./types";
import { normalizeRealtimeEvent } from "./helpers";

interface RealtimeClientRuntime {
  token: string | null;
  getBaseURL(): string;
}

/**
 * Client for subscribing to server-sent realtime events. Connects to the /api/realtime endpoint using EventSource and normalizes incoming event data.
 */
export class RealtimeClient {
  constructor(private client: RealtimeClientRuntime) {}

  /**
   * Subscribe to realtime events for the given tables.
   * Returns an unsubscribe function.
   */
  /**
   * Subscribe to realtime events for the given tables. Establishes an EventSource connection that invokes the callback for each normalized event received.
   * @param tables - table names to subscribe to
   * @param callback - invoked when realtime events are received
   * @returns function to close the event stream and stop receiving events
   */
  subscribe(
    tables: string[],
    callback: (event: RealtimeEvent) => void,
  ): () => void {
    const params = new URLSearchParams({ tables: tables.join(",") });
    if (this.client.token) {
      params.set("token", this.client.token);
    }
    const url = `${this.client.getBaseURL()}/api/realtime?${params}`;
    const es = new EventSource(url);

    es.onmessage = (e) => {
      try {
        const event = normalizeRealtimeEvent(JSON.parse(e.data) as RealtimeEvent);
        callback(event);
      } catch {
        // Ignore parse errors for heartbeat/ping messages.
      }
    };

    return () => es.close();
  }
}
