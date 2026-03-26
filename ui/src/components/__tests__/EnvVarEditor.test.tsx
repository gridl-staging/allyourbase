import { vi, describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EnvVarEditor, hasEnvVarErrors } from "../edge-functions/EnvVarEditor";

vi.mock("lucide-react", async () => {
  const actual = await vi.importActual("lucide-react");
  return actual;
});

describe("EnvVarEditor", () => {
  it("shows empty state when no vars", () => {
    const onChange = vi.fn();
    render(<EnvVarEditor envVars={[]} onChange={onChange} />);
    expect(screen.getByTestId("env-empty")).toHaveTextContent("No environment variables configured.");
  });

  it("adds a new env var on button click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<EnvVarEditor envVars={[]} onChange={onChange} />);
    await user.click(screen.getByTestId("add-env-var"));
    expect(onChange).toHaveBeenCalledWith([{ key: "", value: "" }]);
  });

  it("removes an env var", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <EnvVarEditor
        envVars={[{ key: "A", value: "1" }, { key: "B", value: "2" }]}
        onChange={onChange}
      />,
    );
    await user.click(screen.getByTestId("env-remove-0"));
    expect(onChange).toHaveBeenCalledWith([{ key: "B", value: "2" }]);
  });

  it("shows duplicate key warnings", () => {
    const onChange = vi.fn();
    render(
      <EnvVarEditor
        envVars={[{ key: "API_KEY", value: "a" }, { key: "API_KEY", value: "b" }]}
        onChange={onChange}
      />,
    );
    expect(screen.getByTestId("env-dupe-0")).toHaveTextContent("Duplicate key");
    expect(screen.getByTestId("env-dupe-1")).toHaveTextContent("Duplicate key");
  });

  it("does not show duplicate warning for unique keys", () => {
    const onChange = vi.fn();
    render(
      <EnvVarEditor
        envVars={[{ key: "A", value: "1" }, { key: "B", value: "2" }]}
        onChange={onChange}
      />,
    );
    expect(screen.queryByTestId("env-dupe-0")).not.toBeInTheDocument();
    expect(screen.queryByTestId("env-dupe-1")).not.toBeInTheDocument();
  });

  it("masks values by default with password input type", () => {
    const onChange = vi.fn();
    render(
      <EnvVarEditor envVars={[{ key: "SECRET", value: "hidden" }]} onChange={onChange} />,
    );
    expect(screen.getByTestId("env-value-0")).toHaveAttribute("type", "password");
  });

  it("reveals value on toggle click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <EnvVarEditor envVars={[{ key: "SECRET", value: "hidden" }]} onChange={onChange} />,
    );
    expect(screen.getByTestId("env-value-0")).toHaveAttribute("type", "password");
    await user.click(screen.getByTestId("env-reveal-0"));
    expect(screen.getByTestId("env-value-0")).toHaveAttribute("type", "text");
  });

  it("hides value again on second toggle", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <EnvVarEditor envVars={[{ key: "S", value: "v" }]} onChange={onChange} />,
    );
    await user.click(screen.getByTestId("env-reveal-0"));
    expect(screen.getByTestId("env-value-0")).toHaveAttribute("type", "text");
    await user.click(screen.getByTestId("env-reveal-0"));
    expect(screen.getByTestId("env-value-0")).toHaveAttribute("type", "password");
  });

  it("highlights duplicate keys with red border", () => {
    const onChange = vi.fn();
    render(
      <EnvVarEditor
        envVars={[{ key: "DUP", value: "1" }, { key: "DUP", value: "2" }]}
        onChange={onChange}
      />,
    );
    expect(screen.getByTestId("env-key-0").className).toContain("border-red");
    expect(screen.getByTestId("env-key-1").className).toContain("border-red");
  });
});

describe("hasEnvVarErrors", () => {
  it("returns true for duplicate keys", () => {
    expect(hasEnvVarErrors([{ key: "A", value: "1" }, { key: "A", value: "2" }])).toBe(true);
  });

  it("returns true for value without key", () => {
    expect(hasEnvVarErrors([{ key: "", value: "orphan" }])).toBe(true);
  });

  it("returns false for valid vars", () => {
    expect(hasEnvVarErrors([{ key: "A", value: "1" }, { key: "B", value: "2" }])).toBe(false);
  });

  it("returns false for empty list", () => {
    expect(hasEnvVarErrors([])).toBe(false);
  });
});
