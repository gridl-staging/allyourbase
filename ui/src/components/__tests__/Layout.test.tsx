import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Layout } from "../Layout";
import type { SchemaCache } from "../../types";

// Mock child components to isolate Layout logic.
vi.mock("../TableBrowser", () => ({
  TableBrowser: ({ table }: { table: { name: string } }) => (
    <div data-testid="table-browser">{table.name}</div>
  ),
}));

vi.mock("../SchemaView", () => ({
  SchemaView: ({ table }: { table: { name: string } }) => (
    <div data-testid="schema-view">{table.name}</div>
  ),
}));

vi.mock("../SqlEditor", () => ({
  SqlEditor: () => <div data-testid="sql-editor" />,
}));

vi.mock("../Webhooks", () => ({
  Webhooks: () => <div data-testid="webhooks-view" />,
}));

vi.mock("../StorageBrowser", () => ({
  StorageBrowser: () => <div data-testid="storage-view" />,
}));

vi.mock("../Users", () => ({
  Users: () => <div data-testid="users-view" />,
}));

vi.mock("../FunctionBrowser", () => ({
  FunctionBrowser: () => <div data-testid="functions-view" />,
}));

vi.mock("../SMSHealth", () => ({
  SMSHealth: () => <div data-testid="sms-health-view" />,
}));

vi.mock("../SMSMessages", () => ({
  SMSMessages: () => <div data-testid="sms-messages-view" />,
}));

vi.mock("../Jobs", () => ({
  Jobs: () => <div data-testid="jobs-view" />,
}));

vi.mock("../Schedules", () => ({
  Schedules: () => <div data-testid="schedules-view" />,
}));

vi.mock("../EmailTemplates", () => ({
  EmailTemplates: () => <div data-testid="email-templates-view" />,
}));

function makeSchema(
  tables: Record<string, { schema: string; name: string; kind: string }> = {},
): SchemaCache {
  const full: SchemaCache = {
    schemas: ["public"],
    builtAt: "2024-01-01T00:00:00Z",
    tables: {},
  };
  for (const [key, t] of Object.entries(tables)) {
    full.tables[key] = {
      ...t,
      columns: [],
      primaryKey: [],
    };
  }
  return full;
}

const twoTableSchema = makeSchema({
  "public.posts": { schema: "public", name: "posts", kind: "table" },
  "public.users": { schema: "public", name: "users", kind: "table" },
});

describe("Layout", () => {
  const onLogout = vi.fn();
  const onRefresh = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders sidebar with table names", () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    // "posts" appears in both sidebar and header, so use getAllByText.
    expect(screen.getAllByText("posts").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("users").length).toBeGreaterThanOrEqual(1);
  });

  it("selects first table by default and shows TableBrowser", () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("shows empty state when no tables", () => {
    render(
      <Layout schema={makeSchema()} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("No tables yet")).toBeInTheDocument();
    expect(screen.getByText("Select a table from the sidebar")).toBeInTheDocument();
  });

  it("clicking a table selects it and switches to data view", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("users"));
    expect(screen.getByTestId("table-browser")).toHaveTextContent("users");
  });

  it("switches to schema view", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Schema"));
    expect(screen.getByTestId("schema-view")).toBeInTheDocument();
  });

  it("switches to SQL view", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("SQL"));
    expect(screen.getByTestId("sql-editor")).toBeInTheDocument();
  });

  it("switching tables resets view to data", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();

    // Go to SQL view first.
    await user.click(screen.getByText("SQL"));
    expect(screen.getByTestId("sql-editor")).toBeInTheDocument();

    // Click another table — should go back to data.
    await user.click(screen.getByText("users"));
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("calls onLogout when logout button clicked", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByTitle("Log out"));
    expect(onLogout).toHaveBeenCalledOnce();
  });

  it("calls onRefresh when refresh button clicked", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByTitle("Refresh schema"));
    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it("shows schema prefix for non-public tables", () => {
    const schema = makeSchema({
      "other.events": { schema: "other", name: "events", kind: "table" },
    });
    render(
      <Layout schema={schema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    // "other." appears in sidebar and header, so use getAllByText.
    expect(screen.getAllByText("other.").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("events").length).toBeGreaterThanOrEqual(1);
  });

  it("shows table kind badge in header", () => {
    const schema = makeSchema({
      "public.my_view": { schema: "public", name: "my_view", kind: "view" },
    });
    render(
      <Layout schema={schema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("view")).toBeInTheDocument();
  });

  it("renders sidebar sections with Database, Services, and Admin groups", () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("Tables")).toBeInTheDocument();
    expect(screen.getByText("Database")).toBeInTheDocument();
    expect(screen.getByText("Services")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("Webhooks")).toBeInTheDocument();
    expect(screen.getByText("Storage")).toBeInTheDocument();
    expect(screen.getByText("Functions")).toBeInTheDocument();
    expect(screen.getByText("SQL Editor")).toBeInTheDocument();
    expect(screen.getByText("RLS Policies")).toBeInTheDocument();
  });

  it("switches to webhooks view on Webhooks click", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Webhooks"));
    expect(screen.getByTestId("webhooks-view")).toBeInTheDocument();
    // Tab bar should not be visible in admin views.
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("switches to storage view on Storage click", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Storage"));
    expect(screen.getByTestId("storage-view")).toBeInTheDocument();
  });

  it("clicking a table from admin view switches back to data view", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();

    // Go to admin view first.
    await user.click(screen.getByText("Webhooks"));
    expect(screen.getByTestId("webhooks-view")).toBeInTheDocument();

    // Click a table — should return to data view.
    await user.click(screen.getByText("posts"));
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("switches to functions view on Functions click", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Functions"));
    expect(screen.getByTestId("functions-view")).toBeInTheDocument();
  });

  it("deselects table when switching to admin view", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Storage"));
    // Header should not show table name.
    expect(screen.queryByTestId("table-browser")).not.toBeInTheDocument();
  });

  it("renders Messaging section in sidebar", () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("Messaging")).toBeInTheDocument();
    expect(screen.getByText("SMS Health")).toBeInTheDocument();
    expect(screen.getByText("SMS Messages")).toBeInTheDocument();
    expect(screen.getByText("Email Templates")).toBeInTheDocument();
  });

  it("clicking SMS Health renders SMSHealth component", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("SMS Health"));
    expect(screen.getByTestId("sms-health-view")).toBeInTheDocument();
    // Tab bar should not be visible in admin views.
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking SMS Messages renders SMSMessages component", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("SMS Messages"));
    expect(screen.getByTestId("sms-messages-view")).toBeInTheDocument();
    // Tab bar should not be visible in admin views.
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking a table from SMS view returns to data view", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    // Go to SMS Health first.
    await user.click(screen.getByText("SMS Health"));
    expect(screen.getByTestId("sms-health-view")).toBeInTheDocument();
    // Click a table — should return to data view.
    await user.click(screen.getByText("posts"));
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("clicking Jobs renders Jobs component", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Jobs"));
    expect(screen.getByTestId("jobs-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Schedules renders Schedules component", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Schedules"));
    expect(screen.getByTestId("schedules-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Email Templates renders EmailTemplates component", async () => {
    render(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Email Templates"));
    expect(screen.getByTestId("email-templates-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });
});
