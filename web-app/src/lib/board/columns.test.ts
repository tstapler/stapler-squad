import { Session, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import { getBoardColumnKey } from "./columns";

function makeSession(status: SessionStatus, subStatus?: SubStatus): Session {
  return { id: "s1", status, subStatus: subStatus ?? SubStatus.UNSPECIFIED } as unknown as Session;
}

describe("getBoardColumnKey", () => {
  it("getBoardColumnKey_should_ReturnNeedsReview_When_ActiveWithNeedsApprovalSubStatus", () => {
    const session = makeSession(SessionStatus.ACTIVE, SubStatus.NEEDS_APPROVAL);
    expect(getBoardColumnKey(session)).toBe("needs_review");
  });

  it("getBoardColumnKey_should_ReturnNeedsReview_When_ActiveWithInputRequiredSubStatus", () => {
    const session = makeSession(SessionStatus.ACTIVE, SubStatus.INPUT_REQUIRED);
    expect(getBoardColumnKey(session)).toBe("needs_review");
  });

  it("getBoardColumnKey_should_ReturnRunning_When_StatusIsActiveWithNoApprovalSubStatus", () => {
    const session = makeSession(SessionStatus.ACTIVE);
    expect(getBoardColumnKey(session)).toBe("running");
  });

  it("getBoardColumnKey_should_ReturnRunning_When_StatusIsCreating", () => {
    const session = makeSession(SessionStatus.CREATING);
    expect(getBoardColumnKey(session)).toBe("running");
  });

  it("getBoardColumnKey_should_ReturnRunning_When_StatusIsRestoring", () => {
    const session = makeSession(SessionStatus.RESTORING);
    expect(getBoardColumnKey(session)).toBe("running");
  });

  it("getBoardColumnKey_should_ReturnPaused_When_StatusIsPaused", () => {
    const session = makeSession(SessionStatus.PAUSED);
    expect(getBoardColumnKey(session)).toBe("paused");
  });

  it("getBoardColumnKey_should_ReturnPaused_When_StatusIsHibernated", () => {
    const session = makeSession(SessionStatus.HIBERNATED);
    expect(getBoardColumnKey(session)).toBe("paused");
  });

  it("getBoardColumnKey_should_ReturnComplete_When_StatusIsStopped", () => {
    const session = makeSession(SessionStatus.STOPPED);
    expect(getBoardColumnKey(session)).toBe("complete");
  });

  it("getBoardColumnKey_should_ReturnRunning_When_StatusIsUnspecified", () => {
    const session = makeSession(SessionStatus.UNSPECIFIED);
    expect(getBoardColumnKey(session)).toBe("running");
  });
});
