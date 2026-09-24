// @feature window:switch, window:rename
/**
 * REQ-10 (validation.md's Requirement -> Test Mapping): every other REQ-10 row in this
 * repo's test suite exercises useWindowUrlSync against a mocked router/DOM
 * (web-app/src/lib/window/__tests__/useWindowUrlSync.test.ts,
 * web-app/src/app/__tests__/page.test.tsx). These two cases are the first real-browser
 * proof of ADR-001's "the URL is the source of truth, no shared activeWindowId" claim
 * (implementation/pre-mortem.md P1 #3 / Failure Mode #3).
 *
 * Two real tabs of one browser, sharing localStorage: opened via `context.newPage()` on
 * the test's own `context` fixture, not `browser.newPage()` — `browser.newPage()` creates
 * a brand-new BrowserContext (and therefore an isolated localStorage) on every call, which
 * would silently defeat the point of these tests: the `storage`-event cross-tab sync this
 * feature depends on (Story 1.3.3) only fires between pages that share an origin's
 * localStorage, i.e. pages in the same BrowserContext.
 */
import { test, expect } from '@playwright/test';
import { WindowTabStripPage } from './pages/WindowTabStripPage';

test.describe('multi-window cross-tab (REQ-10)', () => {
  test("switching windows in one tab does not affect the other tab's window", async ({ page, context }) => {
    // Suppress the onboarding modal (useOnboarding.ts shows it 800ms after a
    // fresh load with no `stapler-squad:onboarded` key) — applied at the
    // context level so it also covers page2, opened later via
    // context.newPage(). Without this, its Dialog.Overlay intercepts clicks
    // on the tab strip once a test's interactions run past 800ms, matching
    // this repo's existing e2e convention (see launcher-presets.spec.ts).
    await context.addInitScript(() => {
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });

    // Page 1: a fresh app load creates a single default window ("Window 1").
    await page.goto('/');
    const stripA = new WindowTabStripPage(page);
    await stripA.waitForLoaded();
    await expect(stripA.getTab('Window 1')).toBeVisible();
    const idA = WindowTabStripPage.windowIdFromUrl(page.url());
    expect(idA).toBeTruthy();

    // Create a second window (win-B) — page.tsx wires "+" to switchToWindow(createWindow()).
    await stripA.createWindow();
    await expect(stripA.tabs).toHaveCount(2);
    await expect(stripA.getActiveTab()).toHaveAttribute('title', 'Window 2');
    const idB = WindowTabStripPage.windowIdFromUrl(page.url());
    expect(idB).toBeTruthy();
    expect(idB).not.toBe(idA);

    // Wait for the debounced (~300ms) windows-array save to actually land in
    // localStorage before opening a second real tab — otherwise page 2 can
    // read a stale single-window snapshot on mount (see WindowTabStripPage's
    // waitForPersistedWindowCount doc comment).
    await stripA.waitForPersistedWindowCount(2);

    // Page 2: a second real tab in the SAME browser context, independently navigated to win-A.
    const page2 = await context.newPage();
    const stripB = new WindowTabStripPage(page2);
    await page2.goto(`/?window=${idA}`);
    await stripB.waitForLoaded();

    // Given: page 1 shows win-B active (from creating it above), page 2 independently shows win-A active.
    await expect(stripA.getActiveTab()).toHaveAttribute('title', 'Window 2');
    await expect(stripB.getActiveTab()).toHaveAttribute('title', 'Window 1');

    // When: page 1 switches to win-A via its own tab strip.
    await stripA.switchToTab('Window 1');
    await expect(stripA.getActiveTab()).toHaveAttribute('title', 'Window 1');

    // Then: page 2 is unaffected — still independently on win-A, its URL param unchanged,
    // despite sharing localStorage with page 1's navigation.
    await expect(stripB.getActiveTab()).toHaveAttribute('title', 'Window 1');
    expect(WindowTabStripPage.windowIdFromUrl(page2.url())).toBe(idA);

    await page2.close();
  });

  test("renaming a window in one tab live-updates the other tab's tab strip via the storage listener", async ({
    page,
    context,
  }) => {
    // Suppress the onboarding modal (see the other test's comment above for why).
    await context.addInitScript(() => {
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });

    // Page 1: create a second window so both windows exist in the shared layout.
    await page.goto('/');
    const stripA = new WindowTabStripPage(page);
    await stripA.waitForLoaded();
    await expect(stripA.getTab('Window 1')).toBeVisible();
    await stripA.createWindow();
    await expect(stripA.tabs).toHaveCount(2);
    await stripA.waitForPersistedWindowCount(2);

    // Page 2: same browser context, independently loads the persisted 2-window layout.
    const page2 = await context.newPage();
    const stripB = new WindowTabStripPage(page2);
    await page2.goto('/');
    await stripB.waitForLoaded();
    await expect(stripB.tabs).toHaveCount(2);
    await expect(stripB.getTab('Window 1')).toBeVisible();

    // When: page 1 renames "Window 1" to "Renamed" in place.
    await stripA.renameTab('Window 1', 'Renamed');
    await expect(stripA.getTab('Renamed')).toBeVisible();

    // Then: page 2's tab strip live-updates the label via the storage event listener —
    // no page2.reload() anywhere in this test.
    await expect(stripB.getTab('Renamed')).toBeVisible();
    await expect(stripB.getTab('Window 1')).toHaveCount(0);

    await page2.close();
  });
});
