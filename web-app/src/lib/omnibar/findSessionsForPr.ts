import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import {
  SessionService,
  FindSessionsByPullRequestRequestSchema,
  type SessionArtifactHit,
} from "@/gen/session/v1/session_pb";
import { PullRequestRefSchema } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";

export interface PullRequestTarget {
  host: string;
  owner: string;
  repo: string;
  number: number;
}

/** "acme/app#12" */
export function prLabel(pr: PullRequestTarget): string {
  return `${pr.owner}/${pr.repo}#${pr.number}`;
}

/** The artifacts view URL for a PR: /artifacts?pr=acme/app%2312 (the # is encoded, the owner/repo slash is not). */
export function prSearchHref(pr: PullRequestTarget): string {
  return `/artifacts?pr=${encodeURIComponent(pr.owner)}/${encodeURIComponent(pr.repo)}%23${pr.number}`;
}

/** The user-facing name of a hit source (design/ux.md S9). */
export function sourceLabel(source: string): string {
  return source === "session_pr" ? "Tracked" : "Seen in transcript";
}

export type FindPrOutcome =
  | { kind: "single"; sessionId: string; title: string; sourceLabel: string; viewAllHref: string }
  | { kind: "list"; href: string };

/** One match opens that session; none or several go to the artifacts list, which owns the empty state. */
export function classifyFindPrHits(
  hits: ReadonlyArray<Pick<SessionArtifactHit, "sessionId" | "title" | "source">>,
  pr: PullRequestTarget,
): FindPrOutcome {
  const href = prSearchHref(pr);
  if (hits.length === 1) {
    const [hit] = hits;
    return { kind: "single", sessionId: hit.sessionId, title: hit.title, sourceLabel: sourceLabel(hit.source), viewAllHref: href };
  }
  return { kind: "list", href };
}

export async function findSessionsForPr(pr: PullRequestTarget, signal?: AbortSignal): Promise<FindPrOutcome> {
  const client = createClient(SessionService, getConnectTransport());
  const resp = await client.findSessionsByPullRequest(
    create(FindSessionsByPullRequestRequestSchema, {
      pr: create(PullRequestRefSchema, { host: pr.host, owner: pr.owner, repo: pr.repo, number: pr.number }),
    }),
    { signal },
  );
  return classifyFindPrHits(resp.hits, pr);
}
