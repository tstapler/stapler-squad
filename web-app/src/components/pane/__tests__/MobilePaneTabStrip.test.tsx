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

const makeSession = (id: string, title: string): Partial<Session> => ({
  id,
  title,
});

describe("MobilePaneTabStrip", () => {
  it("returns null when there is only one leaf", () => {
    const { container } = render(
      <MobilePaneTabStrip
        leaves={[makeLeaf("p1", "s1")]}
        focusedPaneId="p1"
        sessions={[makeSession("s1", "My Session") as Session]}
        onFocus={jest.fn()}
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders tab buttons when there are multiple leaves", () => {
    render(
      <MobilePaneTabStrip
        leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", "s2")]}
        focusedPaneId="p1"
        sessions={[
          makeSession("s1", "Session A") as Session,
          makeSession("s2", "Session B") as Session,
        ]}
        onFocus={jest.fn()}
      />
    );

    expect(screen.getByRole("tab", { name: "Session A" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Session B" })).toBeInTheDocument();
  });

  it("marks the focused pane tab as selected", () => {
    render(
      <MobilePaneTabStrip
        leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", "s2")]}
        focusedPaneId="p1"
        sessions={[
          makeSession("s1", "Session A") as Session,
          makeSession("s2", "Session B") as Session,
        ]}
        onFocus={jest.fn()}
      />
    );

    expect(screen.getByRole("tab", { name: "Session A" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Session B" })).toHaveAttribute("aria-selected", "false");
  });

  it("calls onFocus with the pane id when a tab is clicked", () => {
    const onFocus = jest.fn();
    render(
      <MobilePaneTabStrip
        leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", "s2")]}
        focusedPaneId="p1"
        sessions={[
          makeSession("s1", "Session A") as Session,
          makeSession("s2", "Session B") as Session,
        ]}
        onFocus={onFocus}
      />
    );

    fireEvent.click(screen.getByRole("tab", { name: "Session B" }));
    expect(onFocus).toHaveBeenCalledWith("p2");
  });

  it("shows 'Empty' label for a pane with no session", () => {
    render(
      <MobilePaneTabStrip
        leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", null)]}
        focusedPaneId="p1"
        sessions={[makeSession("s1", "Session A") as Session]}
        onFocus={jest.fn()}
      />
    );

    expect(screen.getByRole("tab", { name: "Empty" })).toBeInTheDocument();
  });

  it("renders a tablist with accessible label", () => {
    render(
      <MobilePaneTabStrip
        leaves={[makeLeaf("p1", "s1"), makeLeaf("p2", "s2")]}
        focusedPaneId="p1"
        sessions={[
          makeSession("s1", "Session A") as Session,
          makeSession("s2", "Session B") as Session,
        ]}
        onFocus={jest.fn()}
      />
    );

    expect(screen.getByRole("tablist", { name: "Pane switcher" })).toBeInTheDocument();
  });
});
