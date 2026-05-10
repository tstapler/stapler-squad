import React from "react";
import { render, screen } from "@testing-library/react";
import { PaneSplitRenderer } from "../PaneSplitRenderer";
import type { PaneState } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

// Mock viewport — mobile by default; override per test via mockIsMobile
let mockIsMobile = true;
jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: mockIsMobile, isFoldable: false, isInnerScreen: !mockIsMobile }),
}));

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-detail">{session?.title}</div>
  ),
}));

jest.mock("@/components/pane/PaneHeader", () => ({
  PaneHeader: () => <div data-testid="pane-header" />,
}));

jest.mock("@/components/pane/ResizeHandle", () => ({
  ResizeHandle: () => <div data-testid="resize-handle" />,
}));

jest.mock("@/lib/contexts/CockpitActionsContext", () => ({
  useCockpitActions: () => ({}),
}));

jest.mock("@/components/pane/PaneContext", () => ({
  usePaneContext: () => ({
    pickerPendingSession: null,
    triggerPicker: jest.fn(),
    cancelPicker: jest.fn(),
  }),
}));

const makeSession = (id: string, title: string): Partial<Session> => ({ id, title });

const singlePaneState: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "info" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const verticalSplitState: PaneState = {
  root: {
    type: "split",
    id: "s1",
    direction: "vertical",
    ratio: 0.5,
    first: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: "sess1", activeTab: "info" },
    second: { type: "leaf", id: "p2", viewKind: "session-detail", sessionId: "sess2", activeTab: "info" },
  },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const horizontalSplitState: PaneState = {
  root: {
    type: "split",
    id: "s1",
    direction: "horizontal",
    ratio: 0.5,
    first: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: "sess1", activeTab: "info" },
    second: { type: "leaf", id: "p2", viewKind: "session-detail", sessionId: "sess2", activeTab: "info" },
  },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const sessions = [
  makeSession("sess1", "Session One") as Session,
  makeSession("sess2", "Session Two") as Session,
];

describe("PaneSplitRenderer — mobile layout", () => {
  beforeEach(() => {
    mockIsMobile = true;
  });

  describe("single pane (no splits)", () => {
    it("renders the single pane without a reset-layout button", () => {
      render(
        <PaneSplitRenderer state={singlePaneState} dispatch={jest.fn()} sessions={[]} />
      );
      expect(screen.queryByTestId("reset-layout-btn")).not.toBeInTheDocument();
    });

    it("does not render mobile tab strip for single pane", () => {
      render(
        <PaneSplitRenderer state={singlePaneState} dispatch={jest.fn()} sessions={[]} />
      );
      expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    });
  });

  describe("vertical split on mobile", () => {
    it("shows mobile tab strip when there is a vertical split on mobile", () => {
      render(
        <PaneSplitRenderer state={verticalSplitState} dispatch={jest.fn()} sessions={sessions} />
      );
      expect(screen.getByRole("tablist", { name: "Pane switcher" })).toBeInTheDocument();
    });

    it("collapses to show only the focused pane on mobile vertical split", () => {
      render(
        <PaneSplitRenderer state={verticalSplitState} dispatch={jest.fn()} sessions={sessions} />
      );
      // Mobile vertical split shows only the focused pane, not both
      // The focused pane (p1) should be visible, the other not
      const sessionDetails = screen.getAllByTestId("session-detail");
      expect(sessionDetails).toHaveLength(1);
      expect(sessionDetails[0]).toHaveTextContent("Session One");
    });

    it("shows the second pane when it is focused on mobile vertical split", () => {
      const stateWithSecondFocused: PaneState = { ...verticalSplitState, focusedPaneId: "p2" };
      render(
        <PaneSplitRenderer state={stateWithSecondFocused} dispatch={jest.fn()} sessions={sessions} />
      );
      const sessionDetails = screen.getAllByTestId("session-detail");
      expect(sessionDetails).toHaveLength(1);
      expect(sessionDetails[0]).toHaveTextContent("Session Two");
    });

    it("shows the reset layout button when there are multiple panes", () => {
      render(
        <PaneSplitRenderer state={verticalSplitState} dispatch={jest.fn()} sessions={sessions} />
      );
      expect(screen.getByTestId("reset-layout-btn")).toBeInTheDocument();
    });
  });

  describe("horizontal split on mobile", () => {
    it("shows both panes for horizontal split (stacked rows) on mobile", () => {
      render(
        <PaneSplitRenderer state={horizontalSplitState} dispatch={jest.fn()} sessions={sessions} />
      );
      // Horizontal splits are not collapsed on mobile — both panes are visible
      const sessionDetails = screen.getAllByTestId("session-detail");
      expect(sessionDetails).toHaveLength(2);
    });

    it("shows mobile tab strip for horizontal split", () => {
      render(
        <PaneSplitRenderer state={horizontalSplitState} dispatch={jest.fn()} sessions={sessions} />
      );
      // Tab strip shows for any multi-pane layout on mobile (including horizontal splits)
      // so the "+" add-pane button is always accessible on mobile
      expect(screen.getByRole("tablist")).toBeInTheDocument();
    });
  });
});

describe("PaneSplitRenderer — desktop layout", () => {
  beforeEach(() => {
    mockIsMobile = false;
  });

  it("shows both panes side by side for vertical split on desktop", () => {
    render(
      <PaneSplitRenderer state={verticalSplitState} dispatch={jest.fn()} sessions={sessions} />
    );
    const sessionDetails = screen.getAllByTestId("session-detail");
    expect(sessionDetails).toHaveLength(2);
  });

  it("does not show mobile tab strip on desktop", () => {
    render(
      <PaneSplitRenderer state={verticalSplitState} dispatch={jest.fn()} sessions={sessions} />
    );
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });

  it("renders resize handle between panes on desktop", () => {
    render(
      <PaneSplitRenderer state={verticalSplitState} dispatch={jest.fn()} sessions={sessions} />
    );
    expect(screen.getByTestId("resize-handle")).toBeInTheDocument();
  });
});
