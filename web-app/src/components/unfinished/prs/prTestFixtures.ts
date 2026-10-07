import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import {
  NudgeSessionForPRResponseSchema,
  type NudgeOutcome,
  type NudgeSessionForPRRequest,
} from "@/gen/session/v1/github_user_pb";
import {
  LinkedSessionSchema,
  LinkedSessionStatus,
  UserPRSchema,
  type UserPR,
} from "@/gen/session/v1/types_pb";
import type { NudgeClient } from "@/lib/hooks/useNudgePR";

export function makePR(over: MessageInitShape<typeof UserPRSchema> = {}): UserPR {
  return create(UserPRSchema, {
    owner: "acme",
    repo: "api",
    number: 42,
    title: "Fix flaky lint step",
    htmlUrl: "https://github.com/acme/api/pull/42",
    headRef: "fix-ci",
    baseRef: "main",
    host: "github.com",
    accountLogin: "alice",
    checkConclusion: "success",
    detailsLoaded: true,
    unresolvedThreadCount: 0,
    hasMergeConflict: false,
    ...over,
  });
}

export const session = (id: string, status: LinkedSessionStatus, epoch: number) =>
  create(LinkedSessionSchema, {
    sessionId: id,
    status,
    lastActiveAt: timestampFromDate(new Date(epoch * 1000)),
  });

export const failingPR = (over: MessageInitShape<typeof UserPRSchema> = {}) =>
  makePR({ checkConclusion: "failure", linkedSessions: [session("fix-ci", LinkedSessionStatus.RUNNING, 200)], ...over });

export interface FakeNudgeClient extends NudgeClient {
  calls: NudgeSessionForPRRequest[];
}

/** Resolves each call with the next response (the last repeats); a rejection value is thrown. */
export function fakeNudgeClient(
  ...responses: Array<{ outcome: NudgeOutcome; detail?: string } | Error>
): FakeNudgeClient {
  const calls: NudgeSessionForPRRequest[] = [];
  return {
    calls,
    nudgeSessionForPR: async (req) => {
      const r = responses[Math.min(calls.length, responses.length - 1)];
      calls.push(req);
      if (r instanceof Error) throw r;
      return create(NudgeSessionForPRResponseSchema, { outcome: r.outcome, detail: r.detail ?? "" });
    },
  };
}
