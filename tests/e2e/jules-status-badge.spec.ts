// @feature jules-status-badge
/**
 * E2E coverage for `JulesStatusBadge` (project_plans/google-jules-integration,
 * design/ux.md §7.10/§7.11/§7.12 — the "UX acceptance criteria" table
 * validation.md's implementation/validation.md promised for this file but
 * never actually created; this closes that spec-compliance gap).
 *
 * Three scenarios, deliberately the tractable third of validation.md's six:
 *
 *   §7.10 Screen-reader labels: every icon-bearing element this feature
 *     introduces (badge, escape-hatch link, PR provenance marker) has a
 *     non-empty accessible name distinct from its `title` attribute.
 *   §7.11 Color contrast: every reachable `JulesStatusBadge` phase meets
 *     4.5:1 contrast (Axe `color-contrast`) in both light and dark themes —
 *     reusing accessibility.spec.ts's @axe-core/playwright setup.
 *   §7.12 No optimistic flash: with the item-detail RPC held open, no badge
 *     — neutral or otherwise — is present until real Jules state arrives.
 *
 * Deliberately OUT of scope for this pass (left for a follow-up, per the
 * task brief that produced this file):
 *   §7.4  grayscale/colorblind check — no CDP-level visual-simulation
 *         harness wired up yet; also partially a manual step per
 *         docs/how-to/dispatch-work-to-google-jules.md's Pre-ship
 *         accessibility checklist.
 *   §7.5  mid-session network-throttling staleness check — needs CDP
 *         network-condition emulation this suite doesn't have.
 *   §7.16 reconnect-required auto-recovery — needs a stubbed 401/403
 *         mid-poll against the real poller's cadence; more involved than a
 *         seed-and-assert test.
 *
 * Two further, more specific scope notes discovered while writing this file
 * (not new bugs — narrowing what's actually testable through the real app):
 *
 * 1. Of `JulesSessionPhase`'s six variants (JulesStatusBadge.tsx), only
 *    "running", "done", and "failed" are reachable through SessionsSection's
 *    real data path — `computeJulesPhase` branches only on
 *    `session.endedAt`/`endReason`, so "queued" and "needs-review" are
 *    unproduceable dead states there today (confirmed by
 *    SessionsSection.jules.test.tsx never exercising them either). This spec
 *    covers the three reachable phases; the isolated component contract for
 *    all six (including queued/needs-review) is already covered by
 *    JulesStatusBadge.test.tsx at the unit level.
 * 2. "reconnect-required" needs `GetJulesConfig`'s live
 *    `auth_reconnect_required` flag, which BacklogItemDetail.tsx only ever
 *    reads via a `useEffect` that jules-dispatch.spec.ts's header comment
 *    documents (VERIFICATION STATUS, 2026-09-01) as never executing in a
 *    production (`next build` static-export) build — a pre-existing,
 *    already-filed gap, not something this spec works around. The same root
 *    cause makes /settings/jules's "revoke" affordance (ux.md §7.10's
 *    "revoke button") unreachable here too, so this spec covers the badge,
 *    the escape-hatch link, and the PR provenance marker only.
 *
 * Deviation from the task brief: `page.accessibility.snapshot()` was removed
 * from Playwright as of this repo's pinned version (1.62.1 — grep confirms
 * no `Accessibility` class remains in playwright-core's type defs). §7.10
 * instead asserts accessible names via `getByRole`/`getByLabel` (which read
 * the real browser accessibility tree through CDP under the hood) plus a
 * direct `title` attribute check — equivalent verification, different API.
 *
 * Prerequisites: same as jules-dispatch.spec.ts —
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";
import {
  seedJulesWorkSessionDirect,
  enableBacklogFeatureFlag,
  disableBacklogFeatureFlag,
} from "./pages/BacklogMutations";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

/** Suppresses both first-run onboarding overlays so they can't intercept
 * clicks on the backlog table — matches backlog-plan-approval-flicker.spec.ts. */
function suppressOnboarding(page: Page) {
  return page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
    localStorage.setItem("stapler-squad:backlog-onboarded", "true");
  });
}

