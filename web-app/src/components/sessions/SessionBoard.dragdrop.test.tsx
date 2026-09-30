import React from "react";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Code } from "@connectrpc/connect";
import { SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { attemptColumnMove, SessionBoard } from "./SessionBoard";
import { store } from "@/lib/store/store";
import { setError, setErrorCode } from "@/lib/store/sessionsSlice";

// SessionCard pulls in a lot of machinery irrelevant to drag/drop wiring -- same stub
// strategy as SessionBoard.test.tsx. Renders a selection toggle button when selectMode is
// active so the multi-select drag fan-out tests (Task 6.3.1c-d) can select cards through the
// same isSelected/onToggleSelect prop plumbing SessionBoard wires in production.
jest.mock("./SessionCard", () => ({
  SessionCard: ({
    session,
    selectMode,
    isSelected,
    onToggleSelect,
  }: {
    session: Session;
    selectMode?: boolean;
    isSelected?: boolean;
    onToggleSelect?: () => void;
  }) => (
    <div data-testid="session-card">
      {session.title}
      {selectMode && (
        <button
          type="button"
          data-testid={`select-toggle-${session.id}`}
          aria-pressed={isSelected}
          onClick={() => onToggleSelect?.()}
        >
          {isSelected ? "Selected" : "Select"}
        </button>
      )}
    </div>
  ),
}));

const updateSessionMock = jest.fn();
const resumeHibernatedSessionMock = jest.fn();

jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({
    updateSession: updateSessionMock,
    resumeHibernatedSession: resumeHibernatedSessionMock,
  }),
}));

// Captures the live onDragStart/onDragEnd/onDragCancel handlers SessionBoard registers with
// DndContext, so tests can invoke them directly with a constructed DragStart/DragEndEvent
// instead of simulating real pointer drag physics through dnd-kit's sensors (which jsdom
// cannot drive realistically -- getBoundingClientRect-based collision detection needs a real
// layout engine). useDraggable/useDroppable are left as the real implementation; only the
// DndContext provider component is swapped for a thin capture shim.
type CapturedHandlers = {
  onDragStart?: (event: unknown) => void;
  onDragEnd?: (event: unknown) => Promise<void> | void;
  onDragCancel?: (event: unknown) => void;
};
const captured: CapturedHandlers = {};

jest.mock("@dnd-kit/core", () => {
  const actual = jest.requireActual("@dnd-kit/core");
  return {
    ...actual,
    DndContext: (props: {
      children: React.ReactNode;
      onDragStart?: (e: unknown) => void;
      onDragEnd?: (e: unknown) => Promise<void> | void;
      onDragCancel?: (e: unknown) => void;
    }) => {
      captured.onDragStart = props.onDragStart;
      captured.onDragEnd = props.onDragEnd;
      captured.onDragCancel = props.onDragCancel;
      return React.createElement(React.Fragment, null, props.children);
    },
  };
});

let rectHeight = 600;

beforeAll(() => {
  jest.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(() => rectHeight);
  jest.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(() => 320);
  jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () =>
      ({
        width: 320,
        height: 220,
        top: 0,
        left: 0,
        bottom: 220,
        right: 320,
        x: 0,
        y: 0,
        toJSON: () => {},
      }) as DOMRect
  );
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  updateSessionMock.mockReset();
  resumeHibernatedSessionMock.mockReset();
  captured.onDragStart = undefined;
  captured.onDragEnd = undefined;
  captured.onDragCancel = undefined;
});

function makeSession(overrides: Partial<Session> & { id: string; title: string }): Session {
  return {
    status: SessionStatus.ACTIVE,
    subStatus: SubStatus.UNSPECIFIED,
    tags: [],
    category: "",
    path: "/tmp/session",
    branch: "",
    program: "claude",
    ...overrides,
  } as unknown as Session;
}

function dragEvent(activeId: string, overId: string | null) {
  return {
    active: { id: activeId, data: { current: undefined }, rect: { current: {} } },
    over: overId ? { id: overId, rect: {} } : null,
  } as never;
}

