import React from "react";
import { act, render, screen, within } from "@testing-library/react";
import { Code } from "@connectrpc/connect";
import { SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { attemptColumnMove, SessionBoard } from "./SessionBoard";

// SessionCard pulls in a lot of machinery irrelevant to drag/drop wiring -- same stub
// strategy as SessionBoard.test.tsx.
jest.mock("./SessionCard", () => ({
  SessionCard: ({ session }: { session: Session }) => (
    <div data-testid="session-card">{session.title}</div>
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
