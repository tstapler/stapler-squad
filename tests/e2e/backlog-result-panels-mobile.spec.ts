// @feature backlog-triage-results-panel, backlog-review-results-panel
// Criterion 10: formatted result panels stay usable at 320x568 (no horizontal overflow, 44px disclosure target).
import { test, expect, type APIRequestContext, type Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage, seedHeadlessTriageItem, seedReviewVerdictItem } from "./pages/BacklogItemDetailPage";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

test.use({ viewport: { width: 320, height: 568 }, isMobile: true, hasTouch: true });

async function enableFlag(request: APIRequestContext, name: string) {
  await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
    headers: { "Content-Type": "application/json" },
    data: { name, enabled: true },
  });
}

async function openItem(page: Page, request: APIRequestContext, title: string) {
  await enableBacklogFeatureFlag(request);
  await page.addInitScript(() => {
    window.localStorage.setItem("stapler-squad:onboarded", "true");
    window.localStorage.setItem("stapler-squad:backlog-onboarded", "true");
  });
  return async () => {
    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();
    await new BacklogItemDetailPage(page).openItemByTitle(title);
  };
}

async function expectPanelFitsAt320(page: Page, panelId: string, rawId: string) {
  const panel = page.getByTestId(panelId);
  await expect(panel).toBeVisible();
  const raw = page.getByTestId(rawId);
  await raw.getByText("View raw JSON").click();
  await expect(raw.locator("pre")).toBeVisible();

  const summaryBox = await raw.locator("summary").boundingBox();
  expect(summaryBox!.height).toBeGreaterThanOrEqual(44);
  // Scoped to the panel: the item header's Edit/Close buttons overflow at 320px independently of these panels.
  const overflow = await page.evaluate((id) => {
    const doc = document.scrollingElement as HTMLElement;
    const panelEl = document.querySelector<HTMLElement>(`[data-testid="${id}"]`)!;
    const outside = [...panelEl.querySelectorAll<HTMLElement>("*")].filter((n) => n.getBoundingClientRect().right > window.innerWidth + 1 && !n.closest("pre"));
    return { page: doc.scrollWidth - window.innerWidth, panel: panelEl.scrollWidth - panelEl.clientWidth, outside: outside.length };
  }, panelId);
  expect(overflow).toEqual({ page: 0, panel: 0, outside: 0 });
  const box = await panel.boundingBox();
  expect(box!.x + box!.width).toBeLessThanOrEqual(320);
}

test("triageResultPanel_should_NotOverflowAnd_Have44pxDisclosure_When_320px", async ({ page, request }) => {
  await enableFlag(request, "backlog-triage-results-panel");
  const title = `e2e triage panel mobile ${Date.now()}`;
  const open = await openItem(page, request, title);
  await seedHeadlessTriageItem(request, { title, summary: "A very long unbroken summary " + "x".repeat(200) });
  await open();
  await expectPanelFitsAt320(page, "triage-result-panel", "triage-raw-json");
});

test("reviewVerdictPanel_should_NotOverflowAnd_Have44pxDisclosure_When_320px", async ({ page, request }) => {
  await enableFlag(request, "backlog-review-results-panel");
  const title = `e2e review panel mobile ${Date.now()}`;
  const open = await openItem(page, request, title);
  await seedReviewVerdictItem(request, { title });
  await open();
  await expectPanelFitsAt320(page, "review-verdict-panel", "review-raw-json");
});