describe("attemptColumnMove", () => {
  function makeDeps(overrides: Partial<Parameters<typeof attemptColumnMove>[3]> = {}) {
    return {
      updateSession: jest.fn().mockResolvedValue({ id: "s1" } as Session),
      resumeHibernatedSession: jest.fn().mockResolvedValue({ id: "s1" } as Session),
      approveNeedsReview: jest.fn().mockResolvedValue(true),
      confirmComplete: jest.fn().mockResolvedValue(true),
      getSessionsErrorState: jest.fn().mockReturnValue({ error: null, errorCode: undefined }),
      ...overrides,
    };
  }

  it("attemptColumnMove_should_ReturnMoved_When_LegalActiveToPausedDragSucceeds", async () => {
    const session = makeSession({ id: "sess-123", title: "S", status: SessionStatus.ACTIVE });
    const deps = makeDeps();

    const outcome = await attemptColumnMove(session, "running", "paused", deps);

    expect(deps.updateSession).toHaveBeenCalledWith("sess-123", { status: SessionStatus.PAUSED });
    expect(outcome).toEqual({ type: "moved" });
  });

  it("isLegalBoardDrag_should_RejectIllegal_When_DraggingCompleteToNeedsReview", async () => {
    const session = makeSession({ id: "sess-456", title: "S", status: SessionStatus.STOPPED });
    const deps = makeDeps();

    const outcome = await attemptColumnMove(session, "complete", "needs_review", deps);

    expect(deps.updateSession).not.toHaveBeenCalled();
    expect(outcome).toEqual({ type: "rejected_illegal", from: "complete", to: "needs_review" });
  });

  it("attemptColumnMove_should_ReturnRejectedByServer_When_UpdateSessionResolvesNull", async () => {
    const session = makeSession({ id: "sess-789", title: "S", status: SessionStatus.ACTIVE });
    const deps = makeDeps({
      updateSession: jest.fn().mockResolvedValue(null),
      getSessionsErrorState: jest
        .fn()
        .mockReturnValue({ error: "already stopped", errorCode: Code.FailedPrecondition }),
    });

    const outcome = await attemptColumnMove(session, "running", "complete", deps);

    expect(outcome).toEqual({ type: "rejected_by_server", reason: "already stopped" });
  });

  it("attemptColumnMove_should_ReturnNetworkError_When_UpdateSessionResolvesNullWithNetworkishCode", async () => {
    const session = makeSession({ id: "sess-999", title: "S", status: SessionStatus.ACTIVE });
    const deps = makeDeps({
      updateSession: jest.fn().mockResolvedValue(null),
      getSessionsErrorState: jest.fn().mockReturnValue({ error: "offline", errorCode: Code.Unavailable }),
    });

    const outcome = await attemptColumnMove(session, "running", "paused", deps);

    expect(outcome).toEqual({ type: "network_error" });
  });

  it("attemptColumnMove_should_ShowConfirmDialogAndSkipMutation_When_TargetIsComplete", async () => {
    const session = makeSession({ id: "sess-1", title: "S", status: SessionStatus.ACTIVE });
    let resolveConfirm: (v: boolean) => void = () => {};
    const confirmComplete = jest.fn(
      () => new Promise<boolean>((resolve) => { resolveConfirm = resolve; })
    );
    const deps = makeDeps({ confirmComplete });

    const outcomePromise = attemptColumnMove(session, "running", "complete", deps);
    // Confirmation is pending -- the mutation must not have fired yet.
    await Promise.resolve();
    expect(deps.updateSession).not.toHaveBeenCalled();

    resolveConfirm(true);
    const outcome = await outcomePromise;

    expect(outcome).toEqual({ type: "moved" });
  });

  it("attemptColumnMove_should_ProduceCancelledOutcomeAndNotCallUpdateSession_When_UserCancelsCompleteConfirmation", async () => {
    const session = makeSession({ id: "sess-2", title: "S", status: SessionStatus.ACTIVE });
    const deps = makeDeps({ confirmComplete: jest.fn().mockResolvedValue(false) });

    const outcome = await attemptColumnMove(session, "running", "complete", deps);

    expect(deps.updateSession).not.toHaveBeenCalled();
    expect(outcome).toEqual({ type: "cancelled" });
  });

  it("attemptColumnMove_should_CallStopByUserBranch_When_UserConfirmsCompleteConfirmation", async () => {
    const session = makeSession({ id: "sess-3", title: "S", status: SessionStatus.ACTIVE });
    const deps = makeDeps({ confirmComplete: jest.fn().mockResolvedValue(true) });

    const outcome = await attemptColumnMove(session, "running", "complete", deps);

    expect(deps.updateSession).toHaveBeenCalledWith("sess-3", { status: SessionStatus.STOPPED });
    expect(outcome).toEqual({ type: "moved" });
  });

  it("attemptColumnMove_should_ResolveApproval_When_DraggingFromNeedsReviewToRunning", async () => {
    const session = makeSession({
      id: "sess-321",
      title: "S",
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.NEEDS_APPROVAL,
    });
    const deps = makeDeps();

    const outcome = await attemptColumnMove(session, "needs_review", "running", deps);

    expect(deps.approveNeedsReview).toHaveBeenCalledWith(session);
    expect(deps.updateSession).not.toHaveBeenCalled();
    expect(outcome).toEqual({ type: "moved" });
  });

  it("attemptColumnMove_should_ReturnRejectedByServer_When_ApprovalResolutionFails", async () => {
    const session = makeSession({
      id: "sess-322",
      title: "S",
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.NEEDS_APPROVAL,
    });
    const deps = makeDeps({ approveNeedsReview: jest.fn().mockResolvedValue(false) });

    const outcome = await attemptColumnMove(session, "needs_review", "running", deps);

    expect(outcome.type).toBe("rejected_by_server");
  });

  it("attemptColumnMove_should_ReturnRejectedIllegal_When_NeedsReviewDraggedToAnyTargetOtherThanRunning", async () => {
    const session = makeSession({
      id: "sess-323",
      title: "S",
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.NEEDS_APPROVAL,
    });
    const deps = makeDeps();

    const outcome = await attemptColumnMove(session, "needs_review", "complete", deps);

    expect(deps.approveNeedsReview).not.toHaveBeenCalled();
    expect(deps.confirmComplete).not.toHaveBeenCalled();
    expect(outcome).toEqual({ type: "rejected_illegal", from: "needs_review", to: "complete" });
  });
});

