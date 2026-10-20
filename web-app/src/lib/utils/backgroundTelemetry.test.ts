import { buildActivityRows } from "@/lib/utils/backgroundActivity";
import { makeElevenStopped, makeHiddenSession } from "@/components/ui/__fixtures__/notificationFixtures";
import { createEmptyEvidenceAlert, stateCountLabels, stateCounts, EMPTY_EVIDENCE_WINDOW_MS } from "./backgroundTelemetry";

const HOUR = 3_600_000;
const unknownOnly = { stopped_unknown: 11 };

describe("backgroundTelemetry", () => {
  it("stateCounts_should_carryCountsByState_When_10StoppedUnknownAnd1Failure", () => {
    const sessions = [
      ...Array.from({ length: 10 }, (_, i) => makeHiddenSession({ id: `diagnose:d${i}` })),
      makeHiddenSession({ id: "review:bad", backgroundOutcome: "failed" }),
    ];
    const counts = stateCounts(buildActivityRows([], sessions));
    expect(counts).toEqual({ stopped_unknown: 10, exception_failure: 1 });
    expect(stateCountLabels(counts)).toEqual({ stopped_unknown: "10", exception_failure: "1" });
  });

  it("Alert_should_raiseOnceBackgroundOutcomeEvidenceEmpty_When_finishedZeroAndStoppedUnknownPositiveFor24hFakeClock", () => {
    const notify = jest.fn();
    const alert = createEmptyEvidenceAlert({ notify });
    for (let h = 0; h <= 48; h += 1) alert.observe(unknownOnly, h * HOUR);
    expect(notify).toHaveBeenCalledTimes(1);
  });

  it("Alert_should_notRaise_When_oneFinishedRowAppearsWithinTheWindow", () => {
    const notify = jest.fn();
    const alert = createEmptyEvidenceAlert({ notify });
    for (let h = 0; h < 24; h += 1) alert.observe(h === 12 ? { ...unknownOnly, finished: 1 } : unknownOnly, h * HOUR);
    alert.observe(unknownOnly, 24 * HOUR);
    expect(notify).not.toHaveBeenCalled();
    alert.observe(unknownOnly, 24 * HOUR + EMPTY_EVIDENCE_WINDOW_MS);
    expect(notify).toHaveBeenCalledTimes(1);
  });

  it("Alert_should_resumeFromPersistedState_And_notRaiseWhenNoStoppedSessions", () => {
    const notify = jest.fn();
    const onChange = jest.fn();
    const alert = createEmptyEvidenceAlert({ notify, onChange, initial: { since: 0, fired: false } });
    alert.observe(stateCounts(buildActivityRows([], [])), 30 * HOUR);
    expect(notify).not.toHaveBeenCalled();
    expect(onChange).toHaveBeenLastCalledWith({ since: null, fired: false });
    const eleven = stateCounts(buildActivityRows([], makeElevenStopped("unknown")));
    expect(eleven.stopped_unknown).toBe(11);
  });
});
