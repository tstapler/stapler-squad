import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { PaneSplitRenderer } from "../PaneSplitRenderer";
import type { PaneState } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("@/components/providers/ViewportProvider", () => ({
  useViewport: () => ({ isMobile: false, isFoldable: false, isInnerScreen: true }),
}));
jest.mock("@/components/sessions/SessionDetail", () => ({ SessionDetail: () => <div data-testid="session-detail" /> }));
jest.mock("@/components/sessions/SessionList", () => ({ SessionList: () => <div /> }));
jest.mock("@/components/sessions/SessionBoard", () => ({ SessionBoard: () => <div /> }));
jest.mock("@/components/pane/PaneHeader", () => ({ PaneHeader: () => <div /> }));
jest.mock("@/components/pane/ResizeHandle", () => ({ ResizeHandle: () => <div /> }));
jest.mock("@/lib/contexts/CockpitActionsContext", () => ({ useCockpitActions: () => ({}) }));
jest.mock("@/components/pane/PaneContext", () => ({
  usePaneContext: () => ({ pickerPendingSession: null, triggerPicker: jest.fn(), cancelPicker: jest.fn(), backlogIndex: new Map() }),
}));

const state: PaneState = {
  root: { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: null, activeTab: "info" },
  focusedPaneId: "p1",
  zoomedPaneId: null,
};
const sessions = [{ id: "s1", title: "Alpha", branch: "", program: "", category: "" } as Session];

describe("empty pane", () => {
  it("opens a session picker and assigns the chosen session to that pane", () => {
    const dispatch = jest.fn();
    render(<PaneSplitRenderer state={state} dispatch={dispatch} sessions={sessions} />);

    fireEvent.click(screen.getByRole("button", { name: /Choose a session/ }));
    fireEvent.click(screen.getByRole("button", { name: /Alpha/ }));

    expect(dispatch).toHaveBeenCalledWith({ type: "ASSIGN_SESSION", paneId: "p1", sessionId: "s1" });
    expect(dispatch).toHaveBeenCalledWith({ type: "FOCUS_PANE", paneId: "p1" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes the picker without assigning anything", () => {
    const dispatch = jest.fn();
    render(<PaneSplitRenderer state={state} dispatch={dispatch} sessions={sessions} />);
    fireEvent.click(screen.getByRole("button", { name: /Choose a session/ }));
    fireEvent.click(screen.getByRole("button", { name: "Close session picker" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(dispatch).not.toHaveBeenCalledWith(expect.objectContaining({ type: "ASSIGN_SESSION" }));
  });
});
