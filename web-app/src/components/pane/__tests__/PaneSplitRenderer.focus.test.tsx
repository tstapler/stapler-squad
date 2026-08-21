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
