import { getLastActivityTimestamp, isSessionStale } from "./session-staleness";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

const NOW_MS = 1_700_000_000_000; // fixed reference instant so elapsed-time math is deterministic

function minutesAgo(minutes: number): { seconds: bigint; nanos: number } {
  return { seconds: BigInt(Math.floor(NOW_MS / 1000) - minutes * 60), nanos: 0 };
}

function makeSession(overrides: Record<string, unknown>): Session {
  return {
    id: "s1",
    title: "Test Session",
    status: SessionStatus.ACTIVE,
    ...overrides,
  } as unknown as Session;
}

describe("isSessionStale", () => {
  beforeEach(() => {
    jest.spyOn(Date, "now").mockReturnValue(NOW_MS);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("isSessionStale_should_ReturnFalse_When_SessionIsPausedEvenWithOldActivity", () => {
    const session = makeSession({
      status: SessionStatus.PAUSED,
      lastMeaningfulOutput: minutesAgo(5 * 60),
    });

    expect(isSessionStale(session, 30)).toBe(false);
  });

  it("isSessionStale_should_ReturnTrue_When_ActiveSessionPastThreshold", () => {
    const session = makeSession({
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgo(45),
    });

    expect(isSessionStale(session, 30)).toBe(true);
  });

  it("isSessionStale_should_ReturnFalse_When_ActiveSessionHasNoRecordedActivity", () => {
    const session = makeSession({
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: { seconds: BigInt(0), nanos: 0 },
      lastTerminalUpdate: { seconds: BigInt(0), nanos: 0 },
    });

    expect(isSessionStale(session, 30)).toBe(false);
  });

  it("isSessionStale_should_ReturnFalse_When_ElapsedTimeExactlyEqualsThreshold", () => {
    const session = makeSession({
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgo(30),
    });

    expect(isSessionStale(session, 30)).toBe(false);
  });
});

describe("getLastActivityTimestamp", () => {
  it("getLastActivityTimestamp_should_ReturnLastTerminalUpdate_When_ItIsNewerThanLastMeaningfulOutput", () => {
    const session = makeSession({
      lastMeaningfulOutput: { seconds: BigInt(100), nanos: 0 },
      lastTerminalUpdate: { seconds: BigInt(200), nanos: 0 },
    });

    expect(getLastActivityTimestamp(session)).toEqual({ seconds: BigInt(200), nanos: 0 });
  });
});
