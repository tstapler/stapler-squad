/**
 * Tests for scroll wrapper in SessionListPaneBody (Bug 2 fix).
 *
 * Covers:
 *  - TC-2.1: Scroll container div (overflowY: "auto") exists when session-list pane is rendered
 *  - TC-2.2: SessionList is rendered inside the scroll wrapper (descendant, not sibling)
 *  - TC-2.3: Scroll wrapper does not set overflowX
 *  - TC-2.4: Non-session-list pane (session-detail) does not have overflowY: auto wrapper
 */

import React from "react";
import { render } from "@testing-library/react";
import { PaneSplitRenderer } from "../PaneSplitRenderer";
import type { PaneState } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: true, isFoldable: false, isInnerScreen: false }),
}));

jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: () => <div data-testid="session-detail" />,
}));

jest.mock("@/components/pane/PaneHeader", () => ({
  PaneHeader: () => <div data-testid="pane-header" />,
}));

jest.mock("@/components/pane/ResizeHandle", () => ({
  ResizeHandle: () => <div data-testid="resize-handle" />,
}));

// Mock SessionList to avoid heavy transitive dependencies while still
// rendering a recognizable child we can check containment of.
jest.mock("@/components/sessions/SessionList", () => ({
  SessionList: () => <div data-testid="session-list" />,
}));

jest.mock("@/lib/contexts/CockpitActionsContext", () => ({
  useCockpitActions: () => ({ sessions: [], loading: false, error: null }),
}));

jest.mock("@/components/pane/PaneContext", () => ({
  usePaneContext: () => ({
    pickerPendingSession: null,
    triggerPicker: jest.fn(),
    triggerPickerForceNew: jest.fn(),
    cancelPicker: jest.fn(),
  }),
}));

// ---------------------------------------------------------------------------
// Pane state fixtures
// ---------------------------------------------------------------------------

const sessionListPaneState: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-list", sessionId: null, activeTab: "info" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const sessionDetailPaneState: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "info" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const makeSession = (id: string, title: string): Partial<Session> => ({
  id,
  title,
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SessionListPaneBody — scroll wrapper", () => {
  it("SessionListPaneBody_should_renderScrollContainer_When_sessionsExist", () => {
    render(
      <PaneSplitRenderer
        state={sessionListPaneState}
        dispatch={jest.fn()}
        sessions={[makeSession("s1", "S1") as Session]}
      />
    );
    // React renders overflowY as CSS property "overflow-y"
    const scrollDiv = document.querySelector('[style*="overflow-y"]') as HTMLElement | null;
    expect(scrollDiv).not.toBeNull();
    expect(scrollDiv!.style.overflowY).toBe("auto");
  });

  it("SessionListPaneBody_should_renderSessionListInsideScrollWrapper", () => {
    const { getByTestId } = render(
      <PaneSplitRenderer
        state={sessionListPaneState}
        dispatch={jest.fn()}
        sessions={[makeSession("s1", "S1") as Session]}
      />
    );
    const sessionList = getByTestId("session-list");
    // The parent of the SessionList mock should be the scroll wrapper div
    const parent = sessionList.parentElement as HTMLElement;
    expect(parent).not.toBeNull();
    expect(parent.style.overflowY).toBe("auto");
  });

  it("SessionListPaneBody_should_notSetOverflowX_on_scrollWrapper", () => {
    render(
      <PaneSplitRenderer
        state={sessionListPaneState}
        dispatch={jest.fn()}
        sessions={[makeSession("s1", "S1") as Session]}
      />
    );
    // React renders overflowY as CSS property "overflow-y"
    const scrollDiv = document.querySelector('[style*="overflow-y"]') as HTMLElement | null;
    expect(scrollDiv).not.toBeNull();
    // overflowX should not be "auto" or "scroll" — it is not set in the inline style
    expect(scrollDiv!.style.overflowX).not.toBe("auto");
    expect(scrollDiv!.style.overflowX).not.toBe("scroll");
  });

  it("SessionListPaneBody_should_notWrapNonSessionListPane", () => {
    render(
      <PaneSplitRenderer
        state={sessionDetailPaneState}
        dispatch={jest.fn()}
        sessions={[]}
      />
    );
    // The scroll wrapper (overflowY: auto) is exclusive to session-list pane body
    const scrollDiv = document.querySelector('[style*="overflow-y"]') as HTMLElement | null;
    expect(scrollDiv).toBeNull();
  });
});
