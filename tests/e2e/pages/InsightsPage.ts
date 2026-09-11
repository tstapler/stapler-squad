import { Page, Locator } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Page object for the /insights dashboard (project_plans/insights-cost-intelligence),
 * covering the Findings panel (design/ux.md B1) and the Sessions table's
 * extended sort/search (design/ux.md B2). Session drill-down (B3) navigation
 * helpers that operate on the modal/route content itself live on
 * `SessionDetailPage` (reused across surfaces per that page object's existing
 * convention) — this class only covers getting *to* that content from the
 * dashboard (mocking the summary RPC, clicking a session row).
 *
 * `GetInsightsSummary` is mocked via `page.route` rather than seeding real
 * JSONL transcripts — mirroring `DeepLinkResolveMock.ts`'s pattern — because
 * deterministic waste-finding/waste-score/cache-ROI fixtures are far cheaper
 * to construct as canonical protobuf JSON than as parseable JSONL history
 * that would need to survive the real token-parsing + heuristic pipeline.
 * `useSessionDetail` (session-detail route/modal) calls the *same* RPC
 * method, so one mock installed before navigating to `/insights` also serves
 * a subsequent navigation to `/insights/session-detail?sessionId=...`.
 */
export class InsightsPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  async goto() {
    // Skip the first-run onboarding tour modal (useOnboarding.ts's
    // `stapler-squad:onboarded` key) — a fresh test-mode instance's first
    // navigation would otherwise show it, and it steals focus/intercepts
    // clicks from everything underneath, matching the pattern already used
    // by e.g. accessibility.spec.ts and backlog-*.spec.ts.
    await this.page.addInitScript(() => {
      try {
        window.localStorage.setItem("stapler-squad:onboarded", "true");
      } catch {
        /* ignore */
      }
    });
    await this.page.goto(`${BASE_URL}/insights`, { waitUntil: "domcontentloaded" });
  }

  /**
   * Installs a route handler for the InsightsService.GetInsightsSummary
   * Connect RPC (POST .../api/session.v1.InsightsService/GetInsightsSummary,
   * JSON protocol — the transport's default, see useInsightsService.ts).
   * `responses` is consumed in order, one per request; the last response
   * repeats once exhausted, so a two-entry list (fail, then succeed) is
   * enough to cover a Retry click. Must be called before navigation/action
   * that triggers the request (Playwright routes only affect requests issued
   * after registration).
   */
  async mockGetInsightsSummary(responses: MockInsightsSummaryResponse[]) {
    let callIndex = 0;
    await this.page.route("**/session.v1.InsightsService/GetInsightsSummary", async (route) => {
      const cfg = responses[Math.min(callIndex, responses.length - 1)];
      callIndex += 1;
      if (cfg.delayMs) {
        await new Promise((resolve) => setTimeout(resolve, cfg.delayMs));
      }
      if (cfg.errorMessage) {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          // Connect Unary error shape: { code, message }. Deliberately avoid
          // "internal"/"unavailable"/code numbers 13/14/16 in the message —
          // InsightsDashboard.tsx's friendlyError() rewrites those substrings
          // to a generic string, which would make this mock's message
          // unassertable.
          body: JSON.stringify({ code: "unknown", message: cfg.errorMessage }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(buildInsightsSummaryResponse(cfg.body ?? {})),
      });
    });
  }

  // ---------------------------------------------------------------------
  // Findings panel (design/ux.md B1)
  // ---------------------------------------------------------------------

  getFindingsPanel(): Locator {
    return this.page.getByTestId("findings-panel");
  }

  getFindingsSkeleton(): Locator {
    return this.page.getByTestId("findings-skeleton");
  }

  getFindingCards(): Locator {
    return this.getFindingsPanel().getByRole("listitem");
  }

  getFindingViewSessionLink(nameHint?: string | RegExp): Locator {
    return this.getFindingsPanel().getByRole("link", { name: nameHint ?? /View session/i });
  }

  getFindingsRetryButton(): Locator {
    return this.getFindingsPanel().getByRole("button", { name: "Retry" });
  }

  // ---------------------------------------------------------------------
  // Sessions table (design/ux.md B2)
  // ---------------------------------------------------------------------

  getSessionsSearchInput(): Locator {
    return this.page.getByLabel("Search sessions by project path");
  }

  /** The `<th aria-sort=...>` cell itself — use for `aria-sort` assertions. */
  getColumnHeader(name: string | RegExp): Locator {
    return this.page.getByRole("columnheader", { name });
  }

  /** The clickable/focusable `role="button"` control inside a sortable header cell — use to trigger a sort. */
  getSortableColumnControl(name: string | RegExp): Locator {
    return this.page.getByRole("button", { name });
  }

  /** A session row (rendered as `role="button"` when `onSessionClick` is set — see SessionsTable.tsx). */
  getSessionRow(nameHint: string | RegExp): Locator {
    return this.page.getByRole("button", { name: nameHint });
  }

  /**
   * Data rows only. Structural (`tbody tr`), not `getByRole("row")` — a
   * clickable row's `role` is overridden to "button" (see SessionsTable.tsx's
   * `role={onSessionClick ? "button" : undefined}`), so only the header row
   * still reports role="row" and an ARIA-role locator can never see body rows.
   */
  getSessionsTableRows(): Locator {
    return this.page.locator("table tbody tr");
  }
}

