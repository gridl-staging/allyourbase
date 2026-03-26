import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EdgeFunctionTriggers } from "../EdgeFunctionTriggers";
import {
  listDBTriggers,
  createDBTrigger,
  deleteDBTrigger,
  enableDBTrigger,
  disableDBTrigger,
  listCronTriggers,
  createCronTrigger,
  deleteCronTrigger,
  enableCronTrigger,
  disableCronTrigger,
  manualRunCronTrigger,
  listStorageTriggers,
  createStorageTrigger,
  deleteStorageTrigger,
  enableStorageTrigger,
  disableStorageTrigger,
} from "../../api";
import type {
  DBTriggerResponse,
  CronTriggerResponse,
  StorageTriggerResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listDBTriggers: vi.fn(),
  createDBTrigger: vi.fn(),
  deleteDBTrigger: vi.fn(),
  enableDBTrigger: vi.fn(),
  disableDBTrigger: vi.fn(),
  listCronTriggers: vi.fn(),
  createCronTrigger: vi.fn(),
  deleteCronTrigger: vi.fn(),
  enableCronTrigger: vi.fn(),
  disableCronTrigger: vi.fn(),
  manualRunCronTrigger: vi.fn(),
  listStorageTriggers: vi.fn(),
  createStorageTrigger: vi.fn(),
  deleteStorageTrigger: vi.fn(),
  enableStorageTrigger: vi.fn(),
  disableStorageTrigger: vi.fn(),
}));

const mockListDBTriggers = vi.mocked(listDBTriggers);
const mockCreateDBTrigger = vi.mocked(createDBTrigger);
const mockDeleteDBTrigger = vi.mocked(deleteDBTrigger);
const mockEnableDBTrigger = vi.mocked(enableDBTrigger);
const mockDisableDBTrigger = vi.mocked(disableDBTrigger);
const mockListCronTriggers = vi.mocked(listCronTriggers);
const mockCreateCronTrigger = vi.mocked(createCronTrigger);
const mockDeleteCronTrigger = vi.mocked(deleteCronTrigger);
const mockEnableCronTrigger = vi.mocked(enableCronTrigger);
const mockDisableCronTrigger = vi.mocked(disableCronTrigger);
const mockManualRunCronTrigger = vi.mocked(manualRunCronTrigger);
const mockListStorageTriggers = vi.mocked(listStorageTriggers);
const mockCreateStorageTrigger = vi.mocked(createStorageTrigger);
const mockDeleteStorageTrigger = vi.mocked(deleteStorageTrigger);
const mockEnableStorageTrigger = vi.mocked(enableStorageTrigger);
const mockDisableStorageTrigger = vi.mocked(disableStorageTrigger);

function makeDBTrigger(overrides: Partial<DBTriggerResponse> = {}): DBTriggerResponse {
  return {
    id: "dbt-001",
    functionId: "ef_1",
    tableName: "users",
    schema: "public",
    events: ["INSERT", "UPDATE"],
    filterColumns: [],
    enabled: true,
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
    ...overrides,
  };
}

function makeCronTrigger(overrides: Partial<CronTriggerResponse> = {}): CronTriggerResponse {
  return {
    id: "ct-001",
    functionId: "ef_1",
    scheduleId: "sched-001",
    cronExpr: "*/5 * * * *",
    timezone: "UTC",
    payload: {},
    enabled: true,
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
    ...overrides,
  };
}

function makeStorageTrigger(overrides: Partial<StorageTriggerResponse> = {}): StorageTriggerResponse {
  return {
    id: "st-001",
    functionId: "ef_1",
    bucket: "uploads",
    eventTypes: ["upload"],
    prefixFilter: "",
    suffixFilter: ".jpg",
    enabled: true,
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
    ...overrides,
  };
}

const FUNCTION_ID = "ef_1";
const addToast = vi.fn();

function renderTriggers() {
  return render(
    <EdgeFunctionTriggers functionId={FUNCTION_ID} addToast={addToast} />,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListDBTriggers.mockResolvedValue([]);
  mockListCronTriggers.mockResolvedValue([]);
  mockListStorageTriggers.mockResolvedValue([]);
});

// --- Tab navigation ---

