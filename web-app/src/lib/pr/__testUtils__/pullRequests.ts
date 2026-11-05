import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { PrHoldSchema, PullRequestRefSchema, SessionPullRequestSchema, type PrHold, type SessionPullRequest } from "@/gen/session/v1/types_pb";

type PrInit = Partial<Omit<SessionPullRequest, "createdAt" | "mergedAt" | "closedAt" | "$typeName" | "$unknown">> & {
  number: number;
  /** Epoch milliseconds. */
  createdAt?: number;
  mergedAt?: number;
  closedAt?: number;
};

/** One acme/app PR; timestamps are epoch ms. */
export function makePr(over: PrInit): SessionPullRequest {
  const { createdAt, mergedAt, closedAt, ...rest } = over;
  const ts = (ms?: number) => (ms === undefined ? undefined : timestampFromMs(ms));
  return create(SessionPullRequestSchema, {
    owner: "acme",
    repo: "app",
    state: "open",
    url: `https://github.com/acme/app/pull/${over.number}`,
    ...rest,
    createdAt: ts(createdAt),
    mergedAt: ts(mergedAt),
    closedAt: ts(closedAt),
  });
}

export function makeHold(kind: PrHold["kind"], number: number, over: { ownerSessionId?: string; idleWindowSeconds?: number } = {}): PrHold {
  return create(PrHoldSchema, { kind, pr: create(PullRequestRefSchema, { owner: "acme", repo: "app", number }), ...over });
}
