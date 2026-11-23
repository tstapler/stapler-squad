import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { MobilePaneTabStrip } from "../MobilePaneTabStrip";
import type { LeafPane } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

const makeLeaf = (id: string, sessionId: string | null = null): LeafPane => ({
  type: "leaf",
  id,
  viewKind: "session-detail",
  sessionId,
  activeTab: "info",
});

const makeSession = (id: string, title: string, extra: Partial<Session> = {}): Session =>
  ({ id, title, branch: "", program: "", category: "", ...extra }) as Session;

const twoPanes = (onFocus = jest.fn(), sessions?: Session[]) =>
  render(
    <MobilePaneTabStrip
      leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", "s2")]}
      focusedPaneId="p1"
      sessions={sessions ?? [makeSession("s1", "Session A"), makeSession("s2", "Session B", { branch: "feat/b" })]}
      onFocus={onFocus}
    />,
  );

describe("MobilePaneTabStrip", () => {
  it("returns null when there is only one leaf", () => {
    const { container } = render(
      <MobilePaneTabStrip leaves={[makeLeaf("p1", "s1")]} focusedPaneId="p1" sessions={[makeSession("s1", "A")]} onFocus={jest.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("shows only the focused pane's name and the pane count, with the list closed", () => {
    twoPanes();
    expect(screen.getByRole("button", { name: /current: Session A/ })).toHaveTextContent("2");
    expect(screen.queryByText("Session B")).not.toBeInTheDocument();
  });

  it("opens a list of all panes and focuses the one tapped", () => {
    const onFocus = jest.fn();
    twoPanes(onFocus);
    fireEvent.click(screen.getByRole("button", { name: /Switch pane/ }));
    expect(screen.getByRole("option", { name: /Session A/ })).toHaveAttribute("aria-selected", "true");
    fireEvent.click(screen.getByRole("option", { name: /Session B/ }));
    expect(onFocus).toHaveBeenCalledWith("p2");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("filters by title and branch", () => {
    twoPanes();
    fireEvent.click(screen.getByRole("button", { name: /Switch pane/ }));
    fireEvent.change(screen.getByLabelText("Filter panes"), { target: { value: "feat/b" } });
    expect(screen.queryByRole("option", { name: /Session A/ })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: /Session B/ })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Filter panes"), { target: { value: "zzz" } });
    expect(screen.getByText(/No panes match/)).toBeInTheDocument();
  });

  it("uses only the first line of a multi-line title", () => {
    twoPanes(jest.fn(), [makeSession("s1", "Short head\nlong paragraph body"), makeSession("s2", "B")]);
    expect(screen.getByRole("button", { name: /current: Short head$/ })).toBeInTheDocument();
  });

  it("labels a pane with no session as Empty", () => {
    render(
      <MobilePaneTabStrip leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", null)]} focusedPaneId="p2" sessions={[makeSession("s1", "A")]} onFocus={jest.fn()} />,
    );
    expect(screen.getByRole("button", { name: /current: Empty/ })).toBeInTheDocument();
  });

  it("calls onAddPane from the + button", () => {
    const onAddPane = jest.fn();
    render(
      <MobilePaneTabStrip leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", "s2")]} focusedPaneId="p1" sessions={[]} onFocus={jest.fn()} onAddPane={onAddPane} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Add new pane" }));
    expect(onAddPane).toHaveBeenCalled();
  });
});
