import { answerCountsTowardBadge, badgeCount, type BadgeInputs } from "./badgeCount";

const none: BadgeInputs = { unreadActionable: 0, unconfirmedAnswers: 0, unacknowledgedFailures: 0, escalatedPeerRequests: 0 };

describe("badgeCount", () => {
  it("badgeCount_should_equal6AndTabBadge2_When_3Actionable1Unconfirmed2Failures10Finished1PeerWaiting", () => {
    // 10 Finished rows and 1 un-escalated peer request are not inputs: they contribute nothing.
    expect(badgeCount({ ...none, unreadActionable: 3, unconfirmedAnswers: 1, unacknowledgedFailures: 2, ignored: 11 })).toEqual({
      bell: 6,
      background: 2,
    });
  });

  it.each([
    ["unread actionable", { unreadActionable: 4 }, { bell: 4, background: 0 }],
    ["unconfirmed answers", { unconfirmedAnswers: 2 }, { bell: 2, background: 0 }],
    ["unacknowledged failures", { unacknowledgedFailures: 3 }, { bell: 3, background: 3 }],
    ["escalated peer requests", { escalatedPeerRequests: 1 }, { bell: 1, background: 0 }],
    ["ignored kinds only", { ignored: 25 }, { bell: 0, background: 0 }],
  ])("badgeCount_should_composeTable_When_%s", (_name, over, expected) => {
    expect(badgeCount({ ...none, ...over })).toEqual(expected);
  });

  it("ux_ac67_should_countIndeterminateAt5sButNotPlainSentAt30s", () => {
    expect(answerCountsTowardBadge("send_indeterminate", 5_000)).toBe(true);
    expect(answerCountsTowardBadge("sent", 30_000)).toBe(false);
    expect(answerCountsTowardBadge("sent", 61_000)).toBe(true);
  });
});
