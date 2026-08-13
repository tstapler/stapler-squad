// @feature settings-backlog-sources, backlog:create-source, backlog:list-sources, backlog:update-source, backlog:delete-source
/**
 * E2E tests for the Backlog Sources settings page.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * These tests exercise CreateItemSource / ListItemSources / UpdateItemSource /
 * DeleteItemSource only — none of them trigger an actual sync (TriggerSync),
 * since that would make a real network call to api.github.com with a fake
 * token, which is unsuitable for CI.
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { BacklogSourcesSettingsPage } from "./pages/BacklogSourcesSettingsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

// GetFeatureFlags is NOT a reliable readiness signal here: when a
// FeatureController is wired for a flag (true for "backlog"), GetFeatureFlags
// reports the controller's in-memory IsEnabled() state, while BacklogService's
// RPCs are actually gated by a *separate* interceptor that re-reads
// config.LoadConfig() from disk on every call. UpdateFeatureFlag updates both,
// but they're independent reads with no shared cache — polling the wrong one
// can report "ready" before BacklogService RPCs actually unblock. Poll a real
// BacklogService RPC (ListItemSources) instead, so this waits on the exact
// signal that gates the page's own requests.
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

test.describe("backlog-sources-settings", () => {
  let sourcesPage: BacklogSourcesSettingsPage;

  // Backlog sources are gated behind the "backlog" feature flag (defaults to
  // off), same as tests/e2e/backlog.spec.ts — CreateItemSource returns a
  // "feature not enabled" error otherwise.
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

  test("AC-01 Add Source button disabled until required fields are filled", async ({ page }) => {
    await expect(page.getByRole("button", { name: "Add Source" })).toBeDisabled();
  });

  test("AC-02 add a GitHub Issues source and see it in the list", async () => {
    await sourcesPage.addSource({
      displayName: "E2E Test Source",
      owner: "acme",
      repo: "widgets",
      token: "e2e-fake-token",
    });
    await expect(sourcesPage.row("E2E Test Source")).toBeVisible();
  });

  test("AC-03 toggle a source's enabled state", async () => {
    await sourcesPage.addSource({
      displayName: "E2E Toggle Source",
      owner: "acme",
      repo: "toggle-repo",
      token: "e2e-fake-token",
    });
    const row = sourcesPage.row("E2E Toggle Source");
    const disableSwitch = row.getByRole("switch", { name: "Disable E2E Toggle Source" });
    await expect(disableSwitch).toBeVisible();
    await disableSwitch.click();
    await expect(row.getByRole("switch", { name: "Enable E2E Toggle Source" })).toBeVisible();
  });

  test("AC-04 deleting a source removes it from the list", async () => {
    await sourcesPage.addSource({
      displayName: "E2E Delete Source",
      owner: "acme",
      repo: "delete-repo",
      token: "e2e-fake-token",
    });
    const row = sourcesPage.row("E2E Delete Source");
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "Remove E2E Delete Source" }).click();
    await expect(row).not.toBeVisible();
  });

  test("AC-05 view history shows no sync runs yet for a never-synced source", async () => {
    await sourcesPage.addSource({
      displayName: "E2E History Source",
      owner: "acme",
      repo: "history-repo",
      token: "e2e-fake-token",
    });
    const row = sourcesPage.row("E2E History Source");
    await row.getByRole("button", { name: "View history" }).click();
    await expect(row.getByText("No sync runs yet.")).toBeVisible();
  });
});

test.describe("backlog-sources-settings - feature flag off", () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { "Content-Type": "application/json" },
      data: { name: "backlog", enabled: false },
    });
  });

  test("AC-06 direct navigation redirects to / when the flag is off", async ({ page }) => {
    await page.goto(`${BASE_URL}/settings/backlog-sources`, { waitUntil: "domcontentloaded" });
    await page.waitForURL(`${BASE_URL}/`, { timeout: 10000 });
    await expect(page).toHaveURL(`${BASE_URL}/`);
  });
});
