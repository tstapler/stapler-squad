import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import {
  getMostRecentJumpableSession,
  getMostRecentJumpableSessionForRole,
  isJumpableSession,
  listJumpableRoleOptions,
} from "./jumpToSession";

function makeSession(overrides: Partial<LinkedSession> & { sessionId: string; role: string }): LinkedSession {
  return {
    entityId: overrides.sessionId,
    startedAt: "2026-01-01T00:00:00Z",
    endedAt: undefined,
    estimatedCostUsd: 0,
    commitCountSinceSpawn: 0,
    ...overrides,
  } as LinkedSession;
}

describe("isJumpableSession", () => {
  it("isJumpableSession_should_ReturnTrue_When_RoleIsWork", () => {
    expect(isJumpableSession(makeSession({ sessionId: "s1", role: "work" }))).toBe(true);
  });

  it("isJumpableSession_should_ReturnTrue_When_RoleIsReview", () => {
    expect(isJumpableSession(makeSession({ sessionId: "s1", role: "review" }))).toBe(true);
  });

  it("isJumpableSession_should_ReturnFalse_When_RoleIsJulesWork", () => {
    expect(isJumpableSession(makeSession({ sessionId: "jules-sessions/abc", role: "jules_work" }))).toBe(false);
  });

  it("isJumpableSession_should_ReturnFalse_When_RoleIsTriage", () => {
    expect(isJumpableSession(makeSession({ sessionId: "headless-abc", role: "triage" }))).toBe(false);
  });

  it("isJumpableSession_should_ReturnFalse_When_SessionIdIsBlockedGuardrailPrefix", () => {
    expect(isJumpableSession(makeSession({ sessionId: "review-blocked-abc", role: "review" }))).toBe(false);
  });

  it("isJumpableSession_should_ReturnFalse_When_SessionIdIsManualReviewPrefix", () => {
    expect(isJumpableSession(makeSession({ sessionId: "manual-review-abc", role: "review" }))).toBe(false);
  });
});

describe("getMostRecentJumpableSession", () => {
  it("getMostRecentJumpableSession_should_ReturnLastJumpableEntry_When_MultipleRolesPresent", () => {
    const sessions = [
      makeSession({ sessionId: "w1", role: "work" }),
      makeSession({ sessionId: "r1", role: "review" }),
      makeSession({ sessionId: "headless-t1", role: "triage" }),
    ];
    expect(getMostRecentJumpableSession(sessions)?.sessionId).toBe("r1");
  });

  it("getMostRecentJumpableSession_should_SkipTrailingNonJumpableSessions_When_LatestIsTriage", () => {
    const sessions = [
      makeSession({ sessionId: "w1", role: "work" }),
      makeSession({ sessionId: "headless-t1", role: "triage" }),
    ];
    expect(getMostRecentJumpableSession(sessions)?.sessionId).toBe("w1");
  });

  it("getMostRecentJumpableSession_should_ReturnUndefined_When_NoJumpableSessionExists", () => {
    const sessions = [makeSession({ sessionId: "headless-t1", role: "triage" })];
    expect(getMostRecentJumpableSession(sessions)).toBeUndefined();
  });
});

describe("getMostRecentJumpableSessionForRole", () => {
  it("getMostRecentJumpableSessionForRole_should_ReturnLatestOfThatRole_When_RoleHasMultipleSessions", () => {
    const sessions = [
      makeSession({ sessionId: "w1", role: "work" }),
      makeSession({ sessionId: "w2", role: "work" }),
      makeSession({ sessionId: "r1", role: "review" }),
    ];
    expect(getMostRecentJumpableSessionForRole(sessions, "work")?.sessionId).toBe("w2");
  });

  it("getMostRecentJumpableSessionForRole_should_ReturnUndefined_When_RoleHasNoJumpableSession", () => {
    const sessions = [makeSession({ sessionId: "w1", role: "work" })];
    expect(getMostRecentJumpableSessionForRole(sessions, "review")).toBeUndefined();
  });
});

describe("listJumpableRoleOptions", () => {
  it("listJumpableRoleOptions_should_ReturnOneEntryPerRoleMostRecentFirst_When_BothRolesPresent", () => {
    const sessions = [
      makeSession({ sessionId: "r1", role: "review" }),
      makeSession({ sessionId: "w1", role: "work" }),
      makeSession({ sessionId: "w2", role: "work" }),
    ];
    const options = listJumpableRoleOptions(sessions);
    expect(options.map((s) => s.sessionId)).toEqual(["w2", "r1"]);
  });

  it("listJumpableRoleOptions_should_ExcludeJulesAndTriageRows_When_Present", () => {
    const sessions = [
      makeSession({ sessionId: "jules-sessions/abc", role: "jules_work" }),
      makeSession({ sessionId: "headless-t1", role: "triage" }),
      makeSession({ sessionId: "w1", role: "work" }),
    ];
    expect(listJumpableRoleOptions(sessions).map((s) => s.role)).toEqual(["work"]);
  });

  it("listJumpableRoleOptions_should_ReturnEmptyArray_When_NoLinkedSessions", () => {
    expect(listJumpableRoleOptions([])).toEqual([]);
  });
});
