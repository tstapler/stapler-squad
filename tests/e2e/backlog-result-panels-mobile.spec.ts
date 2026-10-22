// @feature backlog-triage-results-panel, backlog-review-results-panel
// Criterion 10: formatted result panels stay usable at 320x568 (no horizontal overflow, 44px disclosure target).
// The review panel shares ResultPanel.css.ts and RawJsonDisclosure with the triage panel; only triage can be seeded.
import { test, expect } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage, seedHeadlessTriageItem } from "./pages/BacklogItemDetailPage";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

test.use({ viewport: { width: 320, height: 568 }, isMobile: true, hasTouch: true });

test("triageResultPanel_should_NotOverflowAnd_Have44pxDisclosure_When_320px", async ({ page, request }) => {
  await enableBacklogFeatureFlag(request);
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name: "backlog-triage-results-panel", enabled: true },
  });
  await page.addInitScript(() => {
    window.localStorage.setItem("stapler-squad:onboarded", "true");
    window.localStorage.setItem("stapler-squad:backlog-onboarded", "true");
  });

  const title = `e2e triage panel mobile ${Date.now()}`;
  await seedHeadlessTriageItem(request, {
    title,
    summary: "A very long unbroken summary " + "x".repeat(200),
  });

  const backlogPage = new BacklogPage(page);
  await backlogPage.goto();
  await backlogPage.waitForItemCards();
  await new BacklogItemDetailPage(page).openItemByTitle(title);

  const panel = page.getByTestId("triage-result-panel");
  await expect(panel).toBeVisible();
  const raw = page.getByTestId("triage-raw-json");
  await raw.getByText("View raw JSON").click();
  await expect(raw.locator("pre")).toBeVisible();

  const summaryBox = await raw.locator("summary").boundingBox();
  expect(summaryBox!.height).toBeGreaterThanOrEqual(44);
  // Page-level reflow is scoped to the panel: the item header's Edit/Close buttons overflow at 320px independently of this change.
  const overflow = await page.evaluate(() => {
    const doc = document.scrollingElement as HTMLElement;
    const panelEl = document.querySelector<HTMLElement>('[data-testid="triage-result-panel"]')!;
    const outside = [...panelEl.querySelectorAll<HTMLElement>("*")].filter((n) => n.getBoundingClientRect().right > window.innerWidth + 1 && !n.closest("pre"));
    return { page: doc.scrollWidth - window.innerWidth, panel: panelEl.scrollWidth - panelEl.clientWidth, outside: outside.length };
  });
  expect(overflow).toEqual({ page: 0, panel: 0, outside: 0 });
  const box = await panel.boundingBox();
  expect(box!.x + box!.width).toBeLessThanOrEqual(320);
});
