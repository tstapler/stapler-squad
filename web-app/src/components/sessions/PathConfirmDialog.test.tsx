import { readFileSync } from "fs";
import { join } from "path";
import { render, screen, fireEvent } from "@testing-library/react";
import { PathConfirmDialog } from "./PathConfirmDialog";

// Jest maps *.css.ts to a style stub, so the real zIndex ladder isn't importable.
// Read the real numbers from the source and feed them to the component through a mock,
// so these tests pin the actual ladder values rather than made-up ones.
function realZIndexLadder(): Record<string, number> {
  const src = readFileSync(
    join(__dirname, "../../styles/theme-contract.css.ts"),
    "utf8",
  );
  const block = src.slice(src.indexOf("export const zIndex = {"));
  const entries = [
    ...block
      .slice(0, block.indexOf("} as const"))
      .matchAll(/^\s+(\w+):\s*(\d+),/gm),
  ];
  return Object.fromEntries(entries.map((m) => [m[1], Number(m[2])]));
}

jest.mock("@/styles/theme.css", () => {
  const { readFileSync: read } = jest.requireActual("fs");
  const { join: joinPath } = jest.requireActual("path");
  const src: string = read(
    joinPath(process.cwd(), "src/styles/theme-contract.css.ts"),
    "utf8",
  );
  const block = src.slice(src.indexOf("export const zIndex = {"));
  const body = block.slice(0, block.indexOf("} as const"));
  const zIndex: Record<string, number> = {};
  for (const m of body.matchAll(/^\s+(\w+):\s*(\d+),/gm))
    zIndex[m[1]] = Number(m[2]);
  return { zIndex };
});

describe("PathConfirmDialog", () => {
  it("stacks above the Omnibar overlay (zIndex.modal) so its buttons are clickable", () => {
    render(
      <PathConfirmDialog
        path="/tmp/new"
        onCancel={jest.fn()}
        onConfirm={jest.fn()}
      />,
    );

    // Portaled to document.body, it competes with the Omnibar overlay at page level;
    // the old hardcoded 10 left it covered and unclickable.
    const ladder = realZIndexLadder();
    const dialog = screen.getByTestId("path-confirm-dialog");
    expect(Number(dialog.style.zIndex)).toBe(ladder.dialog);
    expect(ladder.dialog).toBeGreaterThan(ladder.modal);
  });

  it("renders into document.body, outside the Omnibar's own DOM", () => {
    const { container } = render(
      <PathConfirmDialog
        path="/tmp/new"
        onCancel={jest.fn()}
        onConfirm={jest.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(document.body).toContainElement(
      screen.getByTestId("path-confirm-dialog"),
    );
  });

  it("shows the path and routes Cancel and Create & Open to their handlers", () => {
    const onCancel = jest.fn();
    const onConfirm = jest.fn();
    render(
      <PathConfirmDialog
        path="/tmp/new"
        onCancel={onCancel}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("/tmp/new")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Create & Open" }));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
