/**
 * @feature pane:picker
 *
 * On narrow screens the "Open session in…" sheet offers a fresh full-screen pane,
 * so opening a session never forces it into an existing pane.
 */
import React from "react";
import { render, act, screen, fireEvent } from "@testing-library/react";
import { PaneTilingContainer } from "../PaneTilingContainer";
import type { PaneState, LeafPane } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: true, isFoldable: false, isInnerScreen: false }),
}));
jest.mock("@/lib/pane/usePaneShortcuts", () => ({ usePaneShortcuts: jest.fn() }));
jest.mock("../PaneSplitRenderer", () => ({
  PaneSplitRenderer: () => <div data-testid="pane-split-renderer" />,
}));
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogSessionIndex: () => ({ index: new Map(), loading: false }),
}));

const leaf = (id: string): LeafPane => ({
  type: "leaf", id, viewKind: "session-detail", sessionId: null, activeTab: "terminal",
});

describe("mobile picker sheet", () => {
  it("offers a new full-screen pane that splits the focused pane and assigns the session", async () => {
    const state: PaneState = {
      root: { type: "split", id: "s", direction: "vertical", ratio: 0.5, first: leaf("p1"), second: leaf("p2") },
      focusedPaneId: "p1",
      zoomedPaneId: null,
    };
    const dispatch = jest.fn();
    const session = { id: "session-X" } as Session;
    const props = { sessions: [session], paneState: state, dispatch };

    const { rerender } = render(<PaneTilingContainer {...props} externalSessionAssign={null} />);
    await act(async () => {
      rerender(<PaneTilingContainer {...props} externalSessionAssign={{ sessionId: "session-X", version: 1 }} />);
    });

    fireEvent.click(screen.getByTestId("mobile-picker-new-pane"));

    expect(dispatch).toHaveBeenCalledWith({
      type: "SPLIT_AND_ASSIGN_SESSION",
      paneId: "p1",
      sessionId: "session-X",
      tab: "terminal",
      direction: "horizontal",
    });
    expect(screen.queryByTestId("mobile-picker-new-pane")).toBeNull();
  });
});
