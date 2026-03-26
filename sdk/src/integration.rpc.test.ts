import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import {
  AUTH_TEST_PASSWORD,
  INTEGRATION_RUN_ID,
  SCHEMA_CACHE_INTERVAL_MS,
  SCHEMA_CACHE_TIMEOUT_MS,
  adminSql,
  cleanupTrackedAuthUsers,
  createTestClient,
  expectAYBError,
  makeUniqueAuthEmail,
  primeIntegrationSuite,
  sqlStringLiteral,
  toCount,
  trackAuthUser,
  waitForCondition,
} from "./integration-helpers";

describe("SDK RPC integration suite", () => {
  const addFn = `sdk_add_${INTEGRATION_RUN_ID}`;
  const greetFn = `sdk_greet_${INTEGRATION_RUN_ID}`;
  const noopFn = `sdk_noop_${INTEGRATION_RUN_ID}`;
  const seriesFn = `sdk_series_${INTEGRATION_RUN_ID}`;
  const divZeroFn = `sdk_div_zero_${INTEGRATION_RUN_ID}`;
  const noSuchFn = `sdk_no_such_fn_${INTEGRATION_RUN_ID}`;
  let client: AYBClient;

  beforeAll(async () => {
    await primeIntegrationSuite();

    // Register an authenticated user (register auto-sets tokens)
    client = createTestClient();
    const email = makeUniqueAuthEmail("rpc");
    const registered = await client.auth.register(email, AUTH_TEST_PASSWORD);
    trackAuthUser(registered.user.id);

    // Create temporary Postgres functions for testing
    await adminSql(
      `CREATE FUNCTION ${addFn}(a int, b int) RETURNS int LANGUAGE SQL AS $$ SELECT a + b $$`,
    );
    await adminSql(
      `CREATE FUNCTION ${greetFn}(name text) RETURNS text LANGUAGE SQL AS $$ SELECT 'Hello, ' || name $$`,
    );
    await adminSql(
      `CREATE FUNCTION ${noopFn}() RETURNS void LANGUAGE SQL AS $$ SELECT $$`,
    );
    await adminSql(
      `CREATE FUNCTION ${seriesFn}(start_n int, end_n int) RETURNS TABLE(n int) LANGUAGE SQL AS $$ SELECT generate_series(start_n, end_n) AS n $$`,
    );
    await adminSql(
      `CREATE FUNCTION ${divZeroFn}(denominator int) RETURNS int LANGUAGE SQL AS $$ SELECT 1 / denominator $$`,
    );

    // Wait for schema cache to pick up the new functions
    await waitForCondition({
      description: `RPC schema cache for ${addFn}, ${seriesFn}, and ${divZeroFn}`,
      timeoutMs: SCHEMA_CACHE_TIMEOUT_MS,
      intervalMs: SCHEMA_CACHE_INTERVAL_MS,
      check: async () => {
        try {
          await client.rpc(addFn, { a: 0, b: 0 });
          await client.rpc<Array<{ n: number }>>(seriesFn, {
            start_n: 1,
            end_n: 1,
          });
          await client.rpc<number>(divZeroFn, { denominator: 1 });
          return true;
        } catch (error) {
          if (error instanceof AYBError) {
            // Only retry the specific expected messages — not arbitrary 404/503
            const isNotYetVisible =
              error.status === 404 &&
              (error.message === `function not found: ${addFn}` ||
                error.message === `function not found: ${seriesFn}` ||
                error.message === `function not found: ${divZeroFn}`);
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
      // Drop all test functions
      await adminSql(`DROP FUNCTION IF EXISTS ${addFn}(int, int)`);
      await adminSql(`DROP FUNCTION IF EXISTS ${greetFn}(text)`);
      await adminSql(`DROP FUNCTION IF EXISTS ${noopFn}()`);
      await adminSql(`DROP FUNCTION IF EXISTS ${seriesFn}(int, int)`);
      await adminSql(`DROP FUNCTION IF EXISTS ${divZeroFn}(int)`);

      // Verify cleanup — none of the test functions remain in pg_proc
      const remaining = await adminSql(
        `SELECT COUNT(*) AS count FROM pg_proc WHERE proname IN (${sqlStringLiteral(addFn)}, ${sqlStringLiteral(greetFn)}, ${sqlStringLiteral(noopFn)}, ${sqlStringLiteral(seriesFn)}, ${sqlStringLiteral(divZeroFn)})`,
      );
      expect(toCount(remaining.rows[0]?.[0])).toBe(0);
    } catch (error) {
      teardownError = error;
    }

    try {
      await cleanupTrackedAuthUsers();
    } catch (error) {
      if (teardownError) {
        throw new AggregateError(
          [teardownError, error],
          "RPC integration teardown failed",
        );
      }
      throw error;
    }

    if (teardownError) {
      throw teardownError;
    }
  }, 35_000);

  it("returns scalar integer from add function", async () => {
    const result = await client.rpc<number>(addFn, { a: 3, b: 7 });
    expect(result).toBe(10);
  });

  it("returns scalar integer with negative numbers", async () => {
    const result = await client.rpc<number>(addFn, { a: -5, b: 3 });
    expect(result).toBe(-2);
  });

  it("returns text from greet function", async () => {
    const result = await client.rpc<string>(greetFn, { name: "world" });
    expect(result).toBe("Hello, world");
  });

  it("returns undefined for void function", async () => {
    const result = await client.rpc(noopFn);
    expect(result).toBeUndefined();
  });

  it("returns ordered rows from set-returning function", async () => {
    const result = await client.rpc<Array<{ n: number }>>(seriesFn, {
      start_n: 2,
      end_n: 4,
    });
    expect(result).toEqual([{ n: 2 }, { n: 3 }, { n: 4 }]);
  });

  it("returns empty array from set-returning function", async () => {
    const result = await client.rpc<Array<{ n: number }>>(seriesFn, {
      start_n: 4,
      end_n: 2,
    });
    expect(result).toEqual([]);
  });

  it("throws 400 AYBError for wrong argument type", async () => {
    await expectAYBError(
      () => client.rpc(addFn, { a: "not-an-integer", b: 1 }),
      400,
      "invalid integer value \u2014 expected a whole number, e.g. 42",
    );
  });

  it("throws 500 AYBError for division by zero runtime error", async () => {
    await expectAYBError(
      () => client.rpc(divZeroFn, { denominator: 0 }),
      500,
      "internal error",
    );
  });

  it("throws 404 AYBError for nonexistent function", async () => {
    await expectAYBError(
      () => client.rpc(noSuchFn),
      404,
      `function not found: ${noSuchFn}`,
    );
  });
});
