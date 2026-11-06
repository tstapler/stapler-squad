import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { UserPRSchema, type UserPR } from "@/gen/session/v1/types_pb";
import { applyFilter, isReadyToMerge } from "./usePRListFilters";

const pr = (number: number, over: MessageInitShape<typeof UserPRSchema> = {}): UserPR =>
  create(UserPRSchema, {
    owner: "acme",
    repo: "api",
    number,
    title: `PR ${number}`,
    checkConclusion: "success",
    hasMergeConflict: false,
    ...over,
  });

describe("ready-to-merge filter", () => {
  it.each<[string, MessageInitShape<typeof UserPRSchema>, boolean]>([
    ["green, no conflict, not draft, no changes requested", {}, true],
    ["merge conflict unknown", { hasMergeConflict: undefined }, false],
    ["merge conflict", { hasMergeConflict: true }, false],
    ["failing checks", { checkConclusion: "failure" }, false],
    ["pending checks", { checkConclusion: "pending" }, false],
    ["draft", { isDraft: true }, false],
    ["changes requested", { changesReqCount: 1 }, false],
  ])("isReadyToMerge_should_Return_%s", (_name, over, want) => {
    expect(isReadyToMerge(pr(1, over))).toBe(want);
  });

  it("applyFilter_should_KeepOnlyReadyPRs_When_FilterIsReadyToMerge", () => {
    const prs = [pr(1), pr(2, { hasMergeConflict: true }), pr(3, { hasMergeConflict: undefined })];
    expect(applyFilter(prs, "ready-to-merge", "").map((p) => p.number)).toEqual([1]);
    expect(applyFilter(prs, "all", "")).toHaveLength(3);
  });
});
