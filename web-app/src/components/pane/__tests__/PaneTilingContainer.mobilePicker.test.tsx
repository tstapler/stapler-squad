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

jest.mock("@/components/sessions/SessionPeekModal", () => ({
  SessionPeekModal: ({ session }: { session: { id: string } }) => (
    <div data-testid="session-peek-modal">{session.id}</div>
  ),
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

  it("lists Open existing pane first when the session is already in a pane, and focuses it", async () => {
    const held: LeafPane = { ...leaf("p2"), sessionId: "session-X" };
    const state: PaneState = {
      root: { type: "split", id: "s", direction: "vertical", ratio: 0.5, first: leaf("p1"), second: held },
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

    const existing = screen.getByTestId("mobile-picker-open-existing");
    expect(existing.nextElementSibling).toBe(screen.getByTestId("mobile-picker-new-pane"));
    fireEvent.click(existing);

    expect(dispatch).toHaveBeenCalledWith({ type: "FOCUS_PANE", paneId: "p2" });
    expect(screen.queryByTestId("mobile-picker-open-existing")).toBeNull();
  });

  it("offers a Temporary modal between New full-screen pane and existing panes; tapping it closes the sheet and opens the peek modal", async () => {
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

    const peek = screen.getByTestId("mobile-picker-peek-modal");
    expect(screen.getByTestId("mobile-picker-new-pane").nextElementSibling).toBe(peek);
    expect(screen.queryByTestId("session-peek-modal")).toBeNull();

    fireEvent.click(peek);

    expect(screen.queryByTestId("mobile-picker-peek-modal")).toBeNull();
    expect(screen.getByTestId("session-peek-modal").textContent).toBe("session-X");
    expect(dispatch).not.toHaveBeenCalled();
  });

  it("keeps the fixed actions and Cancel outside a scrollable pane list when 15+ panes exist", async () => {
    const ids = Array.from({ length: 16 }, (_, i) => `p${i}`);
    const held: LeafPane = { ...leaf("p1"), sessionId: "session-X" };
    const root = ids.slice(1).reduce<PaneState["root"]>(
      (acc, id, i) => ({
        type: "split", id: `s${i}`, direction: "vertical", ratio: 0.5,
        first: acc, second: id === "p1" ? held : leaf(id),
      }),
      leaf("p0"),
    );
    const state: PaneState = { root, focusedPaneId: "p0", zoomedPaneId: null };
    const dispatch = jest.fn();
    const session = { id: "session-X" } as Session;
    const props = { sessions: [session], paneState: state, dispatch };

    const { rerender } = render(<PaneTilingContainer {...props} externalSessionAssign={null} />);
    await act(async () => {
      rerender(<PaneTilingContainer {...props} externalSessionAssign={{ sessionId: "session-X", version: 1 }} />);
    });

    const list = screen.getByTestId("mobile-picker-pane-list");
    expect(list.children.length).toBeGreaterThanOrEqual(15);
    for (const testId of ["mobile-picker-open-existing", "mobile-picker-new-pane", "mobile-picker-peek-modal"]) {
      expect(list.contains(screen.getByTestId(testId))).toBe(false);
    }
    const cancel = screen.getByText("Cancel");
    expect(list.contains(cancel)).toBe(false);
    expect(list.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("omits the pane list when there are no other panes", async () => {
    const state: PaneState = { root: leaf("p1"), focusedPaneId: "p1", zoomedPaneId: null };
    const dispatch = jest.fn();
    const session = { id: "session-X" } as Session;
    const props = { sessions: [session], paneState: state, dispatch };

    const { rerender } = render(<PaneTilingContainer {...props} externalSessionAssign={null} />);
    await act(async () => {
      rerender(<PaneTilingContainer {...props} externalSessionAssign={{ sessionId: "session-X", version: 1 }} />);
    });

    expect(screen.queryByTestId("mobile-picker-pane-list")).toBeNull();
  });
});
