import {
  buildActivityRows,
  buildRollup,
  rollupCopy,
  type ActivityRow,
  type RollupVariant,
} from "./backgroundActivity";
import { makeElevenStopped, makeHiddenSession, makeNotification } from "@/components/ui/__fixtures__/notificationFixtures";

const NOW = Date.UTC(2026, 9, 18, 12, 0, 0);
const relative = (ms: number) => `${Math.floor((NOW - ms) / 60_000)}m ago`;
const countBy = (rows: ActivityRow[], state: ActivityRow["state"]) => rows.filter((r) => r.state === state).length;

describe("buildActivityRows", () => {
  it("buildActivityRows_should_return10FinishedAnd1StoppedUnknownAndNoFailuresRollup_When_10CompletedReviewsAnd1UnknownDiagnoseAndEmptyHistory", () => {
    const rows = buildActivityRows([], makeElevenStopped("completed", NOW), { now: NOW });
    expect(rows).toHaveLength(11);
    expect(rows.filter((r) => r.state === "finished" && r.role === "review")).toHaveLength(10);
    expect(rows.filter((r) => r.state === "stopped_unknown" && r.role === "diagnose")).toHaveLength(1);
    expect(buildRollup(rows)).toMatchObject({ variant: "no_failures_reported", total: 11, exceptions: 0 });
  });

  it("buildActivityRows_should_return11StoppedUnknownRows_When_allOutcomesUnknownAndEmptyHistory", () => {
    const rows = buildActivityRows([], makeElevenStopped("unknown", NOW), { now: NOW });
    expect(countBy(rows, "stopped_unknown")).toBe(11);
    expect(buildRollup(rows).variant).toBe("no_failures_reported");
  });

  it("buildActivityRows_should_markExceptionFailureWithoutAnyHistory_When_outcomeFailed", () => {
    const sessions = [...makeElevenStopped("completed", NOW).slice(0, 3), makeHiddenSession({ id: "review:bad", backgroundOutcome: "failed", updatedAtMs: 1 })];
    const rows = buildActivityRows([], sessions, { now: NOW });
    expect(rows[0]).toMatchObject({ sessionId: "review:bad", state: "exception_failure" });
    expect(buildRollup(rows)).toMatchObject({ variant: "needs_attention", exceptions: 1 });
  });

  it("buildActivityRows_should_notMarkFinished_When_outcomeUnknownEvenIfRoutineHistoryRecordPresent", () => {
    const history = [makeNotification({ id: "ok", sessionId: "review:r0", sessionName: "review:r0", notificationType: "task_complete" })];
    const rows = buildActivityRows(history, makeElevenStopped("unknown", NOW), { now: NOW });
    expect(countBy(rows, "finished")).toBe(0);
  });

  it("buildActivityRows_should_pinExceptionFirstAndNotDuplicate_When_unreadErrorRecordForHiddenSession", () => {
    const history = [makeNotification({ id: "e1", sessionId: "review:abc", sessionName: "review:abc", notificationType: "error", timestamp: NOW })];
    const sessions = [...makeElevenStopped("completed", NOW).slice(0, 10), makeHiddenSession({ id: "review:abc", backgroundOutcome: "completed" })];
    const rows = buildActivityRows(history, sessions, { now: NOW });
    expect(rows).toHaveLength(11);
    expect(rows[0]).toMatchObject({ sessionId: "review:abc", state: "exception_failure" });
  });

  it("buildActivityRows_should_giveExceptionPrecedence_When_sessionHasFailureAndPendingDecision", () => {
    const history = [
      makeNotification({ id: "q", sessionId: "review:abc", sessionName: "review:abc", notificationType: "question", timestamp: NOW }),
      makeNotification({ id: "e", sessionId: "review:abc", sessionName: "review:abc", notificationType: "error", timestamp: NOW - 1000 }),
    ];
    const rows = buildActivityRows(history, [makeHiddenSession({ id: "review:abc" })], { now: NOW });
    expect(rows).toHaveLength(1);
    expect(rows[0].state).toBe("exception_failure");
  });

  it("buildActivityRows_should_keepStableOrder_When_sameInputPolledTwice", () => {
    const sessions = makeElevenStopped("completed", NOW);
    const first = buildActivityRows([], sessions, { now: NOW }).map((r) => r.key);
    const second = buildActivityRows([], [...sessions].reverse(), { now: NOW }).map((r) => r.key);
    expect(second).toEqual(first);
  });

  it("buildActivityRows_should_emitDepartedOnceThenDrop_When_sessionMissingFromNextList", () => {
    const old = makeHiddenSession({ id: "review:old" });
    const keep = makeHiddenSession({ id: "review:keep" });
    const first = buildActivityRows([], [old, keep], { now: NOW });
    const second = buildActivityRows([], [keep], { previous: first, now: NOW });
    expect(second.filter((r) => r.sessionId === "review:old")).toEqual([expect.objectContaining({ state: "departed" })]);
    const third = buildActivityRows([], [keep], { previous: second, now: NOW });
    expect(third.some((r) => r.sessionId === "review:old")).toBe(false);
  });

  it("buildActivityRows_should_labelOtherRole_When_tagsUnknown", () => {
    const rows = buildActivityRows([], [makeHiddenSession({ id: "custom-job" })], { now: NOW });
    expect(rows[0].role).toBe("other");
  });
});

describe("buildRollup", () => {
  it("buildRollup_should_returnNoneVariantWithExactCopy_When_zeroHiddenSessions", () => {
    const rollup = buildRollup([]);
    expect(rollup.variant).toBe("none");
    expect(rollupCopy(rollup, relative)).toBe(
      "No background work. Review and diagnose sessions will appear here while they run.",
    );
  });

  it("buildRollup_should_renderNoFailuresCopyWithLastActivity_When_elevenSessions", () => {
    const rollup = buildRollup(buildActivityRows([], makeElevenStopped("unknown", NOW), { now: NOW }));
    expect(rollupCopy(rollup, relative)).toBe("11 background sessions, no failures reported. Last activity 3m ago.");
  });

  it("buildRollup_should_neverEmitTheWordHealthy_When_anyVariant", () => {
    const failed = buildActivityRows([], [makeHiddenSession({ backgroundOutcome: "failed" })], { now: NOW });
    const variants: Record<RollupVariant, ActivityRow[]> = {
      none: [],
      no_failures_reported: buildActivityRows([], makeElevenStopped("completed", NOW), { now: NOW }),
      needs_attention: failed,
    };
    for (const [variant, rows] of Object.entries(variants)) {
      const rollup = buildRollup(rows);
      expect(rollup.variant).toBe(variant);
      expect(rollupCopy(rollup, relative).toLowerCase()).not.toContain("healthy");
    }
  });
});
