import { getLatestWorkSession } from "./currentWorkSession";
import type { LinkedSession } from "@/lib/hooks/useBacklogService";

function makeSession(overrides: Partial<LinkedSession>): LinkedSession {
  return {
    entityId: "entity-1",
    sessionId: "session-1",
    role: "work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

describe("getLatestWorkSession", () => {
  it("getLatestWorkSession_should_ReturnMostRecentWorkSession_When_MultipleWorkSessionsExist", () => {
    const t1 = "2026-07-01T00:00:00.000Z";
    const t2 = "2026-07-02T00:00:00.000Z";
    const item = {
      linkedSessions: [
        makeSession({ sessionId: "s1", role: "work", startedAt: t1 }),
        makeSession({ sessionId: "s2", role: "work", startedAt: t2 }),
      ],
    };

    expect(getLatestWorkSession(item)?.sessionId).toBe("s2");
  });

  it("getLatestWorkSession_should_ReturnUndefined_When_LinkedSessionsHasNoWorkRole", () => {
    const item = {
      linkedSessions: [
        makeSession({ sessionId: "s1", role: "triage" }),
        makeSession({ sessionId: "s2", role: "review" }),
      ],
    };

    expect(getLatestWorkSession(item)).toBeUndefined();
  });

  it("returns undefined when linkedSessions is empty", () => {
    expect(getLatestWorkSession({ linkedSessions: [] })).toBeUndefined();
  });

  it("returns undefined when item is null or undefined", () => {
    expect(getLatestWorkSession(null)).toBeUndefined();
    expect(getLatestWorkSession(undefined)).toBeUndefined();
  });

  it("returns the sole work session when only one exists", () => {
    const item = {
      linkedSessions: [makeSession({ sessionId: "s1", role: "work" })],
    };

    expect(getLatestWorkSession(item)?.sessionId).toBe("s1");
  });
});