export interface MockInsightsSummaryResponse {
  /** Partial GetInsightsSummaryResponse fields — merged onto sensible defaults, see buildInsightsSummaryResponse. */
  body?: Record<string, unknown>;
  /** When set, the route fails the request instead of returning `body`. */
  errorMessage?: string;
  /** Artificial delay (ms) before fulfilling — used to observe the loading/skeleton state. */
  delayMs?: number;
}

/** Builds a full canonical-JSON GetInsightsSummaryResponse, defaults merged with `overrides`. */
export function buildInsightsSummaryResponse(overrides: Record<string, unknown> = {}) {
  return {
    sessions: [],
    totalCostUsd: 0,
    totalInputTokens: "0",
    totalOutputTokens: "0",
    totalCacheReadTokens: "0",
    overallCacheHitRate: 0,
    daily: [],
    models: [],
    topSkills: [],
    topTools: [],
    isLoading: false,
    unpricedModels: [],
    findings: [],
    activityBreakdown: [],
    ...overrides,
  };
}

/** Builds one WasteFinding as canonical JSON (enum values as their proto string names). */
export function buildFinding(overrides: Record<string, unknown> = {}) {
  return {
    findingType: "FINDING_TYPE_CACHE_HIT_FLOOR_BREACH",
    severity: "SEVERITY_CRITICAL",
    dollarImpactUsd: 4.2,
    sessionId: "session-alpha",
    conversationId: "conv-alpha",
    message: "Cache-hit floor breach: 9% hit rate vs. 40% floor",
    ...overrides,
  };
}

/** Builds one TopToolEntry as canonical JSON. */
export function buildTopTool(overrides: Record<string, unknown> = {}) {
  return {
    toolName: "Read",
    callCount: 3,
    mcpServer: "",
    costUsd: 0.05,
    costMayDoubleCount: false,
    costUnpriced: false,
    ...overrides,
  };
}

/** Builds one SessionTokenSummary as canonical JSON (int64 fields as strings, per proto3 JSON mapping). */
export function buildSession(overrides: Record<string, unknown> = {}) {
  return {
    sessionId: "session-alpha",
    conversationId: "conv-alpha",
    projectPath: "/home/user/repo-alpha",
    primaryModel: "claude-sonnet-4",
    totalInputTokens: "1200000",
    totalOutputTokens: "340000",
    cacheCreationTokens: "50000",
    cacheReadTokens: "900000",
    estimatedCostUsd: 4.1,
    cacheHitRate: 0.42,
    messageCount: 12,
    firstMessageAt: "2026-08-01T00:00:00Z",
    lastMessageAt: "2026-08-01T00:12:04Z",
    isOrphan: false,
    skillActivations: [],
    topTools: [],
    unpricedModels: [],
    cacheRoiUsd: 0.82,
    wasteScore: 22,
    activityType: "ACTIVITY_TYPE_UNSPECIFIED",
    ...overrides,
  };
}

/**
 * Tabs from the currently-focused element in a loop, asserting focus never
 * leaves `dialog`'s DOM subtree, until it wraps back to the element that was
 * focused when this was called (or `maxPresses` is exhausted, in which case
 * it fails) — mirrors accessibility.spec.ts's `assertTabWrapsWithinDialog`
 * helper for the same modal-focus-trap shape (ReviewChangesModal there,
 * SessionDetailDrawer here).
 */
export async function assertTabWrapsWithinDialog(page: Page, dialog: Locator, maxPresses = 30) {
  const initial = await page.evaluateHandle(() => document.activeElement);
  try {
    let wrapped = false;
    for (let i = 0; i < maxPresses; i++) {
      await page.keyboard.press("Tab");
      const stillInside = await dialog.evaluate(
        (el) => !!document.activeElement && el.contains(document.activeElement)
      );
      if (!stillInside) {
        throw new Error(`Tab press #${i + 1} moved focus outside the dialog`);
      }
      if (await page.evaluate((el) => el === document.activeElement, initial)) {
        wrapped = true;
        break;
      }
    }
    if (!wrapped) {
      throw new Error("Tab never wrapped back to the dialog's first focusable element");
    }
  } finally {
    await initial.dispose();
  }
}