describe("EdgeFunctionTriggers tabs", () => {
  it("renders DB, Cron, Storage tabs", async () => {
    renderTriggers();
    await waitFor(() => {
      expect(screen.getByTestId("trigger-tab-db")).toBeInTheDocument();
      expect(screen.getByTestId("trigger-tab-cron")).toBeInTheDocument();
      expect(screen.getByTestId("trigger-tab-storage")).toBeInTheDocument();
    });
  });

  it("defaults to DB tab", async () => {
    renderTriggers();
    await waitFor(() => {
      expect(screen.getByTestId("trigger-tab-db")).toHaveAttribute("data-active", "true");
    });
  });

  it("switches to Cron tab on click", async () => {
    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-tab-cron")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-tab-cron"));
    expect(screen.getByTestId("trigger-tab-cron")).toHaveAttribute("data-active", "true");
    expect(screen.getByTestId("trigger-tab-db")).toHaveAttribute("data-active", "false");
  });
});

// --- DB Triggers ---

describe("DB Triggers", () => {
  it("shows empty state when no DB triggers", async () => {
    mockListDBTriggers.mockResolvedValue([]);
    renderTriggers();
    await waitFor(() => {
      expect(screen.getByText("No database triggers configured.")).toBeInTheDocument();
    });
  });

  it("lists existing DB triggers with table name and events", async () => {
    mockListDBTriggers.mockResolvedValue([
      makeDBTrigger({ tableName: "orders", events: ["INSERT", "DELETE"] }),
    ]);
    renderTriggers();
    await waitFor(() => {
      expect(screen.getByText("orders")).toBeInTheDocument();
      expect(screen.getByText("INSERT, DELETE")).toBeInTheDocument();
    });
  });

  it("renders DB triggers when API payload uses snake_case keys", async () => {
    mockListDBTriggers.mockResolvedValue([
      {
        id: "dbt-snake",
        function_id: "ef_1",
        table_name: "orders",
        schema_name: "public",
        events: ["INSERT", "DELETE"],
        filter_columns: [],
        enabled: true,
        created_at: "2026-02-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      } as unknown as DBTriggerResponse,
    ]);
    renderTriggers();
    await waitFor(() => {
      expect(screen.getByText("orders")).toBeInTheDocument();
      expect(screen.getByText("INSERT, DELETE")).toBeInTheDocument();
    });
  });

  it("shows enabled status badge for DB trigger", async () => {
    mockListDBTriggers.mockResolvedValue([makeDBTrigger({ enabled: true })]);
    renderTriggers();
    await waitFor(() => {
      expect(screen.getByTestId("trigger-enabled-dbt-001")).toHaveTextContent("Enabled");
    });
  });

  it("creates a new DB trigger via form", async () => {
    const created = makeDBTrigger({ id: "dbt-new" });
    mockCreateDBTrigger.mockResolvedValue(created);
    mockListDBTriggers.mockResolvedValueOnce([]).mockResolvedValueOnce([created]);
    renderTriggers();
    const user = userEvent.setup();

    await waitFor(() => expect(screen.getByTestId("add-db-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-db-trigger-btn"));

    // Fill form
    await user.type(screen.getByTestId("db-trigger-table"), "users");
    await user.click(screen.getByTestId("db-event-INSERT"));
    await user.click(screen.getByTestId("db-event-UPDATE"));
    await user.click(screen.getByTestId("db-trigger-submit"));

    await waitFor(() => {
      expect(mockCreateDBTrigger).toHaveBeenCalledWith(FUNCTION_ID, {
        table_name: "users",
        schema: "public",
        events: ["INSERT", "UPDATE"],
        filter_columns: [],
      });
    });
  });

  it("renders DB trigger after create even when cron refresh hangs", async () => {
    const created = makeDBTrigger({ id: "dbt-hang", tableName: "users" });
    const neverCronRefresh = new Promise<CronTriggerResponse[]>(() => {});

    mockListDBTriggers.mockResolvedValueOnce([]).mockResolvedValueOnce([created]);
    mockListCronTriggers.mockResolvedValueOnce([]).mockReturnValue(neverCronRefresh);
    mockListStorageTriggers.mockResolvedValueOnce([]).mockResolvedValueOnce([]);
    mockCreateDBTrigger.mockResolvedValue(created);

    renderTriggers();
    const user = userEvent.setup();

    await waitFor(() => expect(screen.getByTestId("add-db-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-db-trigger-btn"));
    await user.type(screen.getByTestId("db-trigger-table"), "users");
    await user.click(screen.getByTestId("db-event-INSERT"));
    await user.click(screen.getByTestId("db-trigger-submit"));

    await waitFor(() => {
      expect(mockCreateDBTrigger).toHaveBeenCalledWith(FUNCTION_ID, {
        table_name: "users",
        schema: "public",
        events: ["INSERT"],
        filter_columns: [],
      });
    });

    await waitFor(() => {
      expect(screen.getByText("users")).toBeInTheDocument();
    });
  });

  it("shows one error toast when DB trigger creation fails", async () => {
    mockCreateDBTrigger.mockRejectedValue(new Error("create failed"));
    renderTriggers();
    const user = userEvent.setup();

    await waitFor(() => expect(screen.getByTestId("add-db-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-db-trigger-btn"));
    await user.type(screen.getByTestId("db-trigger-table"), "users");
    await user.click(screen.getByTestId("db-event-INSERT"));
    await user.click(screen.getByTestId("db-trigger-submit"));

    await waitFor(() => {
      expect(addToast).toHaveBeenCalledWith("error", "create failed");
    });
    expect(addToast).toHaveBeenCalledTimes(1);
  });

  it("keeps created DB trigger visible when DB refresh fails", async () => {
    const created = makeDBTrigger({ id: "dbt-created", tableName: "users" });
    mockCreateDBTrigger.mockResolvedValue(created);
    mockListDBTriggers.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error("db list failed"));
    mockListCronTriggers.mockResolvedValue([]);
    mockListStorageTriggers.mockResolvedValue([]);

    renderTriggers();
    const user = userEvent.setup();

    await waitFor(() => expect(screen.getByTestId("add-db-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-db-trigger-btn"));
    await user.type(screen.getByTestId("db-trigger-table"), "users");
    await user.click(screen.getByTestId("db-event-INSERT"));
    await user.click(screen.getByTestId("db-trigger-submit"));

    await waitFor(() => {
      expect(screen.getByText("users")).toBeInTheDocument();
    });
  });

  it("disables DB trigger when enabled", async () => {
    const trigger = makeDBTrigger({ enabled: true });
    mockListDBTriggers.mockResolvedValue([trigger]);
    const disabled = makeDBTrigger({ enabled: false });
    mockDisableDBTrigger.mockResolvedValue(disabled);

    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-toggle-dbt-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-toggle-dbt-001"));

    await waitFor(() => {
      expect(mockDisableDBTrigger).toHaveBeenCalledWith(FUNCTION_ID, "dbt-001");
    });
  });

  it("enables DB trigger when disabled", async () => {
    const trigger = makeDBTrigger({ enabled: false });
    mockListDBTriggers.mockResolvedValue([trigger]);
    const enabled = makeDBTrigger({ enabled: true });
    mockEnableDBTrigger.mockResolvedValue(enabled);

    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-toggle-dbt-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-toggle-dbt-001"));

    await waitFor(() => {
      expect(mockEnableDBTrigger).toHaveBeenCalledWith(FUNCTION_ID, "dbt-001");
    });
    expect(mockDisableDBTrigger).not.toHaveBeenCalled();
  });

  it("deletes a DB trigger", async () => {
    const trigger = makeDBTrigger();
    mockListDBTriggers.mockResolvedValue([trigger]);
    mockDeleteDBTrigger.mockResolvedValue(undefined);

    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-delete-dbt-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-delete-dbt-001"));

    // Confirm delete
    await waitFor(() => expect(screen.getByTestId("trigger-confirm-delete-dbt-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-confirm-delete-dbt-001"));

    await waitFor(() => {
      expect(mockDeleteDBTrigger).toHaveBeenCalledWith(FUNCTION_ID, "dbt-001");
    });
  });
});

// --- Cron Triggers ---

describe("Cron Triggers", () => {
  async function switchToCronTab() {
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-tab-cron")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-tab-cron"));
    return user;
  }

  it("shows empty state when no cron triggers", async () => {
    renderTriggers();
    await switchToCronTab();
    await waitFor(() => {
      expect(screen.getByText("No cron triggers configured.")).toBeInTheDocument();
    });
  });

  it("lists existing cron triggers with expression and timezone", async () => {
    mockListCronTriggers.mockResolvedValue([
      makeCronTrigger({ cronExpr: "0 */6 * * *", timezone: "America/New_York" }),
    ]);
    renderTriggers();
    await switchToCronTab();
    await waitFor(() => {
      expect(screen.getByText("0 */6 * * *")).toBeInTheDocument();
      expect(screen.getByText("America/New_York")).toBeInTheDocument();
    });
  });

  it("renders cron triggers when API payload uses snake_case keys", async () => {
    mockListCronTriggers.mockResolvedValue([
      {
        id: "ct-snake",
        function_id: "ef_1",
        schedule_id: "sched-1",
        cron_expr: "0 */6 * * *",
        timezone: "UTC",
        payload: {},
        enabled: true,
        created_at: "2026-02-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      } as unknown as CronTriggerResponse,
    ]);
    renderTriggers();
    await switchToCronTab();
    await waitFor(() => {
      expect(screen.getByText("0 */6 * * *")).toBeInTheDocument();
      expect(screen.getByText("UTC")).toBeInTheDocument();
    });
  });

  it("creates a new cron trigger via form", async () => {
    const created = makeCronTrigger({ id: "ct-new" });
    mockCreateCronTrigger.mockResolvedValue(created);
    mockListCronTriggers.mockResolvedValueOnce([]).mockResolvedValueOnce([created]);

    renderTriggers();
    const user = await switchToCronTab();

    await waitFor(() => expect(screen.getByTestId("add-cron-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-cron-trigger-btn"));

    await user.type(screen.getByTestId("cron-trigger-expr"), "*/10 * * * *");
    await user.click(screen.getByTestId("cron-trigger-submit"));

    await waitFor(() => {
      expect(mockCreateCronTrigger).toHaveBeenCalledWith(FUNCTION_ID, {
        cron_expr: "*/10 * * * *",
        timezone: "UTC",
        payload: undefined,
      });
    });
  });

  it("keeps created cron trigger visible when cron refresh fails", async () => {
    const created = makeCronTrigger({ id: "ct-created", cronExpr: "0 0 * * *" });
    mockCreateCronTrigger.mockResolvedValue(created);
    mockListCronTriggers.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error("cron list failed"));
    mockListDBTriggers.mockResolvedValue([]);
    mockListStorageTriggers.mockResolvedValue([]);

    renderTriggers();
    const user = await switchToCronTab();

    await waitFor(() => expect(screen.getByTestId("add-cron-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-cron-trigger-btn"));
    await user.type(screen.getByTestId("cron-trigger-expr"), "0 0 * * *");
    await user.click(screen.getByTestId("cron-trigger-submit"));

    await waitFor(() => {
      expect(screen.getByText("0 0 * * *")).toBeInTheDocument();
    });
  });

  it("runs a cron trigger manually", async () => {
    const trigger = makeCronTrigger();
    mockListCronTriggers.mockResolvedValue([trigger]);
    mockManualRunCronTrigger.mockResolvedValue({ statusCode: 200, body: "OK" });

    renderTriggers();
    const user = await switchToCronTab();

    await waitFor(() => expect(screen.getByTestId("trigger-run-ct-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-run-ct-001"));

    await waitFor(() => {
      expect(mockManualRunCronTrigger).toHaveBeenCalledWith(FUNCTION_ID, "ct-001");
    });
  });

  it("disables cron trigger when enabled", async () => {
    const trigger = makeCronTrigger({ enabled: true });
    mockListCronTriggers.mockResolvedValue([trigger]);
    mockDisableCronTrigger.mockResolvedValue(makeCronTrigger({ enabled: false }));

    renderTriggers();
    const user = await switchToCronTab();

    await waitFor(() => expect(screen.getByTestId("trigger-toggle-ct-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-toggle-ct-001"));

    await waitFor(() => {
      expect(mockDisableCronTrigger).toHaveBeenCalledWith(FUNCTION_ID, "ct-001");
    });
  });

  it("enables cron trigger when disabled", async () => {
    const trigger = makeCronTrigger({ enabled: false });
    mockListCronTriggers.mockResolvedValue([trigger]);
    mockEnableCronTrigger.mockResolvedValue(makeCronTrigger({ enabled: true }));

    renderTriggers();
    const user = await switchToCronTab();

    await waitFor(() => expect(screen.getByTestId("trigger-toggle-ct-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-toggle-ct-001"));

    await waitFor(() => {
      expect(mockEnableCronTrigger).toHaveBeenCalledWith(FUNCTION_ID, "ct-001");
    });
    expect(mockDisableCronTrigger).not.toHaveBeenCalled();
  });

  it("deletes a cron trigger", async () => {
    const trigger = makeCronTrigger();
    mockListCronTriggers.mockResolvedValue([trigger]);
    mockDeleteCronTrigger.mockResolvedValue(undefined);

    renderTriggers();
    const user = await switchToCronTab();

    await waitFor(() => expect(screen.getByTestId("trigger-delete-ct-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-delete-ct-001"));

    await waitFor(() => expect(screen.getByTestId("trigger-confirm-delete-ct-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-confirm-delete-ct-001"));

    await waitFor(() => {
      expect(mockDeleteCronTrigger).toHaveBeenCalledWith(FUNCTION_ID, "ct-001");
    });
  });
});

// --- Storage Triggers ---

describe("Storage Triggers", () => {
  async function switchToStorageTab() {
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-tab-storage")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-tab-storage"));
    return user;
  }

  it("shows empty state when no storage triggers", async () => {
    renderTriggers();
    await switchToStorageTab();
    await waitFor(() => {
      expect(screen.getByText("No storage triggers configured.")).toBeInTheDocument();
    });
  });

  it("lists existing storage triggers with bucket and event types", async () => {
    mockListStorageTriggers.mockResolvedValue([
      makeStorageTrigger({ bucket: "avatars", eventTypes: ["upload", "delete"] }),
    ]);
    renderTriggers();
    await switchToStorageTab();
    await waitFor(() => {
      expect(screen.getByText("avatars")).toBeInTheDocument();
      expect(screen.getByText("upload, delete")).toBeInTheDocument();
    });
  });

  it("renders storage triggers when API payload uses snake_case keys", async () => {
    mockListStorageTriggers.mockResolvedValue([
      {
        id: "st-snake",
        function_id: "ef_1",
        bucket: "avatars",
        event_types: ["upload", "delete"],
        prefix_filter: "images/",
        suffix_filter: ".png",
        enabled: true,
        created_at: "2026-02-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      } as unknown as StorageTriggerResponse,
    ]);
    renderTriggers();
    await switchToStorageTab();
    await waitFor(() => {
      expect(screen.getByText("avatars")).toBeInTheDocument();
      expect(screen.getByText("upload, delete")).toBeInTheDocument();
    });
  });

  it("creates a new storage trigger via form", async () => {
    const created = makeStorageTrigger({ id: "st-new" });
    mockCreateStorageTrigger.mockResolvedValue(created);
    mockListStorageTriggers.mockResolvedValueOnce([]).mockResolvedValueOnce([created]);

    renderTriggers();
    const user = await switchToStorageTab();

    await waitFor(() => expect(screen.getByTestId("add-storage-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-storage-trigger-btn"));

    await user.type(screen.getByTestId("storage-trigger-bucket"), "uploads");
    await user.click(screen.getByTestId("storage-event-upload"));
    await user.click(screen.getByTestId("storage-trigger-submit"));

    await waitFor(() => {
      expect(mockCreateStorageTrigger).toHaveBeenCalledWith(FUNCTION_ID, {
        bucket: "uploads",
        event_types: ["upload"],
        prefix_filter: "",
        suffix_filter: "",
      });
    });
  });

  it("keeps created storage trigger visible when storage refresh fails", async () => {
    const created = makeStorageTrigger({ id: "st-created", bucket: "default", eventTypes: ["upload"] });
    mockCreateStorageTrigger.mockResolvedValue(created);
    mockListStorageTriggers.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error("storage list failed"));
    mockListDBTriggers.mockResolvedValue([]);
    mockListCronTriggers.mockResolvedValue([]);

    renderTriggers();
    const user = await switchToStorageTab();

    await waitFor(() => expect(screen.getByTestId("add-storage-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-storage-trigger-btn"));
    await user.type(screen.getByTestId("storage-trigger-bucket"), "default");
    await user.click(screen.getByTestId("storage-event-upload"));
    await user.click(screen.getByTestId("storage-trigger-submit"));

    await waitFor(() => {
      const row = screen.getByRole("row", { name: /default/i });
      expect(within(row).getByText("default")).toBeInTheDocument();
    });
  });

  it("shows prefix/suffix filters in storage trigger list", async () => {
    mockListStorageTriggers.mockResolvedValue([
      makeStorageTrigger({ prefixFilter: "images/", suffixFilter: ".png" }),
    ]);
    renderTriggers();
    await switchToStorageTab();
    await waitFor(() => {
      expect(screen.getByText("images/")).toBeInTheDocument();
      expect(screen.getByText(".png")).toBeInTheDocument();
    });
  });

  it("disables storage trigger when enabled", async () => {
    const trigger = makeStorageTrigger({ enabled: true });
    mockListStorageTriggers.mockResolvedValue([trigger]);
    mockDisableStorageTrigger.mockResolvedValue(makeStorageTrigger({ enabled: false }));

    renderTriggers();
    const user = await switchToStorageTab();

    await waitFor(() => expect(screen.getByTestId("trigger-toggle-st-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-toggle-st-001"));

    await waitFor(() => {
      expect(mockDisableStorageTrigger).toHaveBeenCalledWith(FUNCTION_ID, "st-001");
    });
  });

  it("enables storage trigger when disabled", async () => {
    const trigger = makeStorageTrigger({ enabled: false });
    mockListStorageTriggers.mockResolvedValue([trigger]);
    mockEnableStorageTrigger.mockResolvedValue(makeStorageTrigger({ enabled: true }));

    renderTriggers();
    const user = await switchToStorageTab();

    await waitFor(() => expect(screen.getByTestId("trigger-toggle-st-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-toggle-st-001"));

    await waitFor(() => {
      expect(mockEnableStorageTrigger).toHaveBeenCalledWith(FUNCTION_ID, "st-001");
    });
    expect(mockDisableStorageTrigger).not.toHaveBeenCalled();
  });

  it("deletes a storage trigger", async () => {
    const trigger = makeStorageTrigger();
    mockListStorageTriggers.mockResolvedValue([trigger]);
    mockDeleteStorageTrigger.mockResolvedValue(undefined);

    renderTriggers();
    const user = await switchToStorageTab();

    await waitFor(() => expect(screen.getByTestId("trigger-delete-st-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-delete-st-001"));

    await waitFor(() => expect(screen.getByTestId("trigger-confirm-delete-st-001")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-confirm-delete-st-001"));

    await waitFor(() => {
      expect(mockDeleteStorageTrigger).toHaveBeenCalledWith(FUNCTION_ID, "st-001");
    });
  });
});

// --- Validation ---

describe("Form validation", () => {
  it("disables DB trigger submit when table name is empty", async () => {
    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("add-db-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-db-trigger-btn"));

    // Don't fill table name, just check events
    await user.click(screen.getByTestId("db-event-INSERT"));
    expect(screen.getByTestId("db-trigger-submit")).toBeDisabled();
  });

  it("disables DB trigger submit when no events selected", async () => {
    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("add-db-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-db-trigger-btn"));

    await user.type(screen.getByTestId("db-trigger-table"), "users");
    // Don't select any events
    expect(screen.getByTestId("db-trigger-submit")).toBeDisabled();
  });

  it("disables cron trigger submit when expression is empty", async () => {
    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-tab-cron")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-tab-cron"));
    await waitFor(() => expect(screen.getByTestId("add-cron-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-cron-trigger-btn"));

    expect(screen.getByTestId("cron-trigger-submit")).toBeDisabled();
  });

  it("disables storage trigger submit when bucket is empty", async () => {
    renderTriggers();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByTestId("trigger-tab-storage")).toBeInTheDocument());
    await user.click(screen.getByTestId("trigger-tab-storage"));
    await waitFor(() => expect(screen.getByTestId("add-storage-trigger-btn")).toBeInTheDocument());
    await user.click(screen.getByTestId("add-storage-trigger-btn"));

    await user.click(screen.getByTestId("storage-event-upload"));
    expect(screen.getByTestId("storage-trigger-submit")).toBeDisabled();
  });
});
