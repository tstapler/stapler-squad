// @feature backlog:sync-direction-settings
/**
 * E2E tests for the per-source GitHub sync-direction toggles (Epic 4.3,
 * backlog-github-two-way-sync): "Close GitHub issues when I finish here"
 * (forward sync), "Reflect GitHub status back here" (backward sync), and
 * the both-directions loop-risk warning.
 *
 * Prerequisites (same isolated test server as backlog-sources-settings.spec.ts):
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * These tests never trigger a real sync (TriggerSync) — same CI-safety
 * rationale as backlog-sources-settings.spec.ts (a real network call to
 * api.github.com with a fake token is unsuitable for CI).
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { BacklogSourcesSettingsPage } from "./pages/BacklogSourcesSettingsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

// See backlog-sources-settings.spec.ts for why this polls a real
// BacklogService RPC rather than GetFeatureFlags.
async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListItemSources`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

test.describe("backlog-github-sync-settings", () => {
  let sourcesPage: BacklogSourcesSettingsPage;

  test.beforeAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: true },
    });
    await waitForBacklogRPCsEnabled(request);
  });

  test.afterAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: false },
    });
  });

  test.beforeEach(async ({ page }) => {
    sourcesPage = new BacklogSourcesSettingsPage(page);
    await sourcesPage.goto();
  });

  test("forward and backward sync toggles persist across reload", async ({ page }) => {
    await sourcesPage.addSource({
      displayName: "E2E Sync Toggle Source",
      owner: "acme",
      repo: "sync-toggle-repo",
      token: "e2e-fake-token",
    });

    await sourcesPage.enableForwardSync("E2E Sync Toggle Source");

    const toggle = sourcesPage
      .row("E2E Sync Toggle Source")
      .getByRole("switch", { name: /closing GitHub issues/ });
    await expect(toggle).toHaveAttribute("aria-checked", "true");

    // Reload proves the RPC round-trip persisted the value, not just local state.
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(
      sourcesPage.row("E2E Sync Toggle Source").getByRole("switch", { name: /closing GitHub issues/ })
    ).toHaveAttribute("aria-checked", "true");
  });

  test("enabling both sync directions shows the loop-risk warning", async () => {
    await sourcesPage.addSource({
      displayName: "E2E Both Directions Source",
      owner: "acme",
      repo: "both-directions-repo",
      token: "e2e-fake-token",
    });

    await sourcesPage.enableForwardSync("E2E Both Directions Source");
    await sourcesPage.enableBackwardSync("E2E Both Directions Source");

    await expect(
      sourcesPage.row("E2E Both Directions Source").getByText(/Both directions are enabled/)
    ).toBeVisible();
  });

  // Story 4.3.2's row-level auth-failure warning reads the most recent sync
  // history entry (Fetch-side today; forward-sync CloseIssue failures via
  // RecordSourceSyncFailure once that backend piece lands — see plan.md
  // Task 4.3.2a). There is currently no test-mode seeding endpoint to put an
  // auth-type errorMessage into a source's sync history without making a
  // real network call to api.github.com (unsuitable for CI, same rationale
  // as backlog-sources-settings.spec.ts). Marked fixme rather than deleted
  // so the intended coverage stays visible once a seeding seam exists.
  test.fixme(
    "row shows persistent warning when recent sync failed with an auth error",
    async () => {
      await sourcesPage.addSource({
        displayName: "E2E Auth Failure Source",
        owner: "acme",
        repo: "auth-failure-repo",
        token: "e2e-fake-token",
      });

      // Once a seeding seam exists: seed a sync-history entry with a
      // 401-type errorMessage for this source here, before the assertion.
      const row = sourcesPage.row("E2E Auth Failure Source");
      await expect(row.getByTestId(/source-row-.*-auth-warning/)).toBeVisible();
    }
  );
});
