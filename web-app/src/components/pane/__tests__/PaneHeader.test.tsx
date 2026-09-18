import React from "react";
import { render, screen } from "@testing-library/react";
import { PaneHeader } from "../PaneHeader";
import type { LeafPane } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("@/lib/contexts/CockpitActionsContext", () => ({
  useCockpitActions: () => ({}),
}));

const listPane: LeafPane = { type: "leaf", id: "p1", viewKind: "session-list", sessionId: null, activeTab: "info" };

function renderHeader(overrides: Partial<React.ComponentProps<typeof PaneHeader>> = {}) {
  return render(
    <PaneHeader
      pane={listPane}
      sessions={[] as Session[]}
      isFocused={false}
      onClose={jest.fn()}
      onFocus={jest.fn()}
      onZoom={jest.fn()}
      onSplitVertical={jest.fn()}
      onSplitHorizontal={jest.fn()}
      {...overrides}
    />
  );
}

describe("PaneHeader — split button visibility", () => {
  it("renders split buttons when splitButtonVisible is true", () => {
    renderHeader({ splitButtonVisible: true });
    expect(screen.getByTestId("pane-split-vertical-btn")).toBeInTheDocument();
    expect(screen.getByTestId("pane-split-horizontal-btn")).toBeInTheDocument();
  });

  it("does not render split buttons when splitButtonVisible is false (mobile)", () => {
    renderHeader({ splitButtonVisible: false });
    expect(screen.queryByTestId("pane-split-vertical-btn")).not.toBeInTheDocument();
    expect(screen.queryByTestId("pane-split-horizontal-btn")).not.toBeInTheDocument();
  });

  it("does not render split buttons when splitButtonVisible is omitted", () => {
    renderHeader();
    expect(screen.queryByTestId("pane-split-vertical-btn")).not.toBeInTheDocument();
    expect(screen.queryByTestId("pane-split-horizontal-btn")).not.toBeInTheDocument();
  });
});
