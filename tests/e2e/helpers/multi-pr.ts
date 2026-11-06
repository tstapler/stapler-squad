import { execFileSync } from "child_process";
import * as fs from "fs";
import * as path from "path";
import type { Page, Route } from "@playwright/test";
import { slotTmp } from "./base-url";

/**
 * Seeding for the session-multi-pr-tracking specs. A real session or backlog item is created through the
 * API; the server-computed PR fields (`pullRequests`, `prHolds`, `artifacts`) are injected into its RPC
 * response by route interception, the documented precedent of create-pull-request.spec.ts and
 * ci-status-badge.spec.ts (the isolated server has no GitHub account and no PR poller to produce them).
 * The `session_multi_pr` flag is read the same way: the client's advisory read of GetFeatureFlags is patched.
 */

const SESSION_RPC = "**/api/session.v1.SessionService";
const BACKLOG_RPC = "**/api/session.v1.BacklogService";
const JSON_CT = "application/json";

export type PrState = "open" | "merged" | "closed" | "unknown";

export interface PrSeed {
  number: number;
  state?: PrState;
  isDraft?: boolean;
  isActive?: boolean;
  pinned?: boolean;
  role?: string;
  owner?: string;
  repo?: string;
  headBranch?: string;
  baseBranch?: string;
  checkConclusion?: string;
  approvedCount?: number;
  /** ISO timestamp */
  mergedAt?: string;
  closedAt?: string;
  createdAt?: string;
  pollError?: string;
  title?: string;
  /** The item itself links this PR (offers "Stop tracking" on the item list). */
  itemDirect?: boolean;
  outcomeUnverified?: boolean;
  verifyAttempts?: number;
  verifyAttemptLimit?: number;
  verifyExhausted?: boolean;
}

/** ConnectRPC JSON shape of one SessionPullRequest. */
export function prJson(seed: PrSeed): Record<string, unknown> {
  const owner = seed.owner ?? "acme";
  const repo = seed.repo ?? "app";
  return {
    host: "github.com",
    owner,
    repo,
    number: seed.number,
    url: `https://github.com/${owner}/${repo}/pull/${seed.number}`,
    state: seed.state ?? "open",
    isDraft: seed.isDraft ?? false,
    role: seed.role ?? "",
    headBranch: seed.headBranch ?? `feat-${seed.number}`,
    baseBranch: seed.baseBranch ?? "main",
    isActive: seed.isActive ?? false,
    pinned: seed.pinned ?? false,
    checkConclusion: seed.checkConclusion ?? "",
    approvedCount: seed.approvedCount ?? 0,
    createdAt: seed.createdAt ?? "2026-11-05T09:00:00Z",
    mergedAt: seed.mergedAt,
    closedAt: seed.closedAt,
    pollError: seed.pollError ?? "POLL_ERROR_NONE",
    title: seed.title ?? "",
    itemDirect: seed.itemDirect ?? false,
    outcomeUnverified: seed.outcomeUnverified ?? false,
    verifyAttempts: seed.verifyAttempts ?? 0,
    verifyAttemptLimit: seed.verifyAttemptLimit ?? 0,
    verifyExhausted: seed.verifyExhausted ?? false,
  };
}

export const HOLD_KINDS = ["KIND_OPEN_PR", "KIND_UNVERIFIED", "KIND_UNKNOWN", "KIND_SUPERSEDED_ROUND", "KIND_SESSION_ACTIVE", "KIND_MAX_HOLD"] as const;
export type HoldKind = (typeof HOLD_KINDS)[number];

export function holdJson(kind: HoldKind, number: number, extra: Record<string, unknown> = {}): Record<string, unknown> {
  return { kind, pr: { host: "github.com", owner: "acme", repo: "app", number }, ...extra };
}

/** Three PRs, one active: the standard "session with several PRs" fixture. */
export function threePrs(): PrSeed[] {
  return [
    { number: 12, isActive: true, checkConclusion: "success", approvedCount: 1 },
    { number: 11, state: "merged", mergedAt: "2026-11-05T10:00:00Z" },
    { number: 10, state: "merged", mergedAt: "2026-11-04T10:00:00Z" },
  ];
}

async function patchJson(route: Route, patch: (body: Record<string, unknown>) => void): Promise<void> {
  const response = await route.fetch();
  const body = (await response.json()) as Record<string, unknown>;
  patch(body);
  await route.fulfill({ response, json: body });
}

export interface SessionSeed {
  sessionId: string;
  pullRequests?: Record<string, unknown>[];
  prHolds?: Record<string, unknown>[];
  artifacts?: Record<string, unknown>;
  /** Other top-level Session fields to set (e.g. legacy githubPrUrl). */
  fields?: Record<string, unknown>;
}