describe("SessionBoard — onDragEnd wiring", () => {
  const sessions: Session[] = [
    makeSession({ id: "run-1", title: "Active session", status: SessionStatus.ACTIVE }),
  ];

  beforeEach(() => {
    rectHeight = 600;
    updateSessionMock.mockResolvedValue({ id: "run-1" } as Session);
  });

  it("onDragEnd_should_CallUpdateSessionWithPausedStatus_When_DraggingActiveCardToPausedColumn", async () => {
    // Keep the RPC pending so the optimistic re-bucket (fired before the await) is observable
    // before the real `sessions` prop (unchanged in this test) would otherwise reclaim the card.
    let resolveUpdate: (session: Session) => void = () => {};
    updateSessionMock.mockReturnValue(
      new Promise<Session>((resolve) => { resolveUpdate = resolve; })
    );

    render(<SessionBoard sessions={sessions} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:run-1", null));
    });
    let dragEndSettled: Promise<void> | undefined;
    act(() => {
      dragEndSettled = captured.onDragEnd?.(dragEvent("__default__:run-1", "__default__:paused")) as
        | Promise<void>
        | undefined;
    });

    expect(updateSessionMock).toHaveBeenCalledTimes(1);
    expect(updateSessionMock).toHaveBeenCalledWith("run-1", { status: SessionStatus.PAUSED });

    // Optimistic: the card renders under Paused immediately, before the RPC resolves.
    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByText("Active session")).toBeInTheDocument();

    await act(async () => {
      resolveUpdate({ id: "run-1" } as Session);
      await dragEndSettled;
    });
  });

  it("onDragEnd_should_SkipRpcAndBounceCardBack_When_DropTargetIsIllegal", async () => {
    const stopped: Session[] = [
      makeSession({ id: "sess-456", title: "Stopped session", status: SessionStatus.STOPPED }),
    ];
    render(<SessionBoard sessions={stopped} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:sess-456", null));
    });
    await act(async () => {
      await captured.onDragEnd?.(dragEvent("__default__:sess-456", "__default__:needs_review"));
    });

    expect(updateSessionMock).not.toHaveBeenCalled();

    const complete = screen.getByTestId("board-column-complete");
    expect(within(complete).getByText("Stopped session")).toBeInTheDocument();

    expect(screen.getByTestId("board-toast")).toHaveTextContent(
      "Can't move a Complete session to Needs Review."
    );
  });

  it("onDragEnd_should_ShowSpecificServerRejectionReason_When_UpdateSessionResolvesNull", async () => {
    // updateSession resolving null (not rejecting) is how useSessionService reports a
    // server-side rejection -- the real failure detail lives in the sessions Redux slice,
    // which attemptColumnMove reads via getSessionsErrorState. The toast must surface that
    // specific reason, not a generic "already changed state" message (AC5's "specific-reason"
    // requirement).
    updateSessionMock.mockResolvedValue(null);
    store.dispatch(setError("session is already stopped"));
    store.dispatch(setErrorCode(Code.FailedPrecondition));

    const running: Session[] = [
      makeSession({ id: "sess-789", title: "Race session", status: SessionStatus.ACTIVE }),
    ];
    render(<SessionBoard sessions={running} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:sess-789", null));
    });
    await act(async () => {
      await captured.onDragEnd?.(dragEvent("__default__:sess-789", "__default__:paused"));
    });

    expect(screen.getByTestId("board-toast")).toHaveTextContent(
      'Couldn\'t move "Race session": session is already stopped'
    );

    store.dispatch(setError(null));
    store.dispatch(setErrorCode(undefined));
  });

  it("dragCancel_should_ProduceCancelledOutcome_When_EscapeInterruptsDrag", async () => {
    render(<SessionBoard sessions={sessions} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:run-1", null));
    });
    await act(async () => {
      captured.onDragCancel?.(dragEvent("__default__:run-1", null));
    });

    expect(updateSessionMock).not.toHaveBeenCalled();
    const running = screen.getByTestId("board-column-running");
    expect(within(running).getByText("Active session")).toBeInTheDocument();
  });

  // Exercises the real BoardCompleteConfirmDialog wiring end-to-end (not just
  // attemptColumnMove's confirmComplete dependency in isolation) -- the dialog SessionBoard
  // itself renders/portals and the promise it resolves on Cancel/Confirm.
  it("dropIntoComplete_should_RenderRealConfirmDialogAndLeaveCardInPlace_When_UserCancels", async () => {
    render(<SessionBoard sessions={sessions} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:run-1", null));
    });
    act(() => {
      void captured.onDragEnd?.(dragEvent("__default__:run-1", "__default__:complete"));
    });

    const dialog = await screen.findByTestId("board-complete-confirm-overlay");
    expect(within(dialog).getByText(/Stop/)).toBeInTheDocument();
    expect(updateSessionMock).not.toHaveBeenCalled();

    await act(async () => {
      screen.getByTestId("board-complete-confirm-cancel").click();
    });

    expect(updateSessionMock).not.toHaveBeenCalled();
    const running = screen.getByTestId("board-column-running");
    expect(within(running).getByText("Active session")).toBeInTheDocument();
  });
});

