import {
  test,
  expect,
  bootstrapMockedAdminApp,
  mockUsageMeteringApis,
} from "./fixtures";

test.describe("Usage Metering (Browser Mocked)", () => {
  test("navigates to usage page and keeps metric/period filters synchronized", async ({ page }) => {
    await bootstrapMockedAdminApp(page);
    const usageState = await mockUsageMeteringApis(page);

    await page.goto("/admin/");

    await page.locator("aside").getByRole("button", { name: /^Usage$/i }).click();
    await expect(page.getByRole("heading", { name: /Usage Metering/i })).toBeVisible();

    await expect(page.getByRole("cell", { name: "Tenant One" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Usage Trend" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Usage Breakdown" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Tenant Limits" })).toBeVisible();

    await page.getByRole("combobox", { name: "Metric" }).selectOption("storage_bytes");
    await expect(page.getByRole("combobox", { name: "Granularity" })).toHaveValue("day");
    await expect(page.getByRole("combobox", { name: "Breakdown" })).toHaveValue("tenant");

    await expect.poll(() => usageState.trendCalls.at(-1)?.metric ?? "").toBe("storage_bytes");
    await expect.poll(() => usageState.trendCalls.at(-1)?.granularity ?? "").toBe("day");
    await expect.poll(() => usageState.breakdownCalls.at(-1)?.metric ?? "").toBe("storage_bytes");
    await expect.poll(() => usageState.breakdownCalls.at(-1)?.group_by ?? "").toBe("tenant");

    await page.getByRole("combobox", { name: "Period" }).selectOption("week");
    await expect.poll(() => usageState.listCalls.at(-1)?.period ?? "").toBe("week");
    await expect.poll(() => usageState.trendCalls.at(-1)?.period ?? "").toBe("week");
    await expect.poll(() => usageState.breakdownCalls.at(-1)?.period ?? "").toBe("week");
    await expect.poll(() => usageState.limitsCalls.at(-1)?.query.period ?? "").toBe("week");

    await expect.poll(() => usageState.limitsCalls.at(-1)?.tenantId ?? "").toBe("tenant-1");

    await page.getByRole("cell", { name: "Tenant Two" }).click();
    await expect.poll(() => usageState.limitsCalls.at(-1)?.tenantId ?? "").toBe("tenant-2");
    await expect.poll(() => usageState.limitsCalls.at(-1)?.query.period ?? "").toBe("week");
  });
});
