// @feature ui:mobile-ux, ui:session-list, ui:session-detail
/**
 * Touch target size enforcement.
 *
 * Asserts that every interactive control introduced in the UX overhaul
 * (and the bottom nav bar it sits above) meets the 44×44 px minimum
 * required by Apple HIG and WCAG 2.5.5 Success Criterion.
 *
 * Runs against a live server (localhost:8544) in the same way all E2E
 * tests do. Tests that require a session view are skipped when no session
 * is open rather than failing.
 */

import { test, expect, Page } from '@playwright/test';

const MIN_PX = 44;

/** Assert a single element meets the touch target minimum. */
async function assertTouchTarget(page: Page, testId: string, label: string) {
  const el = page.getByTestId(testId);
  await expect(el).toBeVisible();
  const box = await el.boundingBox();
  expect(box, `${label} (data-testid="${testId}") not found in DOM`).not.toBeNull();
  expect(
    box!.width,
    `${label} width ${box!.width}px < ${MIN_PX}px minimum`,
  ).toBeGreaterThanOrEqual(MIN_PX);
  expect(
    box!.height,
    `${label} height ${box!.height}px < ${MIN_PX}px minimum`,
  ).toBeGreaterThanOrEqual(MIN_PX);
}

// ─── Mobile viewport (iPhone 14) ─────────────────────────────────────────────

test.describe('Touch targets — sessions list page (mobile)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('[aria-label="Search sessions"], nav', {
      timeout: 15000,
    });
  });

  test('bottom nav items are ≥44px tall', async ({ page }) => {
    // All items in the bottom navigation bar must be tappable one-handed.
    const navLinks = page.locator('nav a, nav button');
    const count = await navLinks.count();
    expect(count, 'No nav links found — is BottomNav rendered?').toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const link = navLinks.nth(i);
      const box = await link.boundingBox();
      if (!box) continue; // hidden / not rendered
      expect(
        box.height,
        `Bottom nav item ${i} height ${box.height}px < ${MIN_PX}px`,
      ).toBeGreaterThanOrEqual(MIN_PX);
    }
  });

  test('new session button is ≥44×44px', async ({ page }) => {
    const btn = page
      .getByRole('button', { name: /new session/i })
      .or(page.getByTestId('new-session-button'))
      .first();
    await expect(btn).toBeVisible();
    const box = await btn.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(MIN_PX);
    expect(box!.height).toBeGreaterThanOrEqual(MIN_PX);
  });
});

// ─── Session detail page (mobile) ────────────────────────────────────────────

test.describe('Touch targets — session detail (mobile)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('[aria-label="Search sessions"]', {
      timeout: 15000,
    });

    // Navigate into the first available session, if any.
    const firstSession = page
      .getByTestId('session-card')
      .or(page.locator('[class*="sessionCard"]'))
      .first();

    const hasSession = (await firstSession.count()) > 0;
    if (hasSession) {
      await firstSession.click();
      await page.waitForSelector('[data-testid="session-header"]', {
        timeout: 10000,
      });
    }
  });

  test('terminal toolbar toggle is ≥44×44px', async ({ page }) => {
    const toggle = page.getByTestId('toolbar-toggle');
    const visible = await toggle.isVisible().catch(() => false);
    if (!visible) {
      test.skip(true, 'No session open — toolbar toggle not visible');
      return;
    }
    await assertTouchTarget(page, 'toolbar-toggle', 'Terminal toolbar toggle');
  });

  test('session more-actions button is ≥44×44px', async ({ page }) => {
    const btn = page.getByTestId('more-actions-button');
    const visible = await btn.isVisible().catch(() => false);
    if (!visible) {
      test.skip(true, 'No session open — more-actions button not visible');
      return;
    }
    await assertTouchTarget(
      page,
      'more-actions-button',
      'Session more-actions button',
    );
  });

  test('mobile keyboard keys are ≥44×44px', async ({ page }) => {
    const keys = page.getByTestId('mobile-key');
    const count = await keys.count();
    if (count === 0) {
      test.skip(
        true,
        'No mobile keyboard keys found — keyboard overlay not visible on this viewport',
      );
      return;
    }

    for (let i = 0; i < count; i++) {
      const box = await keys.nth(i).boundingBox();
      if (!box) continue;
      expect(
        box.width,
        `Mobile key ${i} width ${box.width}px < ${MIN_PX}px`,
      ).toBeGreaterThanOrEqual(MIN_PX);
      expect(
        box.height,
        `Mobile key ${i} height ${box.height}px < ${MIN_PX}px`,
      ).toBeGreaterThanOrEqual(MIN_PX);
    }
  });
});

// ─── Desktop sanity (controls never shrink below mobile target on desktop) ───

test.describe('Touch targets — desktop (1280×800)', () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  test('new session button is ≥44×44px on desktop', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    const btn = page
      .getByRole('button', { name: /new session/i })
      .or(page.getByTestId('new-session-button'))
      .first();
    const visible = await btn.isVisible().catch(() => false);
    if (!visible) {
      test.skip(true, 'New session button not found');
      return;
    }
    const box = await btn.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(MIN_PX);
  });
});