/**
 * Injects server-computed fields into the given sessions in ListSessions/GetSession (one route for all, since
 * Playwright runs only the latest-registered matching route). Blocks WatchSessions so no real event overwrites them.
 */
export async function seedSessionFields(page: Page, seeds: SessionSeed | SessionSeed[]): Promise<void> {
  const list = Array.isArray(seeds) ? seeds : [seeds];
  const apply = (s: Record<string, unknown> | undefined) => {
    const seed = list.find((x) => x.sessionId === s?.id);
    if (!s || !seed) return;
    if (seed.pullRequests) s.pullRequests = seed.pullRequests;
    if (seed.prHolds) s.prHolds = seed.prHolds;
    if (seed.artifacts) s.artifacts = seed.artifacts;
    Object.assign(s, seed.fields ?? {});
  };
  await page.route(`${SESSION_RPC}/ListSessions`, (route) =>
    patchJson(route, (body) => ((body.sessions ?? []) as Record<string, unknown>[]).forEach(apply)),
  );
  await page.route(`${SESSION_RPC}/GetSession`, (route) =>
    patchJson(route, (body) => apply(body.session as Record<string, unknown> | undefined)),
  );
  await page.route(`${SESSION_RPC}/WatchSessions`, (route) => route.abort());
}

/** Seeds `pullRequests`/`prHolds` on one backlog item's GetBacklogItem (detail responses only, like the server). */
export async function seedBacklogItemPrs(page: Page, itemId: string, seed: { pullRequests: Record<string, unknown>[]; prHolds?: Record<string, unknown>[] }): Promise<void> {
  await page.route(`${BACKLOG_RPC}/GetBacklogItem`, (route) =>
    patchJson(route, (body) => {
      const item = body.item as Record<string, unknown> | undefined;
      if (!item || (item.id !== itemId && item.publicId !== itemId)) return;
      item.pullRequests = seed.pullRequests;
      item.prHolds = seed.prHolds ?? [];
    }),
  );
}

const READER_SCOPES = ["reader:poller", "reader:backlog", "reader:actions", "reader:ui"];

/** Makes the client read `session_multi_pr` as on (every reader scope) or off. */
export async function setMultiPrFlag(page: Page, on: boolean): Promise<void> {
  await page.route(`${SESSION_RPC}/GetFeatureFlags`, (route) =>
    patchJson(route, (body) => {
      const flags = ((body.flags ?? []) as Record<string, unknown>[]).filter((f) => f.name !== "session_multi_pr");
      flags.push({ name: "session_multi_pr", enabled: false, description: "", statusDetail: "", scopes: READER_SCOPES.map((scope) => ({ scope, enabled: on })) });
      body.flags = flags;
      delete body.notModified;
    }),
  );
}

/** Answers one SessionService PR RPC with a fixed body; records the request bodies it saw. */
export async function mockPrRpc(page: Page, method: string, response: Record<string, unknown>): Promise<Record<string, unknown>[]> {
  const seen: Record<string, unknown>[] = [];
  await page.route(`${SESSION_RPC}/${method}`, async (route) => {
    seen.push(route.request().postDataJSON() as Record<string, unknown>);
    await route.fulfill({ status: 200, contentType: JSON_CT, body: JSON.stringify(response) });
  });
  return seen;
}

/** Console errors/page errors collected for the "no console errors" assertions. */
export function collectConsoleErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() === "error" && !/favicon|Failed to load resource|SIZING MISMATCH|Failed to fetch|ResizeObserver loop/i.test(m.text())) errors.push(`console: ${m.text()}`);
  });
  return errors;
}

export async function skipOnboarding(page: Page): Promise<void> {
  await page.addInitScript(() => {
    try {
      localStorage.setItem("stapler-squad:onboarded", "true");
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    } catch { /* storage blocked: the dialog may show */ }
  });
}

export interface UserPrSeed {
  owner: string;
  repo: string;
  number: number;
  title: string;
}

// Minimal proto3 encoder for UserPREvent (proto/session/v1/github_user.proto, types.proto UserPR). The e2e job
// installs only tests/e2e's dependencies and generates no protobuf code, so the specs cannot import web-app's.
const textEncoder = new TextEncoder();
const varint = (n: number): number[] => {
  const out: number[] = [];
  let v = n;
  while (v > 0x7f) {
    out.push((v & 0x7f) | 0x80);
    v = Math.floor(v / 128);
  }
  out.push(v);
  return out;
};
const fieldString = (field: number, value: string): number[] =>
  value === "" ? [] : [...varint((field << 3) | 2), ...varint(textEncoder.encode(value).length), ...textEncoder.encode(value)];
const fieldVarint = (field: number, value: number): number[] => (value === 0 ? [] : [...varint(field << 3), ...varint(value)]);
const fieldMessage = (field: number, body: number[]): number[] => [...varint((field << 3) | 2), ...varint(body.length), ...body];

