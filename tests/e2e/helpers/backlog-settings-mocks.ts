/**
 * Connect (JSON) `page.route` fulfilments for the backlog-settings specs: states the real
 * isolated server cannot produce on demand (status per caller class, a failing or pending RPC,
 * readiness-evidence variants, a quota read-back that differs, a stuck report per auto-merge
 * lookup status, a flag list that changes "in another tab").
 *
 * Every mock is a mutable controller: register the route once before navigation, then change
 * what it answers with `use(...)`. A reply may be a deferred promise, so a test resolves a
 * pending request explicitly instead of sleeping. Route pattern is
 * `**\/session.v1.<Service>/<Method>` (see pages/InsightsPage.ts).
 */
import { expect as baseExpect, type APIRequestContext, type Page, type Route } from "@playwright/test";

/** `expect` with a timeout that tolerates a loaded machine: page loads under load outran the 5 s default. */
export const expect = baseExpect.configure({ timeout: 15_000 });

export const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

export type Reply =
  | { json: unknown; status?: number }
  | { error: { code: string; message: string }; status?: number };

export type Responder = Reply | ((callIndex: number, body: unknown) => Reply | Promise<Reply>);

const ERROR_STATUS: Record<string, number> = {
  failed_precondition: 400,
  invalid_argument: 400,
  not_found: 404,
  unavailable: 503,
  internal: 500,
  unknown: 500,
};

/** A Connect unary error body (`{code, message}`) with the matching HTTP status. */
export function connectError(code: string, message: string): Reply {
  return { error: { code, message }, status: ERROR_STATUS[code] ?? 500 };
}

export function ok(json: unknown): Reply {
  return { json, status: 200 };
}

export interface Deferred {
  /** Pass to `use(() => d.promise)`. */
  promise: Promise<Reply>;
  resolve: (reply: Reply) => void;
}

