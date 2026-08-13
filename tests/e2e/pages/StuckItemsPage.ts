import { APIRequestContext, Page, Locator } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

/**
 * Page object for the "Stuck Backlog Items" section on /unfinished
 * (backlog-stuck-item-visibility, Epic 4.1).
 */
export class StuckItemsPage {
  readonly page: Page;
  readonly section: Locator;
  readonly countRegion: Locator;
  readonly filterGroup: Locator;
  readonly emptyState: Locator;
  readonly filteredEmptyState: Locator;
  readonly clearFilterButton: Locator;
  readonly staleBanner: Locator;
  readonly retryButton: Locator;

  constructor(page: Page) {
    this.page = page;
    this.section = page.getByTestId("stuck-items-section");
    this.countRegion = page.getByTestId("stuck-items-count");
    this.filterGroup = page.getByRole("group", { name: "Filter stuck items by reason" });
    this.emptyState = page.getByTestId("stuck-items-empty");
    this.filteredEmptyState = page.getByTestId("stuck-items-filtered-empty");
    this.clearFilterButton = page.getByTestId("stuck-items-clear-filter");
    this.staleBanner = page.getByTestId("stuck-items-stale-banner");
    this.retryButton = page.getByTestId("stuck-items-retry");
  }

  async goto() {
    await this.page.goto(`${BASE_URL}/unfinished`, { waitUntil: "domcontentloaded" });
    await this.page.waitForSelector('[data-testid="stuck-items-section"]', { timeout: 15000 });
  }

  filterChip(value: string): Locator {
    return this.page.getByTestId(`stuck-filter-chip-${value}`);
  }

  card(itemId: string): Locator {
    return this.page.locator(`[data-testid="stuck-item"][data-reason]`).filter({
      has: this.page.locator(`text=${itemId}`),
    });
  }

  cardByTitle(title: string): Locator {
    return this.page.getByTestId("stuck-item").filter({ hasText: title });
  }

  navBadge(): Locator {
    return this.page.getByTestId("stuck-nav-badge");
  }
}

/**
 * Seeds an open BacklogStuckState row for a backlog item, bypassing the
 * reconciler/detectors entirely (validation.md's own note for the UX
 * Acceptance Tests: "stuck data seeded through the backlog store").
 *
 * Backed by the `/api/debug/backlog/seed-stuck` handler
 * (server/services/backlog_debug_seed_handler.go), which creates a real
 * BacklogItem (status derived from `reason`, mirroring what each real
 * detector anchors on) and inserts an open BacklogStuckState row directly via
 * the ent client — the same approach as backlog_stuck_rpc_test.go's
 * Go-test-only `seedOpenStuckRow` helper, just reachable over HTTP for
 * Playwright. That endpoint is registered ONLY when
 * STAPLER_SQUAD_INSTANCE=e2e-local (server.go), so it is never reachable in a
 * normal deploy. Note: the created BacklogItem's real ID is server-generated
 * — `opts.itemId` is not used as the DB id, only as a human-readable label;
 * locate seeded cards by title (see `cardByTitle` above), not itemId.
 */
export async function seedStuckItem(
  request: APIRequestContext,
  opts: {
    itemId: string;
    title: string;
    reason: string;
    firstDetectedAt?: Date;
    prNumber?: number;
    prUrl?: string;
    context?: string;
    /** When true, writes a real on-disk plan file and sets it as the item's plan_artifacts_path, so ApprovePlan succeeds for real instead of hitting its FailedPrecondition. */
    hasPlan?: boolean;
  }
): Promise<string> {
  const resp = await request.post(`${BASE_URL}/api/debug/backlog/seed-stuck`, {
    headers: { "Content-Type": "application/json" },
    data: {
      itemId: opts.itemId,
      title: opts.title,
      reason: opts.reason,
      firstDetectedAt: (opts.firstDetectedAt ?? new Date()).toISOString(),
      prNumber: opts.prNumber ?? 0,
      prUrl: opts.prUrl ?? "",
      context: opts.context ?? "",
      hasPlan: opts.hasPlan ?? false,
    },
  });
  if (!resp.ok()) {
    throw new Error(
      `seedStuckItem failed (${resp.status()}): debug seed endpoint is not yet implemented ` +
        `— see the KNOWN GAP note on this function. ${await resp.text().catch(() => "")}`
    );
  }
  const body = (await resp.json()) as { itemId: string };
  return body.itemId;
}

export async function enableBacklogFeatureFlag(request: APIRequestContext): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: "backlog", enabled: true },
  });
}

export async function disableBacklogFeatureFlag(request: APIRequestContext): Promise<void> {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: "backlog", enabled: false },
  });
}