test.describe("Jules status badge accessibility & optimistic-flash (§7.10 / §7.11 / §7.12)", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await suppressOnboarding(page);
  });

  test("every icon-bearing element has a non-empty accessible name distinct from its title attribute", async ({
    page,
    request,
  }) => {
    const suffix = Date.now();
    const runningTitle = `e2e-jules-a11y-running-${suffix}`;
    const doneTitle = `e2e-jules-a11y-done-${suffix}`;
    const failedTitle = `e2e-jules-a11y-failed-${suffix}`;
    const prTitle = `e2e-jules-a11y-pr-${suffix}`;

    await seedJulesWorkSessionDirect(request, { title: runningTitle, status: "review" });
    await seedJulesWorkSessionDirect(request, {
      title: doneTitle,
      status: "review",
      ended: true,
      endReason: "jules_completed",
    });
    await seedJulesWorkSessionDirect(request, {
      title: failedTitle,
      status: "review",
      ended: true,
      endReason: "jules_failed",
    });
    // pr_pending + a PR URL + the newest linked session having role
    // jules_work is PullRequestSection.tsx's exact gate for rendering the
    // "Opened by Jules" provenance marker (design/ux.md Surface D).
    await seedJulesWorkSessionDirect(request, {
      title: prTitle,
      status: "pr_pending",
      ended: true,
      endReason: "jules_completed",
      prNumber: 4242,
      prUrl: "https://github.com/tstapler/stapler-squad/pull/4242",
    });

    const backlogPage = new BacklogPage(page);
    const detailPage = new BacklogItemDetailPage(page);

    /**
     * Confirms `locator` resolves via its accessible name (proving the name
     * is non-empty and correctly computed — role queries read the real
     * browser accessibility tree), then confirms `title` is either absent or
     * different from that name.
     */
    async function assertNameDistinctFromTitle(locator: ReturnType<Page["getByRole"]>, expectedName: string) {
      await expect(locator).toBeVisible();
      const title = await locator.getAttribute("title");
      expect(
        title === null || title === "" || title !== expectedName,
        `title ("${title}") must be empty or distinct from the accessible name ("${expectedName}")`
      ).toBe(true);
    }

    await backlogPage.goto();
    await backlogPage.waitForItemCards();
    await detailPage.openItemByTitle(runningTitle);
    await assertNameDistinctFromTitle(page.getByRole("img", { name: "Jules: Running" }), "Jules: Running");

    await backlogPage.goto();
    await backlogPage.waitForItemCards();
    await detailPage.openItemByTitle(doneTitle);
    await assertNameDistinctFromTitle(page.getByRole("img", { name: "Jules: Done" }), "Jules: Done");

    await backlogPage.goto();
    await backlogPage.waitForItemCards();
    await detailPage.openItemByTitle(failedTitle);
    await assertNameDistinctFromTitle(page.getByRole("img", { name: "Jules: Failed" }), "Jules: Failed");

    await backlogPage.goto();
    await backlogPage.waitForItemCards();
    await detailPage.openItemByTitle(prTitle);
    await assertNameDistinctFromTitle(
      page.getByRole("link", { name: "View this session on jules.google.com" }),
      "View this session on jules.google.com"
    );
    // The provenance marker (a plain <span aria-label="Opened by Jules">) has
    // no ARIA role of its own, so it isn't reachable via getByRole — locate
    // it by its aria-label attribute directly (this repo's existing
    // convention for un-role-able elements, e.g. BacklogPage.ts's
    // `[aria-label^="Status:"]`) and verify the same name/title distinctness.
    const prMarker = page.locator('[aria-label="Opened by Jules"]');
    await expect(prMarker).toBeVisible();
    await expect(prMarker).toHaveAttribute("aria-label", "Opened by Jules");
    const markerTitle = await prMarker.getAttribute("title");
    expect(markerTitle === null || markerTitle === "" || markerTitle !== "Opened by Jules").toBe(true);
  });

  test("every JulesStatusBadge phase meets 4.5:1 contrast in light and dark themes", async ({ context, request }) => {
    test.setTimeout(120_000); // Axe scans are CPU-heavy — accessibility.spec.ts's convention.

    const suffix = Date.now();
    const items = [
      { title: `e2e-jules-contrast-running-${suffix}` },
      { title: `e2e-jules-contrast-done-${suffix}`, ended: true, endReason: "jules_completed" },
      { title: `e2e-jules-contrast-failed-${suffix}`, ended: true, endReason: "jules_failed" },
    ];
    for (const it of items) {
      await seedJulesWorkSessionDirect(request, { ...it, status: "review" });
    }

    for (const themeName of ["light", "dark"] as const) {
      // A fresh page per theme iteration (not the shared `page` fixture) —
      // matches accessibility.spec.ts's contrast test, avoiding stacked
      // addInitScript callbacks across iterations.
      const themedPage = await context.newPage();
      await themedPage.addInitScript((name) => {
        localStorage.setItem("stapler-theme", name);
        localStorage.setItem("stapler-squad:onboarded", "true");
        localStorage.setItem("stapler-squad:backlog-onboarded", "true");
      }, themeName);
      // Disable animation so Axe sees the final rendered state, not the
      // Running phase icon's mid-spin opacity (accessibility.spec.ts convention).
      await themedPage.emulateMedia({ reducedMotion: "reduce" });

      const backlogPage = new BacklogPage(themedPage);
      const detailPage = new BacklogItemDetailPage(themedPage);

      for (const it of items) {
        await backlogPage.goto();
        await backlogPage.waitForItemCards();
        await detailPage.openItemByTitle(it.title);
        await expect(themedPage.getByTestId("jules-status-badge")).toBeVisible();

        const results = await new AxeBuilder({ page: themedPage })
          .include('[data-testid="jules-status-badge"]')
          .withRules(["color-contrast"])
          .analyze();

        expect(results.violations, `color-contrast violations for "${it.title}" in ${themeName} theme`).toHaveLength(
          0
        );
      }

      await themedPage.close();
    }
  });

  test("no badge renders before the first Jules state arrives", async ({ page, request }) => {
    const title = `e2e-jules-no-flash-${Date.now()}`;
    const { itemId } = await seedJulesWorkSessionDirect(request, { title, status: "review" });

    // Holds every RPC that can deliver this item's data open until explicitly
    // released — deterministic (no waitForTimeout/race) equivalent of the
    // task brief's suggested artificial setTimeout-before-route.continue()
    // delay (this repo has no existing page.route-based delay helper;
    // checked tests/e2e/ for one first).
    //
    // All three are gated, not just GetBacklogItem: BacklogItemDetail.tsx's
    // `liveRawItem` (fed by useWatchBacklogItems, which internally calls
    // ListBacklogItems for its initial sync before subscribing to
    // WatchBacklogItems' live deltas) can populate `item` *before*
    // GetBacklogItem's own `load()` call resolves — confirmed by running
    // this test with only GetBacklogItem gated, where the badge was already
    // present the instant the pane appeared. Navigating straight to the
    // `?item=` deep link (rather than clicking through an already-loaded
    // list) additionally avoids the list page's own useWatchBacklogItems
    // instance having already warmed the shared store before this item was
    // even seeded.
    let releaseGate: () => void = () => {};
    const gate = new Promise<void>((resolve) => {
      releaseGate = resolve;
    });
    const holdOpen = async (route: import("@playwright/test").Route) => {
      await gate;
      await route.continue();
    };
    await page.route("**/api/session.v1.BacklogService/GetBacklogItem", holdOpen);
    await page.route("**/api/session.v1.BacklogService/ListBacklogItems", holdOpen);
    await page.route("**/api/session.v1.BacklogService/WatchBacklogItems", holdOpen);

    const detailPage = new BacklogItemDetailPage(page);
    await page.goto(`${BASE_URL}/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });

    // Per BacklogItemDetail.tsx's `loading && !item` branch, the pane's
    // data-testid renders immediately as a loading skeleton, independent of
    // any of the three held-open RPCs above.
    await expect(detailPage.pane).toBeVisible();

    // No real Jules state (`item.linkedSessions`) has arrived yet — no
    // badge, neutral or otherwise, should be present.
    await expect(page.getByTestId("jules-status-badge")).toHaveCount(0);

    releaseGate();

    await expect(page.getByTestId("jules-status-badge")).toBeVisible({ timeout: 10_000 });
  });
});