/** A reply the test resolves explicitly (pending-state assertions without sleeps). */
export function deferred(): Deferred {
  let resolve!: (reply: Reply) => void;
  const promise = new Promise<Reply>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

/** Answers call N with replies[N]; the last reply repeats once the list is exhausted. */
export function sequence(...replies: Reply[]): Responder {
  return (n) => replies[Math.min(n, replies.length - 1)];
}

export class ProcedureMock {
  /** Requests the page has issued to this procedure (counted when the route handler runs). */
  readonly requests: unknown[] = [];
  private responder: Responder;
  /** Calls answered before the last `use()`; sequences index from the last `use()`. */
  private base = 0;

  constructor(initial: Responder) {
    this.responder = initial;
  }

  get calls(): number {
    return this.requests.length;
  }

  /** Replace the answer from now on; a sequence starts again from its first reply. */
  use(responder: Responder): void {
    this.responder = responder;
    this.base = this.requests.length;
  }

  async handle(route: Route): Promise<void> {
    const raw = route.request().postData();
    let body: unknown = null;
    try {
      body = raw ? JSON.parse(raw) : null;
    } catch {
      body = raw;
    }
    const index = this.requests.length - this.base;
    this.requests.push(body);
    const responder = this.responder;
    const reply = typeof responder === "function" ? await responder(index, body) : responder;
    if ("error" in reply) {
      await route.fulfill({
        status: reply.status ?? 500,
        contentType: "application/json",
        body: JSON.stringify(reply.error),
      });
      return;
    }
    await route.fulfill({
      status: reply.status ?? 200,
      contentType: "application/json",
      body: JSON.stringify(reply.json),
    });
  }
}

/** Registers `**\/session.v1.<service>/<method>` and returns its controller. */
export async function mockProcedure(
  page: Page,
  service: string,
  method: string,
  initial: Responder,
): Promise<ProcedureMock> {
  const mock = new ProcedureMock(initial);
  await page.route(`**/session.v1.${service}/${method}`, (route) => mock.handle(route));
  return mock;
}

// ---------------------------------------------------------------------------
// BacklogService.GetWorkflowEngineStatus
// ---------------------------------------------------------------------------

export type CallerClass = "operator" | "agent" | "system";
export const CALLER_CLASSES: readonly CallerClass[] = ["operator", "agent", "system"];
export type ModeName = "OFF" | "SHADOW" | "ENFORCE";
export type Verdict = "READY" | "NOT_READY" | "NEEDS_OWNER_ACK" | "UNAVAILABLE";
export type ReviewState = "UNAVAILABLE" | "NOT_DUE" | "OVERDUE" | "ENFORCED";

export interface Readiness {
  divergence24h: number;
  verdict: Verdict;
  strandedItemCount: number;
}

export const CLEAN: Readiness = { divergence24h: 0, verdict: "READY", strandedItemCount: 0 };
export const DIFFERENCES: Readiness = { divergence24h: 3, verdict: "READY", strandedItemCount: 0 };

export interface EngineStatusSpec {
  modes: Record<CallerClass, ModeName>;
  gateWired?: boolean;
  invalidatorWired?: boolean;
  fallbackActive?: boolean;
  divergence24h?: number;
  /** Per-class evidence; a class left out is reported CLEAN. */
  readiness?: Partial<Record<CallerClass, Readiness>>;
  /** ISO timestamp + state; omitted means the server reports no review date. */
  review?: { dueAt: string; state: ReviewState };
}

export function allModes(mode: ModeName): Record<CallerClass, ModeName> {
  return { operator: mode, agent: mode, system: mode };
}

/** JSON body of a GetWorkflowEngineStatusResponse. */
export function engineStatusJson(spec: EngineStatusSpec): unknown {
  const readiness = Object.fromEntries(
    CALLER_CLASSES.map((c) => {
      const r = spec.readiness?.[c] ?? CLEAN;
      return [
        c,
        {
          divergence24h: String(r.divergence24h),
          soakVerdict: `WORKFLOW_ENGINE_SOAK_VERDICT_${r.verdict}`,
          strandedItemCount: String(r.strandedItemCount),
        },
      ];
    }),
  );
  return {
    classModes: Object.fromEntries(CALLER_CLASSES.map((c) => [c, `WORKFLOW_ENGINE_MODE_${spec.modes[c]}`])),
    divergence24h: String(spec.divergence24h ?? 0),
    invalidatorWired: spec.invalidatorWired ?? true,
    gateWired: spec.gateWired ?? true,
    fallbackActive: spec.fallbackActive ?? false,
    classReadiness: readiness,
    ...(spec.review
      ? {
          reviewDueAt: spec.review.dueAt,
          reviewOverdue: spec.review.state === "OVERDUE",
          reviewState: `WORKFLOW_ENGINE_REVIEW_STATE_${spec.review.state}`,
        }
      : { reviewState: "WORKFLOW_ENGINE_REVIEW_STATE_UNAVAILABLE" }),
  };
}

export function engineStatus(spec: EngineStatusSpec): Reply {
  return ok(engineStatusJson(spec));
}

export function mockEngineStatus(page: Page, initial: Responder): Promise<ProcedureMock> {
  return mockProcedure(page, "BacklogService", "GetWorkflowEngineStatus", initial);
}

// ---------------------------------------------------------------------------
// SessionService.GetFeatureFlags / UpdateFeatureFlag (an in-memory flag store)
// ---------------------------------------------------------------------------

export const ENFORCE_FLAGS: Record<CallerClass, string> = {
  operator: "backlog:workflow-engine-enforce",
  agent: "backlog:workflow-engine-enforce-agent",
  system: "backlog:workflow-engine-enforce-system",
};
export const SHADOW_FLAG = "backlog:workflow-engine-shadow";

export interface FlagRow {
  name: string;
  enabled: boolean;
  description: string;
  statusDetail: string;
}

export function defaultFlagRows(): FlagRow[] {
  return [
    { name: "backlog", enabled: true, description: "Enable the backlog.", statusDetail: "" },
    {
      name: "diagnose_nudge_enabled",
      enabled: false,
      description: "Dispatch a diagnostic agent for stuck items.",
      statusDetail: "Default: Off (not set explicitly)",
    },
    {
      name: "autonomous_stall_nudge_enabled",
      enabled: false,
      description: "Nudge stalled autonomous sessions.",
      statusDetail: "Default: Off (not set explicitly)",
    },
    {
      name: SHADOW_FLAG,
      enabled: true,
      description: "Compare stage edits against the built-in rules without enforcing them.",
      statusDetail: "Default: On",
    },
    {
      name: ENFORCE_FLAGS.operator,
      enabled: false,
      description: "Apply stage edits to operator transitions.",
      statusDetail: "Default: Off (not set explicitly)",
    },
    {
      name: ENFORCE_FLAGS.agent,
      enabled: false,
      description: "Apply stage edits to agent transitions.",
      statusDetail: "Default: Off (not set explicitly)",
    },
    {
      name: ENFORCE_FLAGS.system,
      enabled: false,
      description: "Apply stage edits to automation transitions.",
      statusDetail: "Default: Off (not set explicitly)",
    },
  ];
}

/**
 * An in-memory feature-flag server. `list` answers GetFeatureFlags from `rows`; `update` applies
 * a boolean UpdateFeatureFlag to `rows` and echoes the stored row (the read-back the page shows).
 * Replace `update`'s responder to fail or hold a write; `rows` is what "another tab" changes.
 */
export class FlagStore {
  rows: FlagRow[];
  list!: ProcedureMock;
  update!: ProcedureMock;

  constructor(rows: FlagRow[] = defaultFlagRows()) {
    this.rows = rows;
  }

  static async install(page: Page, rows?: FlagRow[]): Promise<FlagStore> {
    const store = new FlagStore(rows);
    store.list = await mockProcedure(page, "SessionService", "GetFeatureFlags", () => ok({ flags: store.rows }));
    store.update = await mockProcedure(page, "SessionService", "UpdateFeatureFlag", (_n, body) =>
      store.applyUpdate(body),
    );
    return store;
  }

  get(name: string): FlagRow {
    const row = this.rows.find((r) => r.name === name);
    if (!row) throw new Error(`flag ${name} is not in the mock store`);
    return row;
  }

  set(name: string, enabled: boolean): void {
    this.get(name).enabled = enabled;
  }

  /** The default update behavior; reusable from a custom responder that wants to delay then apply. */
  applyUpdate(body: unknown): Reply {
    const req = body as { name: string; enabled?: boolean };
    const row = this.get(req.name);
    row.enabled = Boolean(req.enabled);
    return ok({ flag: row });
  }

  /** Engine modes the real server would report for the stored flags. */
  modes(): Record<CallerClass, ModeName> {
    const shadow = this.get(SHADOW_FLAG).enabled;
    return Object.fromEntries(
      CALLER_CLASSES.map((c) => [c, this.get(ENFORCE_FLAGS[c]).enabled ? "ENFORCE" : shadow ? "SHADOW" : "OFF"]),
    ) as Record<CallerClass, ModeName>;
  }
}

/** A status responder derived from the flag store, so the banner follows the Features page. */
export function statusFollowing(store: FlagStore, extra: Omit<EngineStatusSpec, "modes"> = {}): Responder {
  return () => engineStatus({ modes: store.modes(), ...extra });
}

// ---------------------------------------------------------------------------
// QuotaService
// ---------------------------------------------------------------------------

export interface QuotaValues {
  enabled: boolean;
  /** Tokens, as a decimal string (int64 on the wire). */
  budget: string;
  pause: number;
  resume: number;
  /** USD; undefined means the soft budget is off. */
  usd?: number;
}

export const QUOTA_DEFAULTS: QuotaValues = { enabled: false, budget: "0", pause: 20, resume: 15 };

export function quotaJson(values: QuotaValues, source: "NOT_CONFIGURED" | "CONFIGURED" | "UNSPECIFIED" = "CONFIGURED") {
  return {
    settings: {
      enabled: values.enabled,
      pauseBelowHeadroomPct: values.pause,
      resumeMarginPct: values.resume,
      assumedWindowTokenBudget: values.budget,
      ...(values.usd === undefined ? {} : { defaultCostBudgetThresholdUsd: values.usd }),
    },
    quotaSource: `QUOTA_SOURCE_${source}`,
  };
}

/** An UpdateQuotaConfig that stores exactly what was sent (the read-back equals the entry). */
export function echoQuotaUpdate(): Responder {
  return (_n, body) =>
    ok({ settings: (body as { settings: Record<string, unknown> }).settings, quotaSource: "QUOTA_SOURCE_CONFIGURED" });
}

/** An UpdateQuotaConfig that stores the sent values except for the given overrides (a differing read-back). */
export function quotaUpdateStoring(overrides: Record<string, unknown>): Responder {
  return (_n, body) =>
    ok({
      settings: { ...(body as { settings: Record<string, unknown> }).settings, ...overrides },
      quotaSource: "QUOTA_SOURCE_CONFIGURED",
    });
}

export function mockQuotaGet(page: Page, initial: Responder): Promise<ProcedureMock> {
  return mockProcedure(page, "QuotaService", "GetQuotaConfig", initial);
}

export function mockQuotaUpdate(page: Page, initial: Responder): Promise<ProcedureMock> {
  return mockProcedure(page, "QuotaService", "UpdateQuotaConfig", initial);
}

// ---------------------------------------------------------------------------
// BacklogService.ListStuckBacklogItems
// ---------------------------------------------------------------------------

export type LookupStatus = "UNSPECIFIED" | "OK" | "RATE_LIMITED" | "NO_PERMISSION" | "NOT_FOUND" | "ERROR";

export interface StuckItemSpec {
  itemId?: string;
  title?: string;
  prUrl?: string;
  lookup: LookupStatus;
  /** Meaningful only for OK. */
  allowAutoMerge?: boolean;
  /** ISO timestamp; omit for "no auto_merge_checked_at". */
  checkedAt?: string;
}

export const STUCK_ITEM_TITLE = "fix: auto-merge line";

export function stuckItemJson(spec: StuckItemSpec): unknown {
  const now = new Date().toISOString();
  return {
    itemId: spec.itemId ?? "e2e-automerge-1",
    title: spec.title ?? STUCK_ITEM_TITLE,
    status: "pr_pending",
    reason: "STUCK_REASON_PR_READY_UNMERGED",
    firstDetectedAt: now,
    lastCheckedAt: now,
    prNumber: 148,
    prUrl: spec.prUrl ?? "https://github.com/tstapler/stapler-squad/pull/148",
    context: "PR is green and mergeable but not merged",
    autoMergeLookupStatus: `AUTO_MERGE_LOOKUP_STATUS_${spec.lookup}`,
    ...(spec.lookup === "OK" && spec.allowAutoMerge !== undefined ? { allowAutoMerge: spec.allowAutoMerge } : {}),
    ...(spec.checkedAt ? { autoMergeCheckedAt: spec.checkedAt } : {}),
  };
}

export function stuckReport(...items: StuckItemSpec[]): Reply {
  return ok({ items: items.map(stuckItemJson) });
}

export function mockStuckList(page: Page, initial: Responder): Promise<ProcedureMock> {
  return mockProcedure(page, "BacklogService", "ListStuckBacklogItems", initial);
}

// ---------------------------------------------------------------------------
// Real-server setup shared by the specs
// ---------------------------------------------------------------------------

/** Keeps the onboarding tours from covering the page under test. */
export async function skipOnboarding(page: Page): Promise<void> {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
    localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    // Quiet mode sends toasts straight to the tray so the seeded sessions cannot cover the page under test.
    localStorage.setItem("ssq.notifications.quietMode", "true");
  });
}

