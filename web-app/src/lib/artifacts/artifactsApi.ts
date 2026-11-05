import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import {
  SessionService,
  FindSessionsByPullRequestRequestSchema,
  ListSessionArtifactsRequestSchema,
  SearchSessionArtifactsRequestSchema,
  type SessionArtifactHit,
  type SessionArtifactRow,
} from "@/gen/session/v1/session_pb";
import { PullRequestRefSchema } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { BrowseParams, PrTarget } from "./browseQuery";

export const PAGE_SIZE = 50;

export interface BrowsePage {
  rows: SessionArtifactRow[];
  nextPageToken: string;
  totalCount: number;
  unscanned: number;
}

export interface HitList {
  hits: SessionArtifactHit[];
  unscanned: number;
}

/** The three reads the page makes; tests replace the module-level instance. */
export interface ArtifactsApi {
  browse(p: BrowseParams, pageToken: string, signal: AbortSignal): Promise<BrowsePage>;
  search(p: BrowseParams, pr: PrTarget | null, signal: AbortSignal): Promise<HitList>;
  findByPr(pr: PrTarget, signal: AbortSignal): Promise<HitList>;
}

const refOf = (t: PrTarget) => create(PullRequestRefSchema, { host: t.host, owner: t.owner, repo: t.repo, number: t.number });
const client = () => createClient(SessionService, getConnectTransport());

export const artifactsApi: ArtifactsApi = {
  async browse(p, pageToken, signal) {
    const resp = await client().listSessionArtifacts(
      create(ListSessionArtifactsRequestSchema, {
        types: p.type ? [p.type] : [],
        repo: p.repo,
        state: p.state,
        pageToken,
        pageSize: PAGE_SIZE,
      }),
      { signal },
    );
    return { rows: resp.rows, nextPageToken: resp.nextPageToken, totalCount: resp.totalCount, unscanned: resp.unscannedSessionCount };
  },
  async search(p, pr, signal) {
    const resp = await client().searchSessionArtifacts(
      create(SearchSessionArtifactsRequestSchema, { query: p.q, ...(pr ? { pr: refOf(pr) } : {}) }),
      { signal },
    );
    return { hits: resp.hits, unscanned: resp.unscannedSessionCount };
  },
  async findByPr(pr, signal) {
    const resp = await client().findSessionsByPullRequest(
      create(FindSessionsByPullRequestRequestSchema, { pr: refOf(pr) }),
      { signal },
    );
    return { hits: resp.hits, unscanned: resp.unscannedSessionCount };
  },
};
