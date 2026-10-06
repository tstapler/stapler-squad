// @feature backlog:list-stuck
/**
 * E2E tests for the "Stuck Backlog Items" section on /unfinished
 * (backlog-stuck-item-visibility, Epic 4.1 / Story 4.1.5).
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * Test data is seeded via `seedStuckItem()` in ./pages/StuckItemsPage.ts,
 * which writes an open BacklogStuckState row directly (bypassing the
 * reconciler/detectors), per validation.md's own note for these tests.
 *
 * `seedStuckItem()` calls the `/api/debug/backlog/seed-stuck` debug endpoint
 * (server/services/backlog_debug_seed_handler.go), which is only registered
 * when STAPLER_SQUAD_INSTANCE=e2e-local (see server.go) — never reachable in
 * a normal deploy.
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import {
  StuckItemsPage,
  seedStuckItem,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from "./pages/StuckItemsPage";
import { BacklogPage } from "./pages/BacklogPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListStuckBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

test.describe("stuck items", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    // Skip the first-run onboarding modal (shows on any fresh browser context after
    // 800ms — see web-app/src/components/onboarding/useOnboarding.ts), matching the
    // convention in triggers-panel.spec.ts / rule-builder-ci-passing.spec.ts.
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
  });

  test("shows nav badge with no click and full detail within two clicks", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-pr-ready-1",
      title: "fix: benchmark job CI",
      reason: "pr_ready_unmerged",
      firstDetectedAt: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000),
      prNumber: 148,
      prUrl: "https://github.com/tstapler/stapler-squad/pull/148",
      context: "PR #148 green & mergeable, unmerged for 3 days",
    });

    // 0 clicks: nav badge visible from any page.
    await page.goto(`${BASE_URL}/`, { waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("stuck-nav-badge")).toBeVisible();

    // Click 1: navigate to /unfinished via the nav link (nav-pages.ts labels this
    // route "Up Next", not "Unfinished" — the page's own heading/route name differs
    // from its nav entry's display label).
    await page.getByRole("link", { name: /Up Next/ }).first().click();
    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();
    await expect(stuckPage.section).toBeVisible();

    // Click 2: expand the card for full detail.
    const card = stuckPage.cardByTitle("fix: benchmark job CI");
    await card.click();
    await expect(page.getByTestId("stuck-item-detail")).toBeVisible();
  });

  test("filters to a single reason class in one chip click", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-pr-ready-2",
      title: "fix: filter test pr-ready",
      reason: "pr_ready_unmerged",
      prNumber: 200,
      prUrl: "https://github.com/tstapler/stapler-squad/pull/200",
    });
    await seedStuckItem(request, {
      itemId: "e2e-rework-2",
      title: "fix: filter test rework-cap",
      reason: "rework_cap",
      context: "cap hit",
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();

    const prChip = page.getByRole("button", { name: /PR ready to merge/ });
    await prChip.click();
    await expect(prChip).toHaveAttribute("aria-pressed", "true");
    await expect(stuckPage.cardByTitle("fix: filter test pr-ready")).toBeVisible();
    await expect(stuckPage.cardByTitle("fix: filter test rework-cap")).not.toBeVisible();
  });

  test("opens the source PR in one click from expanded detail", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-pr-ready-3",
      title: "fix: pr link test",
      reason: "pr_ready_unmerged",
      prNumber: 148,
      prUrl: "https://github.com/tstapler/stapler-squad/pull/148",
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();
    await stuckPage.cardByTitle("fix: pr link test").click();

    const prLink = page.getByRole("link", { name: /PR #148/ });
    await expect(prLink).toBeVisible();
    await expect(prLink).toHaveAttribute("target", "_blank");
  });

  test("clears a zero-result filter in one click back to All", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-rework-3",
      title: "fix: zero-result filter test",
      reason: "rework_cap",
      context: "cap hit",
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();

    // pr_ready_unmerged has 0 items in this scenario.
    await stuckPage.filterChip("1").click(); // STUCK_REASON_PR_READY_UNMERGED = 1
    await expect(stuckPage.filteredEmptyState).toBeVisible();
    await expect(stuckPage.clearFilterButton).toBeVisible();

    await stuckPage.clearFilterButton.click();
    await expect(stuckPage.filterChip("all")).toHaveAttribute("aria-pressed", "true");
    await expect(stuckPage.filteredEmptyState).not.toBeVisible();
  });

  test("filtered-empty state always exposes Clear filter", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-rework-4",
      title: "fix: clear filter always test",
      reason: "rework_cap",
      context: "cap hit",
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();
    await stuckPage.filterChip("1").click(); // 0-count reason
    await expect(stuckPage.clearFilterButton).toBeVisible();
    await stuckPage.clearFilterButton.click();
    await expect(stuckPage.filterChip("all")).toHaveAttribute("aria-pressed", "true");
  });

  test("is fully operable by keyboard for chips and cards", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-keyboard-1",
      title: "fix: keyboard operability test",
      reason: "pr_ready_unmerged",
      prNumber: 148,
      prUrl: "https://github.com/tstapler/stapler-squad/pull/148",
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();

    const chip = page.getByRole("button", { name: /PR ready to merge/ });
    await chip.focus();
    await page.keyboard.press("Space");
    await expect(chip).toHaveAttribute("aria-pressed", "true");

    const card = stuckPage.cardByTitle("fix: keyboard operability test");
    await card.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByTestId("stuck-item-detail")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("stuck-item-detail")).not.toBeVisible();
    await expect(card).toBeFocused();
  });

  test("snoozes an item in two clicks and it leaves the list", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-snooze-1",
      title: "fix: snooze flow test",
      reason: "rework_cap",
      context: "cap hit",
    });

    const stuckPage = new StuckItemsPage(page);
    await stuckPage.goto();

    const card = stuckPage.cardByTitle("fix: snooze flow test");
    await expect(card).toBeVisible();

    // Hover reveals the "Snooze" control (matches UnfinishedItem's hover-reveal
    // dismiss/snooze buttons); Playwright's .click() hovers implicitly first.
    const snoozeTrigger = card.getByTestId("stuck-item-snooze-trigger");
    await card.hover();
    await expect(snoozeTrigger).toBeVisible();

    // Click 1: open the duration picker.
    await snoozeTrigger.click();
    const picker = page.getByTestId("stuck-item-snooze-picker");
    await expect(picker).toBeVisible();

    // Click 2: confirm with the default pre-selected duration (1 day).
    await picker.getByTestId("stuck-item-snooze-confirm").click();

    await expect(card).not.toBeVisible();
  });

  test("retries a stuck item from the BlockerChip on the backlog detail page", async ({ page, request }) => {
    const itemId = await seedStuckItem(request, {
      itemId: "e2e-blocker-chip-retry-1",
      title: "fix: blocker chip retry test",
      reason: "stale_work",
      context: "no activity for 4 days",
    });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForPageLoad();
    await backlogPage.openItemDetail("fix: blocker chip retry test");

    const lifecycleSummary = page.getByTestId("lifecycle-summary");
    await expect(lifecycleSummary).toBeVisible();

    const retryButton = lifecycleSummary.getByTestId("blocker-chip-retry");
    await expect(retryButton).toBeEnabled();
    await retryButton.click();

    // Pending state fires synchronously on click.
    await expect(retryButton).toBeDisabled();
    await expect(lifecycleSummary.getByTestId("blocker-chip-duration")).toHaveTextContent("Retrying…");

    // "stale_work" is a wired remediation reason (server/services/backlog_service_stuck.go),
    // so it resolves to idle (no inline error) rather than CodeUnimplemented.
    await expect(retryButton).toBeEnabled({ timeout: 10_000 });
    await expect(lifecycleSummary.getByTestId("blocker-chip-error")).not.toBeVisible();

    expect(itemId).toBeTruthy();
  });

  test("deep-links from /unfinished?item=<id> to an auto-expanded stuck item card", async ({ page, request }) => {
    const itemId = await seedStuckItem(request, {
      itemId: "e2e-deep-link-1",
      title: "fix: deep link auto-expand test",
      reason: "rework_cap",
      context: "cap hit",
    });

    await page.goto(`${BASE_URL}/unfinished?item=${itemId}`, { waitUntil: "domcontentloaded" });
    const stuckPage = new StuckItemsPage(page);
    await expect(stuckPage.section).toBeVisible();

    // The linked item auto-expands without a manual click.
    await expect(page.getByTestId("stuck-item-detail")).toBeVisible();
    await expect(stuckPage.cardByTitle("fix: deep link auto-expand test")).toBeVisible();
  });

  test("ignores an unknown ?item= id and renders the section normally", async ({ page, request }) => {
    await seedStuckItem(request, {
      itemId: "e2e-deep-link-negative-1",
      title: "fix: deep link negative test",
      reason: "rework_cap",
      context: "cap hit",
    });

    await page.goto(`${BASE_URL}/unfinished?item=does-not-exist`, { waitUntil: "domcontentloaded" });
    const stuckPage = new StuckItemsPage(page);
    await expect(stuckPage.section).toBeVisible();

    // No card should be auto-expanded for a non-matching id.
    await expect(page.getByTestId("stuck-item-detail")).not.toBeVisible();
    await expect(stuckPage.cardByTitle("fix: deep link negative test")).toBeVisible();
  });
});
