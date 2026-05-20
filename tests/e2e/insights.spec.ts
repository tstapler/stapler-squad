// @feature insights-dashboard
// Tests for the /insights page (token usage analytics dashboard).
// Validates: page renders, nav link exists, summary cards shown, no crash.

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

test.describe("insights-dashboard", () => {
  test("insights-dashboard_should_renderPage_When_navigatedTo", async ({ page }) => {
    await page.goto(`${BASE_URL}/insights`, { waitUntil: "domcontentloaded" });

    // Page title should be set
    await expect(page).toHaveTitle(/Insights/i);
  });

  test("insights-dashboard_should_showHeading_When_pageLoads", async ({ page }) => {
    await page.goto(`${BASE_URL}/insights`, { waitUntil: "domcontentloaded" });

    // h1 with "Insights" text visible
    await expect(page.getByRole("heading", { name: /Insights/i, level: 1 })).toBeVisible({
      timeout: 10000,
    });
  });

  test("insights-dashboard_should_showSubtitle_When_pageLoads", async ({ page }) => {
    await page.goto(`${BASE_URL}/insights`, { waitUntil: "domcontentloaded" });

    // Subtitle text visible
    await expect(page.getByText(/Token usage analytics/i)).toBeVisible({ timeout: 10000 });
  });

  test("insights-dashboard_should_reachableFromNav_When_hamburgerOpened", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    // Insights is a secondary (hamburger-only) nav item
    const hamburger = page.getByRole("button", { name: /menu|more/i });
    if (await hamburger.isVisible()) {
      await hamburger.click();
      const link = page.getByRole("link", { name: /^Insights$/i });
      await expect(link).toBeVisible({ timeout: 5000 });
    } else {
      // On wide viewport it may be in sidebar
      const link = page.getByRole("link", { name: /^Insights$/i });
      await expect(link).toBeVisible({ timeout: 5000 });
    }
  });

  test("insights-dashboard_should_showLoadingOrData_When_apiAvailable", async ({ page }) => {
    await page.goto(`${BASE_URL}/insights`, { waitUntil: "domcontentloaded" });

    // Either loading state, empty state, or actual data — no crash
    const hasContent = await Promise.race([
      page.getByText(/Loading token data/i).waitFor({ timeout: 8000 }).then(() => true).catch(() => false),
      page.getByText(/No token usage data/i).waitFor({ timeout: 8000 }).then(() => true).catch(() => false),
      page.getByText(/Total Cost/i).waitFor({ timeout: 8000 }).then(() => true).catch(() => false),
      page.getByText(/Parsing conversation history/i).waitFor({ timeout: 8000 }).then(() => true).catch(() => false),
    ]);

    expect(hasContent).toBe(true);
  });
});
