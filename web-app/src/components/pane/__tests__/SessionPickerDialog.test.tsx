import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SessionPickerDialog } from "../SessionPickerDialog";
import type { Session } from "@/gen/session/v1/types_pb";

const s = (id: string, title: string, extra: Partial<Session> = {}) =>
  ({ id, title, branch: "", program: "", category: "", ...extra }) as Session;

const setup = (open: string[] = []) => {
  const onPick = jest.fn();
  const onClose = jest.fn();
  render(
    <SessionPickerDialog
      sessions={[s("1", "Alpha", { branch: "feat/x" }), s("2", "Beta"), s("3", "Gone", { archivedAt: {} as never })]}
      openSessionIds={new Set(open)}
      onPick={onPick}
      onClose={onClose}
    />,
  );
  return { onPick, onClose };
};

describe("SessionPickerDialog", () => {
  it("lists live sessions and hides archived ones", () => {
    setup();
    expect(screen.getByRole("button", { name: /Alpha/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Beta/ })).toBeInTheDocument();
    expect(screen.queryByText("Gone")).not.toBeInTheDocument();
  });

  it("picks a session on click", () => {
    const { onPick } = setup();
    fireEvent.click(screen.getByRole("button", { name: /Beta/ }));
    expect(onPick).toHaveBeenCalledWith("2");
  });

  it("filters by branch and picks the first match with Enter", () => {
    const { onPick } = setup();
    const input = screen.getByLabelText("Filter sessions");
    fireEvent.change(input, { target: { value: "feat/x" } });
    expect(screen.queryByRole("button", { name: /Beta/ })).not.toBeInTheDocument();
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onPick).toHaveBeenCalledWith("1");
  });

  it("says when a session is already open in another pane", () => {
    setup(["2"]);
    expect(screen.getByRole("button", { name: /Beta.*will move here/ })).toBeInTheDocument();
  });

  it("closes from the close button", () => {
    const { onClose } = setup();
    fireEvent.click(screen.getByRole("button", { name: "Close session picker" }));
    expect(onClose).toHaveBeenCalled();
  });
});
