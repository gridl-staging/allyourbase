import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { RecordForm } from "../RecordForm";
import type { Column } from "../../types";

function makeColumn(overrides: Partial<Column> = {}): Column {
  return {
    name: "title",
    position: 1,
    type: "text",
    nullable: false,
    isPrimaryKey: false,
    jsonType: "string",
    ...overrides,
  };
}

const idCol = makeColumn({
  name: "id",
  type: "uuid",
  isPrimaryKey: true,
  default: "gen_random_uuid()",
});
const titleCol = makeColumn({ name: "title", type: "text" });
const bodyCol = makeColumn({ name: "body", type: "text", nullable: true });
const ageCol = makeColumn({ name: "age", type: "integer", jsonType: "number" });
const activeCol = makeColumn({ name: "active", type: "boolean", jsonType: "boolean" });
const metaCol = makeColumn({ name: "meta", type: "jsonb", jsonType: "object" });
const statusCol = makeColumn({
  name: "status",
  type: "text",
  enumValues: ["draft", "published"],
});

describe("RecordForm", () => {
  const onSubmit = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    onSubmit.mockResolvedValue(undefined);
  });

  it("renders create mode title", () => {
    render(
      <RecordForm
        columns={[titleCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    expect(screen.getByText("New Record")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeInTheDocument();
  });

  it("renders edit mode title", () => {
    render(
      <RecordForm
        columns={[titleCol]}
        primaryKey={[]}
        initialData={{ title: "hello" }}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="edit"
      />,
    );
    expect(screen.getByText("Edit Record")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save Changes" }),
    ).toBeInTheDocument();
  });

  it("populates fields from initialData", () => {
    render(
      <RecordForm
        columns={[titleCol, bodyCol]}
        primaryKey={[]}
        initialData={{ title: "Hello", body: "World" }}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="edit"
      />,
    );
    expect(screen.getByDisplayValue("Hello")).toBeInTheDocument();
    expect(screen.getByDisplayValue("World")).toBeInTheDocument();
  });

  it("submits coerced values on create", async () => {
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[idCol, titleCol, ageCol]}
        primaryKey={["id"]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    // id is uuid → renders as input[type=text], title is text → textarea, age is integer → input[type=number].
    // Find title textarea (text type columns render as textarea).
    const textareas = document.querySelectorAll("textarea");
    expect(textareas.length).toBe(1); // Only title is text type.
    await user.type(textareas[0], "My Post");

    // age is number → input[type=number].
    const ageInput = document.querySelector(
      'input[type="number"]',
    ) as HTMLInputElement;
    await user.type(ageInput, "25");

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledOnce();
    });
    const submitted = onSubmit.mock.calls[0][0];
    expect(submitted).not.toHaveProperty("id"); // PK with default skipped.
    expect(submitted.title).toBe("My Post");
    expect(submitted.age).toBe(25); // Coerced to number.
  });

  it("skips PK fields on edit", async () => {
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[idCol, titleCol]}
        primaryKey={["id"]}
        initialData={{ id: "abc-123", title: "Old" }}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="edit"
      />,
    );

    const textareas = document.querySelectorAll("textarea");
    await user.clear(textareas[0]);
    await user.type(textareas[0], "New");
    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledOnce();
    });
    const submitted = onSubmit.mock.calls[0][0];
    expect(submitted).not.toHaveProperty("id");
    expect(submitted.title).toBe("New");
  });

  it("renders boolean as select", () => {
    render(
      <RecordForm
        columns={[activeCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    const select = screen.getByRole("combobox");
    expect(select).toBeInTheDocument();
    expect(screen.getByText("true")).toBeInTheDocument();
    expect(screen.getByText("false")).toBeInTheDocument();
  });

  it("renders enum as select with options", () => {
    render(
      <RecordForm
        columns={[statusCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    expect(screen.getByText("-- select --")).toBeInTheDocument();
    expect(screen.getByText("draft")).toBeInTheDocument();
    expect(screen.getByText("published")).toBeInTheDocument();
  });

  it("renders jsonb as textarea with 5 rows", () => {
    render(
      <RecordForm
        columns={[metaCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    const textarea = document.querySelector("textarea") as HTMLTextAreaElement;
    expect(textarea).toBeInTheDocument();
    expect(textarea.rows).toBe(5);
  });

  it("coerces JSON on submit", async () => {
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[metaCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    const textarea = document.querySelector("textarea") as HTMLTextAreaElement;
    await user.type(textarea, '{{"key":"val"}');
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledOnce();
    });
    expect(onSubmit.mock.calls[0][0].meta).toEqual({ key: "val" });
  });

  it("shows error on submit failure", async () => {
    onSubmit.mockRejectedValueOnce(new Error("constraint violation"));
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[titleCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    const textarea = document.querySelector("textarea") as HTMLTextAreaElement;
    await user.type(textarea, "test");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("constraint violation")).toBeInTheDocument();
    });
  });

  it("disables submit button while saving", async () => {
    onSubmit.mockImplementation(() => new Promise(() => {}));
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[titleCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    const textarea = document.querySelector("textarea") as HTMLTextAreaElement;
    await user.type(textarea, "test");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Saving..." }),
      ).toBeDisabled();
    });
  });

  it("calls onClose when Cancel clicked", async () => {
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[titleCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("shows required indicator for non-nullable columns without default", () => {
    render(
      <RecordForm
        columns={[titleCol, bodyCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    // titleCol is not nullable and has no default → should show *.
    expect(screen.getByText("*")).toBeInTheDocument();
  });

  it("renders UUID field with generate button and placeholder", () => {
    const userIdCol = makeColumn({
      name: "user_id",
      type: "uuid",
    });
    render(
      <RecordForm
        columns={[userIdCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );
    // Should show placeholder with UUID example format.
    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    expect(input.placeholder).toContain("550e8400");
    // Should have a generate button.
    expect(screen.getByTitle("Generate random UUID")).toBeInTheDocument();
  });

  it("generate button fills UUID field with valid UUID", async () => {
    const user = userEvent.setup();
    const userIdCol = makeColumn({
      name: "user_id",
      type: "uuid",
    });
    render(
      <RecordForm
        columns={[userIdCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    await user.click(screen.getByTitle("Generate random UUID"));

    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    // crypto.randomUUID() produces a valid v4 UUID.
    expect(input.value).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    );
  });

  it("shows validation hint for invalid UUID on blur", async () => {
    const user = userEvent.setup();
    const userIdCol = makeColumn({
      name: "user_id",
      type: "uuid",
    });
    render(
      <RecordForm
        columns={[userIdCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    await user.type(input, "not-a-uuid");
    await user.tab(); // blur

    expect(screen.getByText(/click the dice to generate one/i)).toBeInTheDocument();
  });

  it("clears UUID hint when user starts typing again", async () => {
    const user = userEvent.setup();
    const userIdCol = makeColumn({
      name: "user_id",
      type: "uuid",
    });
    render(
      <RecordForm
        columns={[userIdCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    await user.type(input, "bad");
    await user.tab(); // blur → hint appears
    expect(screen.getByText(/click the dice/i)).toBeInTheDocument();

    await user.click(input);
    await user.type(input, "x"); // typing clears the hint
    expect(screen.queryByText(/click the dice/i)).not.toBeInTheDocument();
  });

  it("shows validation hint for invalid JSON on blur", async () => {
    const user = userEvent.setup();
    render(
      <RecordForm
        columns={[metaCol]}
        primaryKey={[]}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="create"
      />,
    );

    const textarea = document.querySelector("textarea") as HTMLTextAreaElement;
    await user.type(textarea, "not json");
    await user.tab(); // blur

    expect(screen.getByText(/Invalid JSON/i)).toBeInTheDocument();
  });

  it("PK fields are disabled in edit mode", () => {
    render(
      <RecordForm
        columns={[idCol, titleCol]}
        primaryKey={["id"]}
        initialData={{ id: "abc", title: "hi" }}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="edit"
      />,
    );
    // uuid renders as input[type=text]; title renders as textarea.
    const idInput = document.querySelector('input[type="text"]') as HTMLInputElement;
    const titleTextarea = document.querySelector("textarea") as HTMLTextAreaElement;
    expect(idInput).toBeDisabled();
    expect(titleTextarea).not.toBeDisabled();
  });

  it("stringifies object initialData values", () => {
    render(
      <RecordForm
        columns={[metaCol]}
        primaryKey={[]}
        initialData={{ meta: { foo: "bar" } }}
        onSubmit={onSubmit}
        onClose={onClose}
        mode="edit"
      />,
    );
    const textarea = document.querySelector("textarea") as HTMLTextAreaElement;
    expect(JSON.parse(textarea.value)).toEqual({ foo: "bar" });
  });
});
