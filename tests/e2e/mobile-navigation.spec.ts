// @feature ui:mobile-navigation, ui:session-list, ui:bottom-nav
// E2E verification for three mobile UX bug fixes:
//  E2E-1: Session card tap switches to detail pane on mobile (REQ-1a)
//  E2E-2: Session list container is scrollable (REQ-2a)
//  E2E-3: --bottom-nav-height CSS variable is set to a non-zero px value (REQ-3d)

import { test, expect } from "@playwright/test";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const MOBILE_VIEWPORT = { width: 390, height: 844 };

test.describe("mobile-navigation", () => {
  test("mobile_should_showDetailPane_When_sessionCardTapped", async ({
    page,
  }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 15000 });

    const firstCard = page.locator('[data-testid="session-card"]').first();
    const cardVisible = await firstCard.isVisible().catch(() => false);
    if (!cardVisible) {
      test.skip();
      return;
    }

    await firstCard.click();

    // After click, session-detail pane should be visible
    await expect(
      page.locator('[data-testid="session-detail"]').first()
    ).toBeVisible({ timeout: 5000 });
  });

  test("mobile_should_scrollSessionList_When_sessionsExist", async ({
    page,
  }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 15000 });

    const scrollWrapper = page.locator('[data-testid="session-list-scroll"]').first();
    await expect(scrollWrapper).toBeVisible({ timeout: 10000 });

    // Verify the scroll container has overflow-y scroll/auto applied via CSS class
    const overflowY = await scrollWrapper.evaluate((el) =>
      getComputedStyle(el).overflowY
    );
    expect(["auto", "scroll"]).toContain(overflowY);
  });

  test("mobile_should_setBottomNavHeightVar_When_pageLoads", async ({
    page,
  }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 15000 });

    // Wait for BottomNav ResizeObserver to set the variable to a non-zero value
    await page.waitForFunction(
      () => {
        const v = getComputedStyle(document.documentElement).getPropertyValue(
          "--bottom-nav-height"
        );
        return v.trim().length > 0 && v.trim() !== "0px";
      },
      { timeout: 5000 }
    );

    const value = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue(
        "--bottom-nav-height"
      )
    );

    expect(value.trim()).toMatch(/^\d+px$/);
    expect(value.trim()).not.toBe("0px");
  });
});
