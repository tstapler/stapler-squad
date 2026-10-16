import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { statusForColumnMove } from "./statusForColumnMove";

function makeSession(status: SessionStatus): Session {
  return { id: "s1", status } as unknown as Session;
}

describe("statusForColumnMove", () => {
  it("statusForColumnMove_should_ReturnPaused_When_TargetColumnIsPausedFromActive", () => {
    expect(statusForColumnMove(makeSession(SessionStatus.ACTIVE), "paused")).toBe(SessionStatus.PAUSED);
  });

  it("statusForColumnMove_should_ReturnStopped_When_TargetColumnIsComplete", () => {
    expect(statusForColumnMove(makeSession(SessionStatus.ACTIVE), "complete")).toBe(SessionStatus.STOPPED);
  });

  it("statusForColumnMove_should_ReturnActive_When_TargetIsRunningFromPaused", () => {
    expect(statusForColumnMove(makeSession(SessionStatus.PAUSED), "running")).toBe(SessionStatus.ACTIVE);
  });

  it("statusForColumnMove_should_ReturnNull_When_TargetIsRunningFromHibernated", () => {
    expect(statusForColumnMove(makeSession(SessionStatus.HIBERNATED), "running")).toBeNull();
  });

  it("statusForColumnMove_should_ReturnNull_When_TargetColumnIsNeedsReview", () => {
    expect(statusForColumnMove(makeSession(SessionStatus.ACTIVE), "needs_review")).toBeNull();
  });
});
