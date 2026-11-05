import React from "react";
import { render, screen } from "@testing-library/react";
import { PaneHeader } from "../PaneHeader";
import type { LeafPane } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";

const mockOnRetryNowSession = jest.fn().mockResolvedValue(true);
jest.mock("@/lib/contexts/CockpitActionsContext", () => ({
  useCockpitActions: () => ({ onRetryNowSession: mockOnRetryNowSession }),
}));

// SessionActionsOverflow needs SessionServiceContext, which is irrelevant to the
// badge behavior under test here — stub it out rather than standing up that provider.
const mockOverflow = jest.fn((_props: { onRetryNow?: (id: string) => Promise<boolean | void> }) => null);
jest.mock("@/components/sessions/SessionActionsOverflow", () => ({
  SessionActionsOverflow: (props: { onRetryNow?: (id: string) => Promise<boolean | void> }) => mockOverflow(props),
}));

const mockUsePaneContext = jest.fn(() => ({ backlogIndex: new Map<string, BacklogIndexEntry>() }));
jest.mock("../PaneContext", () => ({
  usePaneContext: () => mockUsePaneContext(),
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

describe("PaneHeader — backlog-origin badge", () => {
  it("renders the badge when the pane context's backlogIndex has an entry for the pane's session", () => {
    const session = { id: "s1", title: "My Session" } as Session;
    const detailPane: LeafPane = { type: "leaf", id: "p1", viewKind: "session-detail", sessionId: "s1", activeTab: "terminal" };
    mockUsePaneContext.mockReturnValueOnce({
      backlogIndex: new Map<string, BacklogIndexEntry>([
        ["s1", { itemId: "item-1", itemTitle: "Fix the thing", itemStatus: "in_progress", sessionRole: "work" }],
      ]),
    });

    renderHeader({ pane: detailPane, sessions: [session] });

    expect(screen.getByTestId("backlog-origin-badge")).toBeInTheDocument();
  });
});

describe("PaneHeader — retry now wiring", () => {
  it("forwards the cockpit's onRetryNowSession to the session actions overflow menu", () => {
    const session = { id: "s1", title: "Session One" } as Session;
    const pane: LeafPane = { ...listPane, viewKind: "session-detail", sessionId: "s1" };
    renderHeader({ pane, sessions: [session] });
    const props = mockOverflow.mock.calls[0][0];
    void props.onRetryNow?.("s1");
    expect(mockOnRetryNowSession).toHaveBeenCalledWith("s1");
  });
});
