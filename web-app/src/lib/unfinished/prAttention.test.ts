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
    ["failing CI", pr({ checkConclusion: "failure" }), true, true],
    ["timed_out CI", pr({ checkConclusion: "timed_out" }), true, true],
    ["action_required CI", pr({ checkConclusion: "action_required" }), true, true],
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

  it("prAttention_should_CountFailingCheckAndFlagUnknowns_When_DetailsNotLoaded", () => {
    const a = prAttention(
      pr({ checkConclusion: "failure", unresolvedThreadCount: undefined, hasMergeConflict: undefined }),
    );
    expect(a).toMatchObject({
      failingChecks: 1,
      unresolvedThreads: 0,
      threadsUnknown: true,
      conflictUnknown: true,
      needsAttention: true,
      nudgeable: true,
    });
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