describe("SessionBoard — drag-freeze against live pushes (Story 3.2.2)", () => {
  beforeEach(() => {
    rectHeight = 600;
    // Never resolves within the test -- freeze must hold for the whole in-flight window.
    updateSessionMock.mockReturnValue(new Promise(() => {}));
  });

  it("dragFreeze_should_KeepCardInPreDragColumn_When_LivePushChangesStatusMidDrag", async () => {
    const initial: Session[] = [
      makeSession({ id: "sess-123", title: "Active session", status: SessionStatus.ACTIVE }),
    ];
    const { rerender } = render(<SessionBoard sessions={initial} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:sess-123", null));
    });

    // Simulate a watchSessions push changing the dragged session's status mid-drag.
    const pushed: Session[] = [
      makeSession({ id: "sess-123", title: "Active session", status: SessionStatus.STOPPED }),
    ];
    rerender(<SessionBoard sessions={pushed} />);

    // Still rendered under Running -- the live push is suppressed for the in-flight ID.
    const running = screen.getByTestId("board-column-running");
    expect(within(running).getByText("Active session")).toBeInTheDocument();
    const complete = screen.getByTestId("board-column-complete");
    expect(within(complete).queryByText("Active session")).not.toBeInTheDocument();
  });
});

// Task 4.1.1a / row 49 of validation.md: the drop handler (onDragEnd) and the MoveToMenu
// selection handler must converge on the exact same attemptColumnMove call -- exercised here
// via the real MoveToMenu UI (not by calling attemptColumnMove directly, which the top
// `describe("attemptColumnMove", ...)` block above already covers in isolation).
//
// Several of these tests resolve the (mocked) updateSession RPC and update the `sessions`
// prop -- simulating the watchSessions live push a Redux-connected parent would deliver, since
// useSessionService's real updateSession dispatches `upsertSession` into the store BEFORE its
// promise resolves (see useSessionService.ts) -- inside the *same* `act()`/order-of-operations
// window. That mirrors production, where the store update and the optimistic-override-clearing
// continuation land close enough together that the card never has a moment to revert to its
// pre-move column; doing the two separately in a test (resolve now, rerender later) would
// introduce an artificial revert-then-recover flicker this static-`sessions`-prop test harness
// doesn't otherwise reproduce.
describe("SessionBoard — MoveToMenu wiring (Phase 4)", () => {
  const sessions: Session[] = [
    makeSession({ id: "run-1", title: "Active session", status: SessionStatus.ACTIVE }),
  ];
  const pausedSessions: Session[] = [
    makeSession({ id: "run-1", title: "Active session", status: SessionStatus.PAUSED }),
  ];

  beforeEach(() => {
    rectHeight = 600;
  });

  it("attemptColumnMove_should_ProduceIdenticalOutcomeAsMoveToMenu_When_SameLogicalMoveTriggeredByDragOrMenu", async () => {
    // Keep the RPC pending, same reason as the drag-triggered version of this assertion above
    // ("onDragEnd_should_CallUpdateSessionWithPausedStatus..."): the optimistic re-bucket is
    // only observable before the mutation settles.
    let resolveUpdate: (session: Session) => void = () => {};
    updateSessionMock.mockReturnValue(
      new Promise<Session>((resolve) => { resolveUpdate = resolve; })
    );
    const user = userEvent.setup();
    render(<SessionBoard sessions={sessions} />);

    const running = screen.getByTestId("board-column-running");
    await user.click(within(running).getByTestId("move-to-menu-trigger"));
    await user.click(screen.getByRole("menuitem", { name: "Paused" }));

    expect(updateSessionMock).toHaveBeenCalledTimes(1);
    expect(updateSessionMock).toHaveBeenCalledWith("run-1", { status: SessionStatus.PAUSED });

    // Optimistic: the card renders under Paused immediately, exactly as the drag path does --
    // same shared attemptColumnMove call, same onOptimisticMove callback.
    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByText("Active session")).toBeInTheDocument();

    await act(async () => {
      resolveUpdate({ id: "run-1" } as Session);
    });
  });

  it("liveRegion_should_AnnounceMovedText_When_MoveToMenuSelectionSucceeds", async () => {
    let resolveUpdate: (session: Session) => void = () => {};
    updateSessionMock.mockReturnValue(
      new Promise<Session>((resolve) => { resolveUpdate = resolve; })
    );
    const user = userEvent.setup();
    const { rerender } = render(<SessionBoard sessions={sessions} />);

    const running = screen.getByTestId("board-column-running");
    await user.click(within(running).getByTestId("move-to-menu-trigger"));
    await user.click(screen.getByRole("menuitem", { name: "Paused" }));

    await act(async () => {
      rerender(<SessionBoard sessions={pausedSessions} />);
      resolveUpdate({ id: "run-1" } as Session);
    });

    expect(screen.getByTestId("board-live-region")).toHaveTextContent(
      "Active session moved to Paused."
    );
  });

  it("moveToMenu_should_PlaceFocusOnTriggerInNewColumn_When_MoveSucceeds", async () => {
    let resolveUpdate: (session: Session) => void = () => {};
    updateSessionMock.mockReturnValue(
      new Promise<Session>((resolve) => { resolveUpdate = resolve; })
    );
    const user = userEvent.setup();
    const { rerender } = render(<SessionBoard sessions={sessions} />);

    const running = screen.getByTestId("board-column-running");
    await user.click(within(running).getByTestId("move-to-menu-trigger"));
    await user.click(screen.getByRole("menuitem", { name: "Paused" }));

    await act(async () => {
      rerender(<SessionBoard sessions={pausedSessions} />);
      resolveUpdate({ id: "run-1" } as Session);
    });

    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByTestId("move-to-menu-trigger")).toHaveFocus();
  });

  it("dragCard_should_PlaceFocusOnDragHandleInNewColumn_When_MoveSucceeds", async () => {
    let resolveUpdate: (session: Session) => void = () => {};
    updateSessionMock.mockReturnValue(
      new Promise<Session>((resolve) => { resolveUpdate = resolve; })
    );
    const { rerender } = render(<SessionBoard sessions={sessions} />);

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:run-1", null));
    });
    let dragEndSettled: Promise<void> | undefined;
    act(() => {
      dragEndSettled = captured.onDragEnd?.(dragEvent("__default__:run-1", "__default__:paused")) as
        | Promise<void>
        | undefined;
    });

    await act(async () => {
      rerender(<SessionBoard sessions={pausedSessions} />);
      resolveUpdate({ id: "run-1" } as Session);
      await dragEndSettled;
    });

    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByLabelText("Drag Active session to move")).toHaveFocus();
  });
});

