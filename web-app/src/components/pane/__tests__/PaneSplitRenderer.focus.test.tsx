/**
 * Enforcement test for Bug 2 — focus outline shown on single pane.
 *
 * `leafContainer({ focused: isFocused })` applied the primary-color focus
 * outline even when there was only one pane (no competing pane to distinguish).
 * Fix: gate on `hasSplits` — only show the outline when multiple panes exist.
 *
 * Pre-fix failure: `leafContainer` called with `{ focused: true }` for single
 * pane. Post-fix: must be called with `{ focused: false }` regardless of
 * `isFocused` when there is only one leaf.
 */
import React from "react";
import { render } from "@testing-library/react";
import { PaneSplitRenderer } from "../PaneSplitRenderer";
import type { PaneState } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

// Override the default CSS proxy mock with a spyable jest.fn() for leafContainer.
const mockLeafContainer = jest.fn((_args?: { focused?: boolean }) => "leafContainer");

jest.mock("@/styles/pane/paneSplit.css", () => ({
  splitContainer: jest.fn(() => "splitContainer"),
  leafContainer: (args?: { focused?: boolean }) => mockLeafContainer(args),
  leafZoomed: "leafZoomed",
  emptyPaneSlot: "emptyPaneSlot",
  paneBody: "paneBody",
}));

jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: false, isFoldable: false, isInnerScreen: true }),
}));
jest.mock("@/components/sessions/SessionDetail", () => ({
  SessionDetail: () => <div data-testid="session-detail" />,
}));
// Every pane state in this file uses viewKind "session-detail", so
// SessionListPaneBody (which renders SessionList) never actually mounts --
// but PaneSplitRenderer.tsx still statically imports SessionList at module
// scope regardless of which branch runs at render time, transitively
// loading SessionCard -> RemoteConnectionIndicator -> its .css.ts module.
// That collides with this file's own jest.mock("@/styles/pane/paneSplit.css",
// ...) override above: Jest's moduleNameMapper maps every .css/.css.ts
// import to the SAME shared mock file (src/__mocks__/styleMock.js), so a
// jest.mock() targeting one .css specifier ends up registered against that
// shared resolved path and silently overrides EVERY other .css import in
// this test file too -- including RemoteConnectionIndicator.css, whose real
// `dots` export the mock factory above never defines, crashing this file's
// entire module load with "Cannot read properties of undefined (reading
// 'connected')" before a single test could run. Mocking SessionList (never
// actually exercised by this file's scenarios anyway) stops that import
// chain from loading at all, mirroring the SessionDetail/PaneHeader stubs
// already here.
jest.mock("@/components/sessions/SessionList", () => ({
  SessionList: () => <div data-testid="session-list" />,
}));
// Same collision as SessionList above, via a different import path: PaneSplitRenderer.tsx
// also statically imports SessionBoard (board-kanban-view), which transitively loads
// BoardCard -> SessionCard -> RemoteConnectionIndicator -> its .css.ts module.
jest.mock("@/components/sessions/SessionBoard", () => ({
  SessionBoard: () => <div data-testid="session-board" />,
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
    cancelPicker: jest.fn(),
  }),
}));

const singlePaneState: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "info" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

const splitPaneState: PaneState = {
  root: {
    type: "split",
    id: "s1",
    direction: "vertical",
    ratio: 0.5,
    first: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "info" },
    second: { type: "leaf", id: "p2", viewKind: "session-detail", sessionId: null, activeTab: "info" },
  },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};

describe("PaneSplitRenderer — focus outline gate (Bug 2)", () => {
  beforeEach(() => {
    mockLeafContainer.mockClear();
  });

  it("calls leafContainer with focused:false for a single pane, even if that pane is focused", () => {
    // Single pane: p1 is focused (focusedPaneId === p1.id) but hasSplits is false
    render(
      <PaneSplitRenderer state={singlePaneState} dispatch={jest.fn()} sessions={[]} />
    );

    expect(mockLeafContainer).toHaveBeenCalled();
    // Every leafContainer call must have focused: false — no outline on single pane
    for (const call of mockLeafContainer.mock.calls) {
      expect(call[0]).toEqual({ focused: false });
    }
  });

  it("calls leafContainer with focused:true for the focused pane when splits exist", () => {
    // Two panes: p1 is focused, hasSplits is true
    render(
      <PaneSplitRenderer state={splitPaneState} dispatch={jest.fn()} sessions={[]} />
    );

    const calls = mockLeafContainer.mock.calls.map((c) => c[0]);
    // The focused pane (p1) should receive focused:true
    expect(calls).toContainEqual({ focused: true });
    // The unfocused pane (p2) should receive focused:false
    expect(calls).toContainEqual({ focused: false });
  });

  it("calls leafContainer with focused:false for the unfocused pane when splits exist", () => {
    render(
      <PaneSplitRenderer state={splitPaneState} dispatch={jest.fn()} sessions={[]} />
    );

    const calls = mockLeafContainer.mock.calls.map((c) => c[0]);
    // p2 is not focused
    const unfocusedCalls = calls.filter((c) => c?.focused === false);
    expect(unfocusedCalls.length).toBeGreaterThanOrEqual(1);
  });
});
