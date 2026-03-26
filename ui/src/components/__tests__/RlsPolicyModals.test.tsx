import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  RlsPolicyCreateModal,
  type CreatePolicyFormState,
} from "../RlsPolicyCreateModal";
import {
  RlsPolicyActionModals,
  type RlsPolicyModalState,
} from "../RlsPolicyActionModals";
import { makePolicy } from "./rls-test-fixtures";

const defaultFormState: CreatePolicyFormState = {
  name: "",
  command: "ALL",
  usingExpression: "",
  withCheckExpression: "",
  isPermissive: true,
};

describe("RlsPolicyCreateModal", () => {
  it("returns null when closed", () => {
    const { container } = render(
      <RlsPolicyCreateModal
        isOpen={false}
        selectedTable={{ schema: "public", name: "posts" }}
        formState={defaultFormState}
        isSubmitting={false}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
        onApplyTemplate={vi.fn()}
        onNameChange={vi.fn()}
        onCommandChange={vi.fn()}
        onPermissiveChange={vi.fn()}
        onUsingExpressionChange={vi.fn()}
        onWithCheckExpressionChange={vi.fn()}
      />,
    );

    expect(container.firstChild).toBeNull();
  });

  it("wires template/apply/submit callbacks", async () => {
    const user = userEvent.setup();
    const onApplyTemplate = vi.fn();
    const onSubmit = vi.fn();

    render(
      <RlsPolicyCreateModal
        isOpen
        selectedTable={{ schema: "public", name: "posts" }}
        formState={{ ...defaultFormState, name: "policy_a" }}
        isSubmitting={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
        onApplyTemplate={onApplyTemplate}
        onNameChange={vi.fn()}
        onCommandChange={vi.fn()}
        onPermissiveChange={vi.fn()}
        onUsingExpressionChange={vi.fn()}
        onWithCheckExpressionChange={vi.fn()}
      />,
    );

    await user.click(screen.getByText("Owner only"));
    await user.click(screen.getByRole("button", { name: "Create Policy" }));

    expect(onApplyTemplate).toHaveBeenCalledTimes(1);
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});

describe("RlsPolicyActionModals", () => {
  it("renders and confirms delete modal", async () => {
    const user = userEvent.setup();
    const onConfirmDelete = vi.fn();

    render(
      <RlsPolicyActionModals
        modal={{ kind: "delete", policy: makePolicy() }}
        isDeleting={false}
        onClose={vi.fn()}
        onConfirmDelete={onConfirmDelete}
      />,
    );

    expect(screen.getByText("Delete Policy")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirmDelete).toHaveBeenCalledTimes(1);
  });

  it("renders SQL preview modal", () => {
    const modal: RlsPolicyModalState = {
      kind: "sql-preview",
      sql: 'CREATE POLICY "owner_access" ON "public"."posts" FOR ALL;',
    };

    render(
      <RlsPolicyActionModals
        modal={modal}
        isDeleting={false}
        onClose={vi.fn()}
        onConfirmDelete={vi.fn()}
      />,
    );

    expect(screen.getByText("SQL Preview")).toBeInTheDocument();
    expect(screen.getByText(/CREATE POLICY/)).toBeInTheDocument();
  });
});
