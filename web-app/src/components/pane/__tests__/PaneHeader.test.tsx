import React from "react";
import { render, screen } from "@testing-library/react";
import { PaneHeader } from "../PaneHeader";
import type { LeafPane } from "@/lib/pane/paneTypes";
import type { Session } from "@/gen/session/v1/types_pb";
import type { SessionActionsOverflowProps } from "@/components/sessions/SessionActionsOverflow";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";

const mockOnRetryNowSession = jest.fn().mockResolvedValue(true);
const mockOnArchiveSession = jest.fn().mockResolvedValue(true);
const mockOnUnarchiveSession = jest.fn().mockResolvedValue(true);
const mockOnHibernateSession = jest.fn();
const mockOnResumeFromHibernation = jest.fn();
jest.mock("@/lib/contexts/CockpitActionsContext", () => ({
  useCockpitActions: () => ({
    onRetryNowSession: mockOnRetryNowSession,
    onArchiveSession: mockOnArchiveSession,
    onUnarchiveSession: mockOnUnarchiveSession,
    onHibernateSession: mockOnHibernateSession,
    onResumeFromHibernation: mockOnResumeFromHibernation,
  }),
}));

const mockUpdate = jest.fn();
jest.mock("@/lib/hooks/useSessionActions", () => ({
  useSessionActions: () => ({ update: mockUpdate }),
}));

// SessionActionsOverflow needs SessionServiceContext, which is irrelevant to the
// badge behavior under test here — stub it out rather than standing up that provider.
type OverflowProps = Pick<
  SessionActionsOverflowProps,
  "onRetryNow" | "onArchive" | "onUnarchive" | "onHibernate" | "onResumeFromHibernation" | "onOpenInNewPane" | "onChangeProgram"
>;
const mockOverflow = jest.fn((_props: OverflowProps) => null);
jest.mock("@/components/sessions/SessionActionsOverflow", () => ({
  SessionActionsOverflow: (props: OverflowProps) => mockOverflow(props),
}));

const mockTriggerPickerForceNew = jest.fn();
const mockUsePaneContext = jest.fn(() => ({
  backlogIndex: new Map<string, BacklogIndexEntry>(),
  triggerPickerForceNew: mockTriggerPickerForceNew,
}));
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
      triggerPickerForceNew: mockTriggerPickerForceNew,
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

describe("PaneHeader — archive wiring", () => {
  it("forwards the cockpit's archive and unarchive actions to the session actions overflow menu", () => {
    const session = { id: "s1", title: "Session One" } as Session;
    const pane: LeafPane = { ...listPane, viewKind: "session-detail", sessionId: "s1" };
    renderHeader({ pane, sessions: [session] });
    const props = mockOverflow.mock.calls[0][0];
    void props.onArchive?.("s1");
    void props.onUnarchive?.("s1");
    expect(mockOnArchiveSession).toHaveBeenCalledWith("s1");
    expect(mockOnUnarchiveSession).toHaveBeenCalledWith("s1");
  });
});

describe("PaneHeader — overflow menu wiring", () => {
  const session = { id: "s1", title: "Session One" } as Session;
  const pane: LeafPane = { ...listPane, viewKind: "session-detail", sessionId: "s1" };

  beforeEach(() => {
    mockOverflow.mockClear();
    renderHeader({ pane, sessions: [session] });
  });

  it("forwards hibernate and resume-from-hibernation to the cockpit", () => {
    const props = mockOverflow.mock.calls[0][0];
    props.onHibernate?.();
    props.onResumeFromHibernation?.();
    expect(mockOnHibernateSession).toHaveBeenCalledWith("s1");
    expect(mockOnResumeFromHibernation).toHaveBeenCalledWith("s1");
  });

  it("opens the session in a new pane via the pane context", () => {
    mockOverflow.mock.calls[0][0].onOpenInNewPane?.();
    expect(mockTriggerPickerForceNew).toHaveBeenCalledWith(session);
  });

  it("changes the program through the session actions", async () => {
    mockUpdate.mockResolvedValueOnce({});
    await mockOverflow.mock.calls[0][0].onChangeProgram?.("s1", "aider");
    expect(mockUpdate).toHaveBeenCalledWith({ program: "aider" });
  });
});