// Task 6.3.1c-d: dragging a card that's part of the current cross-column selection moves the
// whole selection (client-side fan-out: one attemptColumnMove/updateSession call per selected
// session ID, same target column) -- including surfacing which sessions failed on a partial
// failure (AC8).
describe("SessionBoard — multi-select drag fan-out (Task 6.3.1c-d, AC8)", () => {
  beforeEach(() => {
    rectHeight = 600;
  });

  it("multiSelectDrag_should_CallUpdateSessionOncePerSelectedId_When_DraggingASelectedCard", async () => {
    const sessA = makeSession({ id: "sess-a", title: "Session A", status: SessionStatus.ACTIVE });
    const sessB = makeSession({ id: "sess-b", title: "Session B", status: SessionStatus.ACTIVE });
    updateSessionMock.mockResolvedValue({ id: "sess-a" } as Session);

    const user = userEvent.setup();
    render(<SessionBoard sessions={[sessA, sessB]} />);

    await user.click(screen.getByTestId("board-select-mode-toggle"));
    await user.click(screen.getByTestId("select-toggle-sess-a"));
    await user.click(screen.getByTestId("select-toggle-sess-b"));

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:sess-a", null));
    });
    await act(async () => {
      await captured.onDragEnd?.(dragEvent("__default__:sess-a", "__default__:paused"));
    });

    expect(updateSessionMock).toHaveBeenCalledTimes(2);
    expect(updateSessionMock).toHaveBeenCalledWith("sess-a", { status: SessionStatus.PAUSED });
    expect(updateSessionMock).toHaveBeenCalledWith("sess-b", { status: SessionStatus.PAUSED });

    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByText("Session A")).toBeInTheDocument();
    expect(within(paused).getByText("Session B")).toBeInTheDocument();
  });

  it("multiSelectDrag_should_ReportPerSessionOutcome_When_OneOfTwoSelectedSessionsRejectsTransition", async () => {
    // Session A: Running -> legal drag to Paused. Session B: Complete (Stopped) -> Paused is
    // illegal (legalBoardTransitions["complete"] has no outbound edges) -- exercises AC8's
    // "some selected sessions succeed, some fail -- surface which failed" partial-failure path.
    const sessA = makeSession({ id: "sess-a", title: "Session A", status: SessionStatus.ACTIVE });
    const sessB = makeSession({ id: "sess-b", title: "Session B", status: SessionStatus.STOPPED });
    updateSessionMock.mockResolvedValue({ id: "sess-a" } as Session);

    const user = userEvent.setup();
    render(<SessionBoard sessions={[sessA, sessB]} />);

    await user.click(screen.getByTestId("board-select-mode-toggle"));
    await user.click(screen.getByTestId("select-toggle-sess-a"));
    await user.click(screen.getByTestId("select-toggle-sess-b"));

    await act(async () => {
      captured.onDragStart?.(dragEvent("__default__:sess-a", null));
    });
    await act(async () => {
      await captured.onDragEnd?.(dragEvent("__default__:sess-a", "__default__:paused"));
    });

    // Only the legal transition mutated -- the illegal one never calls updateSession.
    expect(updateSessionMock).toHaveBeenCalledTimes(1);
    expect(updateSessionMock).toHaveBeenCalledWith("sess-a", { status: SessionStatus.PAUSED });

    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByText("Session A")).toBeInTheDocument();
    // Session B stays put -- its rejected transition doesn't move it.
    const complete = screen.getByTestId("board-column-complete");
    expect(within(complete).getByText("Session B")).toBeInTheDocument();

    // Surfaces which session failed, not just that "something" failed.
    expect(screen.getByTestId("board-toast")).toHaveTextContent(
      "Moved 1 of 2 selected sessions to Paused — couldn't move: Session B."
    );
  });
});
