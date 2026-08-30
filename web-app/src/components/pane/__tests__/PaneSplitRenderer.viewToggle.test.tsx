import React, { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { PaneSplitRenderer } from "../PaneSplitRenderer";
import { SessionViewModeProvider } from "@/lib/contexts/SessionViewModeContext";
import type { SessionViewMode } from "@/lib/hooks/useSessionViewMode";
import type { PaneState } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: false, isFoldable: false, isInnerScreen: true }),
}));

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: () => <div data-testid="session-detail" />,
}));
jest.mock("@/components/pane/PaneHeader", () => ({
  PaneHeader: () => <div data-testid="pane-header" />,
}));
jest.mock("@/components/pane/ResizeHandle", () => ({
  ResizeHandle: () => <div />,
}));

jest.mock("@/lib/contexts/CockpitActionsContext", () => ({
  useCockpitActions: () => ({}),
}));

jest.mock("@/components/pane/PaneContext", () => ({
  usePaneContext: () => ({
    pickerPendingSession: null,
    triggerPicker: jest.fn(),
    triggerPickerForceNew: jest.fn(),
    cancelPicker: jest.fn(),
  }),
}));

const mockListSessions = jest.fn();
let mockSessions: Partial<Session>[] = [];
let mockLoading = false;
jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    sessions: mockSessions,
    loading: mockLoading,
    error: null,
    listSessions: mockListSessions,
    hibernateSession: jest.fn(),
    resumeHibernatedSession: jest.fn(),
  }),
}));

const mockSessionListRender = jest.fn();
jest.mock("@/components/sessions/SessionList", () => ({
  SessionList: (props: { sessions: unknown[] }) => {
    mockSessionListRender(props);
    return <div data-testid="session-list-view">list:{props.sessions.length}</div>;
  },
}));

const mockSessionBoardRender = jest.fn();
jest.mock("@/components/sessions/SessionBoard", () => ({
  SessionBoard: (props: { sessions: unknown[] }) => {
    mockSessionBoardRender(props);
    return <div data-testid="session-board-view">board:{props.sessions.length}</div>;
  },
}));

const sessionListPaneState: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-list", sessionId: null, activeTab: "info" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

// Wraps PaneSplitRenderer in a stateful SessionViewModeProvider so clicking the
// toggle buttons exercises the real state flow, mirroring how page.tsx wires it.
function Harness({ initialMode = "list" as SessionViewMode }) {
  const [viewMode, setViewMode] = useState<SessionViewMode>(initialMode);
  return (
    <SessionViewModeProvider value={{ viewMode, setViewMode }}>
      <PaneSplitRenderer state={sessionListPaneState} dispatch={jest.fn()} sessions={mockSessions as Session[]} />
    </SessionViewModeProvider>
  );
}

beforeEach(() => {
  mockSessions = [{ id: "s1", title: "one" } as Partial<Session>, { id: "s2", title: "two" } as Partial<Session>];
  mockLoading = false;
  mockSessionListRender.mockClear();
  mockSessionBoardRender.mockClear();
});

describe("SessionListPaneBody view toggle", () => {
  it("renders SessionList in list mode", () => {
    render(<Harness initialMode="list" />);
    expect(screen.getByTestId("session-list-view")).toBeInTheDocument();
    expect(screen.queryByTestId("session-board-view")).not.toBeInTheDocument();
  });

  it("renders SessionBoard in board mode", () => {
    render(<Harness initialMode="board" />);
    expect(screen.getByTestId("session-board-view")).toBeInTheDocument();
    expect(screen.queryByTestId("session-list-view")).not.toBeInTheDocument();
  });

  it("clicking the Board toggle switches the rendered view without refetching sessions", () => {
    render(<Harness initialMode="list" />);
    expect(screen.getByTestId("session-list-view")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("session-view-mode-board"));

    expect(screen.getByTestId("session-board-view")).toBeInTheDocument();
    expect(screen.queryByTestId("session-list-view")).not.toBeInTheDocument();
    expect(mockListSessions).not.toHaveBeenCalled();
  });

  it("passes the same sessions/props shape to both views (filters/search live in shared props)", () => {
    render(<Harness initialMode="list" />);
    fireEvent.click(screen.getByTestId("session-view-mode-board"));
    const listCallProps = mockSessionListRender.mock.calls[0][0];
    const boardCallProps = mockSessionBoardRender.mock.calls[0][0];
    expect(boardCallProps.sessions).toEqual(listCallProps.sessions);
    expect(boardCallProps.storageKeyPrefix).toEqual(listCallProps.storageKeyPrefix);
  });

  it("marks the active view button aria-pressed=true and the inactive one false", () => {
    render(<Harness initialMode="list" />);
    expect(screen.getByTestId("session-view-mode-list")).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("session-view-mode-board")).toHaveAttribute("aria-pressed", "false");

    fireEvent.click(screen.getByTestId("session-view-mode-board"));

    expect(screen.getByTestId("session-view-mode-list")).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByTestId("session-view-mode-board")).toHaveAttribute("aria-pressed", "true");
  });

  it("announces the switch via the live region", () => {
    render(<Harness initialMode="list" />);
    expect(screen.getByRole("status")).toHaveTextContent("List view, showing 2 sessions");

    fireEvent.click(screen.getByTestId("session-view-mode-board"));

    expect(screen.getByRole("status")).toHaveTextContent("Board view, showing 2 sessions");
  });

  describe("initial loading state", () => {
    beforeEach(() => {
      mockLoading = true;
    });

    it("renders the 4 board-column skeletons (not the empty-state) when loading in board mode", () => {
      render(<Harness initialMode="board" />);
      expect(screen.getByTestId("board-columns-skeleton")).toBeInTheDocument();
      expect(screen.getByTestId("board-column-skeleton-running")).toBeInTheDocument();
      expect(screen.getByTestId("board-column-skeleton-needs_review")).toBeInTheDocument();
      expect(screen.getByTestId("board-column-skeleton-paused")).toBeInTheDocument();
      expect(screen.getByTestId("board-column-skeleton-complete")).toBeInTheDocument();
      expect(screen.queryByTestId("session-board-view")).not.toBeInTheDocument();
    });

    it("keeps the existing row skeleton when loading in list mode", () => {
      render(<Harness initialMode="list" />);
      expect(screen.getByTestId("session-list-skeleton")).toBeInTheDocument();
      expect(screen.queryByTestId("board-columns-skeleton")).not.toBeInTheDocument();
    });
  });
});
