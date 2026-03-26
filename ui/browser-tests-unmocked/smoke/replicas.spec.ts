import {
  test,
  expect,
  probeEndpoint,
  seedReplica,
  cleanupReplicaByName,
  fetchReplicaStatuses,
  waitForDashboard,
} from "../fixtures";

function resolveReplicaSeedTarget():
  | { host: string; port: number; database: string; sslMode: string }
  | null {
  const replicaURL = process.env.AYB_DATABASE_REPLICA_URLS
    ?.split(",")
    .map((value) => value.trim())
    .find((value) => value.length > 0);
  if (!replicaURL) {
    return null;
  }

  try {
    const parsed = new URL(replicaURL);
    const database = parsed.pathname.replace(/^\/+/, "");
    return {
      host: parsed.hostname,
      port: parsed.port ? Number(parsed.port) : 5432,
      database: database || "postgres",
      sslMode: parsed.searchParams.get("sslmode") || "disable",
    };
  } catch {
    return null;
  }
}

/**
 * SMOKE TEST: Replicas
 *
 * Critical Path: Seed a replica → Navigate to Replicas → Verify the seeded
 * replica renders in the table body with URL and state. Skip when the
 * environment does not expose a real standby that can pass add-replica checks.
 */

test.describe("Smoke: Replicas", () => {
  const seededReplicaNames: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (seededReplicaNames.length > 0) {
      const name = seededReplicaNames.pop();
      if (!name) continue;
      await cleanupReplicaByName(request, adminToken, name).catch(() => {});
    }
  });

  test("seeded replica renders in the replicas table", async ({ page, request, adminToken }) => {
    const status = await probeEndpoint(request, adminToken, "/api/admin/replicas");
    test.skip(
      status === 501 || status === 404,
      `Replicas service not configured (status ${status})`,
    );

    const seedTarget = resolveReplicaSeedTarget();
    test.skip(
      !seedTarget,
      "Replicas smoke requires AYB_DATABASE_REPLICA_URLS to point at a reachable standby",
    );

    const runId = Date.now();
    const replicaName = `smoke-replica-${runId}`;
    const baseline = await fetchReplicaStatuses(request, adminToken);
    try {
      await seedReplica(request, adminToken, {
        name: replicaName,
        host: seedTarget.host,
        database: seedTarget.database,
        port: seedTarget.port,
        ssl_mode: seedTarget.sslMode,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      test.skip(
        /(status 503|replica lifecycle not available|dial connectivity pool|connectivity check failed|target is not a replica|target is not a standby replica)/i.test(message),
        `Replica seeding unavailable in this environment: ${message}`,
      );
      throw error;
    }
    seededReplicaNames.push(replicaName);
    const updated = await fetchReplicaStatuses(request, adminToken);
    expect(updated.replicas).toHaveLength(baseline.replicas.length + 1);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /Replicas/i }).click();
    await expect(page.getByRole("heading", { name: /Replicas/i })).toBeVisible({ timeout: 15_000 });

    // Verify table column headers
    await expect(page.getByRole("columnheader", { name: /URL/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /State/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Lag/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /Connections/i })).toBeVisible();

    // Use the seeded replica name to identify the exact row when multiple replicas share a host.
    const replicaRow = page
      .locator("tr")
      .filter({
        has: page.getByRole("button", { name: `Promote ${replicaName}` }),
      })
      .first();
    await expect(replicaRow).toBeVisible({ timeout: 5000 });
    await expect(replicaRow).toContainText(seedTarget.host);
    await expect(replicaRow).toContainText("healthy");
  });
});
