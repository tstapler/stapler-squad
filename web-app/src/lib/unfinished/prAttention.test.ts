import { prAttention } from "./prAttention";

type Input = Parameters<typeof prAttention>[0];

const base: Input = {
  isDraft: false,
  checkConclusion: "success",
  failingChecks: [],
  changesReqCount: 0,
  unresolvedThreadCount: 0,
  hasMergeConflict: false,
};
const pr = (over: Partial<Input>): Input => ({ ...base, ...over });
const check = (name: string) => ({ name, url: "", conclusion: "failure" }) as Input["failingChecks"][number];

describe("prAttention", () => {
  it.each([
    ["failing CI with no itemised check", pr({ checkConclusion: "failure" }), true, false],
    ["failing check list", pr({ failingChecks: [check("lint"), check("unit")] }), true, true],
    ["merge conflict", pr({ hasMergeConflict: true }), true, true],
    ["unresolved threads", pr({ unresolvedThreadCount: 3 }), true, true],
    ["changes requested only", pr({ changesReqCount: 1 }), true, false],
  ])(
    "prAttention_should_FlagNeedsAttention_When_%s",
    (_label, input, needsAttention, nudgeable) => {
      const a = prAttention(input);
      expect(a.needsAttention).toBe(needsAttention);
      expect(a.nudgeable).toBe(nudgeable);
    },
  );

  it.each([
    ["draft with failing CI and conflict", pr({ isDraft: true, checkConclusion: "failure", hasMergeConflict: true })],
    ["pending CI", pr({ checkConclusion: "pending" })],
    ["empty conclusion", pr({ checkConclusion: "" })],
    ["unknown threads and conflict", pr({ unresolvedThreadCount: undefined, hasMergeConflict: undefined })],
    ["green PR", pr({})],
  ])("prAttention_should_NotCount_When_%s", (_label, input) => {
    const a = prAttention(input);
    expect(a.needsAttention).toBe(false);
    expect(a.nudgeable).toBe(false);
  });

  it("prAttention_should_NotBeNudgeable_When_RollupFailsButNoCheckIsItemised", () => {
    const a = prAttention(
      pr({ checkConclusion: "failure", unresolvedThreadCount: undefined, hasMergeConflict: undefined }),
    );
    expect(a).toMatchObject({
      failingChecks: 0,
      checksFailingUnlisted: true,
      threadsUnknown: true,
      conflictUnknown: true,
      needsAttention: true,
      nudgeable: false,
    });
  });

  it("prAttention_should_NotTreatActionRequiredOrTimedOutRollupAsFailing_When_ServerItemisedNothing", () => {
    // Rollup states never carry these; the server itemises per check.
    expect(prAttention(pr({ checkConclusion: "action_required" })).needsAttention).toBe(false);
    expect(prAttention(pr({ checkConclusion: "timed_out" })).needsAttention).toBe(false);
  });

  it("prAttention_should_StayNudgeable_When_RollupFailsAndAnotherReasonIsActionable", () => {
    expect(prAttention(pr({ checkConclusion: "failure", hasMergeConflict: true })).nudgeable).toBe(true);
  });

  it("prAttention_should_SeparateNeedsAttentionFromNudgeable_When_ChangesRequestedOnly", () => {
    expect(prAttention(pr({ changesReqCount: 2 }))).toMatchObject({
      changesRequested: true,
      needsAttention: true,
      nudgeable: false,
    });
    expect(prAttention(pr({ hasMergeConflict: true })).nudgeable).toBe(true);
    expect(prAttention(pr({ hasMergeConflict: undefined })).nudgeable).toBe(false);
  });

  it("prAttention_should_UseListLength_When_MoreFailingChecksThanConclusion", () => {
    expect(prAttention(pr({ failingChecks: [check("a"), check("b")] })).failingChecks).toBe(2);
  });
});