function encodeUserPREvent(prs: UserPrSeed[]): Uint8Array {
  const authState = [...fieldVarint(1, 1), ...fieldString(2, "e2e-user")];
  const body = [
    ...fieldString(1, "snapshot"),
    ...prs.flatMap((p) =>
      fieldMessage(
        2,
        [
          ...fieldString(1, p.owner),
          ...fieldString(2, p.repo),
          ...fieldVarint(3, p.number),
          ...fieldString(4, p.title),
          ...fieldString(5, `https://github.com/${p.owner}/${p.repo}/pull/${p.number}`),
          ...fieldString(6, "OPEN"),
          ...fieldString(7, `feat-${p.number}`),
          ...fieldString(8, "main"),
          ...fieldString(10, "success"),
          ...fieldVarint(21, 1),
          ...fieldString(23, "github.com"),
          ...fieldString(24, "e2e-user"),
        ],
      ),
    ),
    ...fieldMessage(3, authState),
  ];
  return Uint8Array.from(body);
}


/**
 * Serves the Up Next PRs panel's WatchUserPRs WebSocket stream with one snapshot, as one connected account.
 * The isolated server has no GitHub account, so without this the panel only shows "not connected".
 * Frame: a Connect envelope (flag 0, 4-byte length, binary UserPREvent) per watch-ws-transport.ts.
 */
export async function seedUserPrs(page: Page, prs: UserPrSeed[]): Promise<void> {
  const payload = encodeUserPREvent(prs);
  const frame = new Uint8Array(5 + payload.length);
  new DataView(frame.buffer).setUint32(1, payload.length, false);
  frame.set(payload, 5);
  await page.routeWebSocket(/WatchUserPRs/, (ws) => {
    ws.onMessage(() => ws.send(Buffer.from(frame)));
  });
}

export type ScanState = "not_linked" | "scanning" | "none_found" | "has_data" | "failed";

export interface ArtifactRecordSeed {
  /** ISO timestamp the transcript line was written. */
  at: string;
  record: Record<string, unknown>;
}

const rec = (kind: string, at: string, body: Record<string, unknown>) => ({ kind, firstSeenAt: at, ...body });
export const prCreated = (at: string, number: number, source = "created"): ArtifactRecordSeed["record"] =>
  rec("pr", at, { pr: { host: "github.com", owner: "acme", repo: "app", number, source, branch: `feat-${number}`, base: "main" } });
export const commitRecord = (at: string, sha: string, subject: string, branch = "feat-b") => rec("commit", at, { commit: { sha, subject, branch } });
export const planRecord = (at: string, path: string) => rec("plan_doc", at, { planDoc: { path } });
export const ciRecord = (at: string, url: string) => rec("ci_link", at, { ciLink: { url } });

/** ConnectRPC JSON of a SessionArtifacts message. */
export function artifactsJson(state: ScanState, records: Record<string, unknown>[] = [], extra: Record<string, unknown> = {}): Record<string, unknown> {
  return { scanState: state, transcriptSupported: true, lastScannedAt: "2026-11-05T11:55:00Z", prUrls: [], commitShas: [], externalUrls: [], records, ...extra };
}

/** Two created PRs, a commit and plan doc each side of their create times, an unrelated link and one mentioned PR. */
export function groupedRecords(): Record<string, unknown>[] {
  return [
    commitRecord("2026-11-05T09:00:00Z", "aaaaaaa1111111111111111111111111111111aa", "chore: early work"),
    prCreated("2026-11-05T10:00:00Z", 11),
    commitRecord("2026-11-05T10:30:00Z", "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", "fix: token refresh"),
    prCreated("2026-11-05T11:00:00Z", 12),
    planRecord("2026-11-05T11:15:00Z", "project_plans/x/plan.md"),
    prCreated("2026-11-05T11:20:00Z", 40, "mentioned"),
  ];
}

/** A throwaway git repo with one commit, so a session on it has a real VCS tab (a plain /tmp session shows "not a version-controlled directory"). */
export function createGitRepo(name: string): string {
  const dir = path.join(slotTmp("multipr-repos"), `${name}-${Date.now()}`);
  fs.mkdirSync(dir, { recursive: true });
  const git = (...args: string[]) => execFileSync("git", args, { cwd: dir, stdio: "ignore", env: { ...process.env, GIT_AUTHOR_NAME: "e2e", GIT_AUTHOR_EMAIL: "e2e@example.com", GIT_COMMITTER_NAME: "e2e", GIT_COMMITTER_EMAIL: "e2e@example.com" } });
  git("init", "-b", "main");
  git("commit", "--allow-empty", "-m", "init");
  return dir;
}
