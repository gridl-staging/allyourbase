import { EventSource as NodeEventSource } from "eventsource";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import type { RealtimeEvent } from "./types";
import {
  AUTH_TEST_PASSWORD,
  INTEGRATION_RUN_ID,
  SCHEMA_CACHE_INTERVAL_MS,
  SCHEMA_CACHE_TIMEOUT_MS,
  adminSql,
  cleanupTrackedAuthUsers,
  createTestClient,
  dropTableAndAssertRemoved,
  expectRealtimeEventShape,
  makeUniqueAuthEmail,
  primeIntegrationSuite,
  sqlStringLiteral,
  sleep,
  toCount,
  trackAuthUser,
  waitForCollectionSchemaCache,
  waitForCondition,
} from "./integration-helpers";

(globalThis as typeof globalThis & { EventSource?: typeof NodeEventSource }).EventSource ??=
  NodeEventSource as unknown as typeof globalThis.EventSource;

type RPCNotifyInsertedRow = {
  id: number;
  title: string;
  created_at: string;
};

describe("SDK RPC notify integration suite", () => {
  const tableName = `sdk_rpc_notify_${INTEGRATION_RUN_ID}`;
  const functionName = `sdk_rpc_notify_insert_${INTEGRATION_RUN_ID}`;
  const rpcNotifyOptions = {
    notify: { table: tableName, action: "create" as const },
  };
  let client: AYBClient;

  async function openSubscription(): Promise<{
    receivedEvents: RealtimeEvent[];
    unsubscribe: () => void;
  }> {
    const receivedEvents: RealtimeEvent[] = [];
    const unsubscribe = client.realtime.subscribe([tableName], (event) => {
      receivedEvents.push(event);
    });
    // Keep the same SSE warm-up pattern used in integration.realtime.test.ts.
    await sleep(500);
    return { receivedEvents, unsubscribe };
  }

  async function waitForRealtimeAction(
    receivedEvents: RealtimeEvent[],
    action: RealtimeEvent["action"],
  ): Promise<void> {
    await waitForCondition({
      description: `${action} realtime event for ${tableName}`,
      timeoutMs: 10_000,
      intervalMs: 200,
      check: async () =>
        receivedEvents.some(
          (event) => event.action === action && event.table === tableName,
        ),
    });
  }

  function insertViaRPC(title: string): Promise<RPCNotifyInsertedRow> {
    return client.rpc<RPCNotifyInsertedRow>(
      functionName,
      { p_title: title },
      rpcNotifyOptions,
    );
  }

  beforeAll(async () => {
    await primeIntegrationSuite();

    client = createTestClient();
    const email = makeUniqueAuthEmail("rpc-notify");
    const registered = await client.auth.register(email, AUTH_TEST_PASSWORD);
    trackAuthUser(registered.user.id);

    await adminSql(
      `CREATE TABLE ${tableName} (
        id serial PRIMARY KEY,
        title text NOT NULL,
        created_at timestamptz NOT NULL DEFAULT now()
      )`,
    );
    await adminSql(`ALTER TABLE ${tableName} ENABLE ROW LEVEL SECURITY`);
    await adminSql(
      `CREATE POLICY sdk_rpc_notify_all ON ${tableName} FOR ALL USING (true) WITH CHECK (true)`,
    );

    await adminSql(
      `CREATE FUNCTION ${functionName}(p_title text, OUT id integer, OUT title text, OUT created_at timestamptz)
       LANGUAGE plpgsql
       AS $$
       BEGIN
         INSERT INTO ${tableName}(title)
         VALUES (p_title)
         RETURNING ${tableName}.id, ${tableName}.title, ${tableName}.created_at
         INTO id, title, created_at;
       END;
       $$`,
    );

    await waitForCollectionSchemaCache(client, tableName, "rpc notify");

    await waitForCondition({
      description: `RPC schema cache for ${functionName}`,
      timeoutMs: SCHEMA_CACHE_TIMEOUT_MS,
      intervalMs: SCHEMA_CACHE_INTERVAL_MS,
      check: async () => {
        try {
          await client.rpc<RPCNotifyInsertedRow>(functionName, {
            p_title: "schema-probe",
          });
          return true;
        } catch (error) {
          if (error instanceof AYBError) {
            const isNotYetVisible =
              error.status === 404 &&
              error.message === `function not found: ${functionName}`;
            const isCacheLoading =
              error.status === 503 &&
              error.message.includes("schema cache not ready");
            if (isNotYetVisible || isCacheLoading) {
              return false;
            }
          }
          throw error;
        }
      },
    });
  }, 60_000);

  afterAll(async () => {
    let teardownError: unknown;

    try {
      await adminSql(`DROP FUNCTION IF EXISTS ${functionName}(text)`);
      const remainingFunctions = await adminSql(
        `SELECT COUNT(*) AS count FROM pg_proc WHERE proname = ${sqlStringLiteral(functionName)}`,
      );
      expect(toCount(remainingFunctions.rows[0]?.[0])).toBe(0);

      await dropTableAndAssertRemoved(tableName);
    } catch (error) {
      teardownError = error;
    }

    try {
      await cleanupTrackedAuthUsers();
    } catch (error) {
      if (teardownError) {
        throw new AggregateError(
          [teardownError, error],
          "RPC notify integration teardown failed",
        );
      }
      throw error;
    }

    if (teardownError) {
      throw teardownError;
    }
  }, 35_000);

  it("rpc notify create returns inserted row and emits matching realtime event", async () => {
    const { receivedEvents, unsubscribe } = await openSubscription();

    const rpcResult = await insertViaRPC("rpc notify");

    await waitForRealtimeAction(receivedEvents, "create");

    const matchingCreateEvents = receivedEvents.filter(
      (event) => event.action === "create" && event.table === tableName,
    );

    expect(matchingCreateEvents).toHaveLength(1);
    expect(rpcResult.title).toBe("rpc notify");
    expect(matchingCreateEvents[0]?.record).toEqual(rpcResult);
    expectRealtimeEventShape(receivedEvents);

    const eventCountAtUnsubscribe = receivedEvents.length;
    unsubscribe();

    await insertViaRPC("after-unsubscribe");

    const postUnsubscribeWindowStartedAt = Date.now();
    await waitForCondition({
      description: "no additional rpc notify events after unsubscribe",
      timeoutMs: 1_500,
      intervalMs: 150,
      check: async () => {
        expect(receivedEvents).toHaveLength(eventCountAtUnsubscribe);
        return Date.now() - postUnsubscribeWindowStartedAt >= 1_000;
      },
    });
  });
});
