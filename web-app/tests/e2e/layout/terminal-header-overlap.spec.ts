/**
 * Terminal / header overlap test harnesses.
 *
 * Verifies that session-detail modals never render their content underneath
 * the sticky navigation header, and that tabs + toolbar buttons are always
 * visible (no fullscreen-only toggling required).
 *
 * Uses /test/layout-overlap which renders the modal shell with no backend.
 *
 * Modes:
 *   (default)  — sessions page modal
 *   ?mode=rq   — review-queue page modal
 */

import { test, expect, Page } from "@playwright/test";

// Allow 2 px tolerance for sub-pixel rounding differences across browsers.
const OVERLAP_TOLERANCE_PX = 2;

async function getHeaderBottom(page: Page): Promise<number> {
  const box = await page.getByTestId("app-header").boundingBox();
  if (!box) throw new Error("app-header not found");
  return box.y + box.height;
}

async function getModalTop(page: Page): Promise<number> {
  const box = await page.getByTestId("modal-content").boundingBox();
  if (!box) throw new Error("modal-content not found");
  return box.y;
}

async function getModalBottom(page: Page): Promise<number> {
  const box = await page.getByTestId("modal-content").boundingBox();
  if (!box) throw new Error("modal-content not found");
  return box.y + box.height;
}

async function navigateTo(page: Page, mode?: string) {
  const url = mode ? `/test/layout-overlap?mode=${mode}` : "/test/layout-overlap";
  await page.goto(url);
  await page.getByTestId("modal-content").waitFor({ state: "visible" });
  await page.waitForTimeout(100); // allow one layout paint
}

// ─── overlap: modal stays below header ───────────────────────────────────────

test.describe("Modal must not overlap sticky header", () => {
  test("sessions page: modal top is below header", async ({ page }) => {
    await navigateTo(page);
    const headerBottom = await getHeaderBottom(page);
    const modalTop = await getModalTop(page);
    console.log(`[sessions] headerBottom=${headerBottom} modalTop=${modalTop}`);
    expect(modalTop + OVERLAP_TOLERANCE_PX).toBeGreaterThanOrEqual(headerBottom);
  });

  test("review-queue page: modal top is below header", async ({ page }) => {
    await navigateTo(page, "rq");
    const headerBottom = await getHeaderBottom(page);
    const modalTop = await getModalTop(page);
    console.log(`[rq] headerBottom=${headerBottom} modalTop=${modalTop}`);
    expect(modalTop + OVERLAP_TOLERANCE_PX).toBeGreaterThanOrEqual(headerBottom);
  });

  test("sessions page: terminal toolbar is below header", async ({ page }) => {
    await navigateTo(page);
    const headerBottom = await getHeaderBottom(page);
    const box = await page.getByTestId("terminal-toolbar").boundingBox();
    if (!box) throw new Error("terminal-toolbar not found");
    console.log(`[sessions] headerBottom=${headerBottom} toolbarTop=${box.y}`);
    expect(box.y + OVERLAP_TOLERANCE_PX).toBeGreaterThanOrEqual(headerBottom);
  });

  test("review-queue page: terminal toolbar is below header", async ({ page }) => {
    await navigateTo(page, "rq");
    const headerBottom = await getHeaderBottom(page);
    const box = await page.getByTestId("terminal-toolbar").boundingBox();
    if (!box) throw new Error("terminal-toolbar not found");
    console.log(`[rq] headerBottom=${headerBottom} toolbarTop=${box.y}`);
    expect(box.y + OVERLAP_TOLERANCE_PX).toBeGreaterThanOrEqual(headerBottom);
  });
});

// ─── vertical bounds: modal fits inside viewport ──────────────────────────────

test.describe("Modal must fit inside viewport vertically", () => {
  for (const mode of [undefined, "rq"] as const) {
    const label = mode ?? "sessions";

    test(`${label}: modal bottom does not exceed viewport`, async ({ page }) => {
      await navigateTo(page, mode);
      const modalBottom = await getModalBottom(page);
      const vh = page.viewportSize()!.height;
      console.log(`[${label}] modalBottom=${modalBottom} vh=${vh}`);
      expect(modalBottom).toBeLessThanOrEqual(vh + OVERLAP_TOLERANCE_PX);
    });
  }
});

// ─── tabs always visible (no fullscreen toggling needed) ─────────────────────

test.describe("Tabs must always be visible without toggling fullscreen", () => {
  for (const mode of [undefined, "rq"] as const) {
    const label = mode ?? "sessions";

    test(`${label}: all five tabs are rendered`, async ({ page }) => {
      await navigateTo(page, mode);
      await expect(page.getByTestId("session-tabs")).toBeVisible();
      for (const tab of ["terminal", "diff", "vcs", "logs", "info"]) {
        await expect(page.getByTestId(`tab-${tab}`)).toBeVisible();
      }
    });

    test(`${label}: tabs are below the header`, async ({ page }) => {
      await navigateTo(page, mode);
      const headerBottom = await getHeaderBottom(page);
      const box = await page.getByTestId("session-tabs").boundingBox();
      if (!box) throw new Error("session-tabs not found");
      console.log(`[${label}] headerBottom=${headerBottom} tabsTop=${box.y}`);
      expect(box.y + OVERLAP_TOLERANCE_PX).toBeGreaterThanOrEqual(headerBottom);
    });
  }
});

// ─── toolbar buttons always visible ──────────────────────────────────────────

test.describe("Terminal toolbar buttons must always be visible", () => {
  const TOOLBAR_BUTTONS = ["debug", "record", "raw", "resize", "clear", "bottom", "copy"];

  for (const mode of [undefined, "rq"] as const) {
    const label = mode ?? "sessions";

    test(`${label}: all toolbar buttons are visible`, async ({ page }) => {
      await navigateTo(page, mode);
      for (const btn of TOOLBAR_BUTTONS) {
        await expect(page.getByTestId(`toolbar-btn-${btn}`)).toBeVisible();
      }
    });
  }
});
