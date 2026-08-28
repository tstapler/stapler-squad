import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { BOARD_COLUMNS, BoardColumnKey } from "./columns";
import { isLegalBoardDrag, isLegalBoardDragForSession, legalBoardTransitions } from "./transitions";

function makeSession(status: SessionStatus): Session {
  return { id: "s1", status } as unknown as Session;
}

describe("isLegalBoardDrag", () => {
  const expected: Record<BoardColumnKey, BoardColumnKey[]> = {
    running: ["paused", "complete"],
    paused: ["running", "complete"],
    needs_review: [],
    complete: [],
  };

  for (const { key: from } of BOARD_COLUMNS) {
    for (const { key: to } of BOARD_COLUMNS) {
      const legal = expected[from].includes(to);
      it(`isLegalBoardDrag_should_Return${legal ? "True" : "False"}_When_DraggingFrom${from}To${to}`, () => {
        expect(isLegalBoardDrag(from, to)).toBe(legal);
      });
    }
  }

  it("isLegalBoardDrag_should_MatchLegalBoardTransitionsTable_When_QueriedForEveryColumn", () => {
    expect(legalBoardTransitions).toEqual(expected);
  });
});

describe("isLegalBoardDragForSession", () => {
  it("isLegalBoardDragForSession_should_ReturnFalse_When_SessionStatusIsCreatingEvenIfColumnDragIsLegal", () => {
    const creatingSession = makeSession(SessionStatus.CREATING);
    expect(isLegalBoardDrag("running", "paused")).toBe(true);
    expect(isLegalBoardDragForSession(creatingSession, "running", "paused")).toBe(false);
  });

  it("isLegalBoardDragForSession_should_ReturnFalse_When_SessionStatusIsRestoringEvenIfColumnDragIsLegal", () => {
    const restoringSession = makeSession(SessionStatus.RESTORING);
    expect(isLegalBoardDrag("running", "complete")).toBe(true);
    expect(isLegalBoardDragForSession(restoringSession, "running", "complete")).toBe(false);
  });

  it("isLegalBoardDragForSession_should_DelegateToIsLegalBoardDrag_When_SessionStatusIsActive", () => {
    const activeSession = makeSession(SessionStatus.ACTIVE);
    expect(isLegalBoardDragForSession(activeSession, "running", "paused")).toBe(true);
    expect(isLegalBoardDragForSession(activeSession, "running", "needs_review")).toBe(false);
  });
});
