import { create } from "@bufbuild/protobuf";
import { UserPRSchema, type UserPR } from "@/gen/session/v1/types_pb";

let seq = 0;

/** A healthy, non-draft PR; override fields to make it need attention. */
export function makePR(overrides: Partial<Omit<UserPR, "$typeName" | "$unknown">> = {}): UserPR {
  seq += 1;
  return create(UserPRSchema, {
    owner: "acme",
    repo: "api",
    number: seq,
    title: `PR ${seq}`,
    headRef: `branch-${seq}`,
    checkConclusion: "success",
    detailsLoaded: true,
    unresolvedThreadCount: 0,
    hasMergeConflict: false,
    ...overrides,
  });
}

/** 4 PRs: 2 failing CI, 1 merge conflict, 1 healthy => 3 need attention. */
export function makeAttentionPRs(overrides: Partial<Omit<UserPR, "$typeName" | "$unknown">> = {}): UserPR[] {
  return [
    makePR({ checkConclusion: "failure", ...overrides }),
    makePR({ checkConclusion: "failure", ...overrides }),
    makePR({ hasMergeConflict: true, ...overrides }),
    makePR({ ...overrides }),
  ];
}