/** Writes a flag on the isolated e2e server through the same RPC the Features page uses. */
export async function setFlagViaApi(request: APIRequestContext, name: string, enabled: boolean): Promise<void> {
  const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name, enabled },
  });
  if (!resp.ok()) throw new Error(`UpdateFeatureFlag ${name}=${enabled} failed (${resp.status()}): ${await resp.text()}`);
}

/** Puts the four engine flags back to their defaults (shadow on, every enforce flag off). */
export async function resetEngineFlags(request: APIRequestContext): Promise<void> {
  await setFlagViaApi(request, SHADOW_FLAG, true);
  for (const flag of Object.values(ENFORCE_FLAGS)) await setFlagViaApi(request, flag, false);
}

/** Moves the page clock past the 5 s focus/visibility refetch window (needs `page.clock.install()` first). */
export async function pastRefetchWindow(page: Page): Promise<void> {
  await page.clock.fastForward(6000);
}

/**
 * A real soft keyboard gives the page scroll room beneath its content; the simulated one (`setKeyboardOpen`)
 * only shrinks `visualViewport`, so a short page cannot scroll a low field far enough. Add that room.
 */
export async function addKeyboardScrollRoom(page: Page, keyboardPx: number): Promise<void> {
  await page.evaluate((px) => {
    document.body.style.paddingBottom = `${px * 2}px`;
  }, keyboardPx);
}

/** Waits until React has hydrated the element with this test id (the static export prerenders its loading state). */
export async function whenHydrated(page: Page, testId: string): Promise<void> {
  await page.waitForFunction((id) => {
    const el = document.querySelector(`[data-testid="${id}"]`);
    return Boolean(el) && Object.keys(el as object).some((key) => key.startsWith("__reactFiber"));
  }, testId);
  // Effects (listeners) run just after the hydration commit: let two frames pass.
  await page.evaluate(() => new Promise<void>((done) => requestAnimationFrame(() => requestAnimationFrame(() => done()))));
}

/** Fires the event the pages listen to for "this tab was looked at again". */
export async function wakeTab(page: Page): Promise<void> {
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")));
}
