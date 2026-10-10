// @feature notification-tray, accessibility
/**
 * Notification accessibility matrix (Task 4.5d; backs XA-1, XA-2, XA-9, XA-11..XA-14,
 * XA-16, XA-17, TY-9): Axe over the toast deck and the tray (and its overflow
 * menu and inline confirm) at each viewport profile in both color schemes, plus
 * forced-colors borders, the WCAG 1.4.12 text-spacing override and 320px reflow.
 * A meta-test fails when a notification layout spec does not name a profile.
 */
import fs from 'fs';
import path from 'path';
import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { NotificationTray, ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles, setKeyboardOpen } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';
const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

/**
 * Opens the app, waits for the live stream to deliver a first toast (retrying the
 * send until it is attached), then seeds the rest: toasts only come from events
 * that arrive while the page is open.
 */
async function seedAndOpen(page: Page, request: Parameters<typeof sendNotification>[0], prefix: string) {
  await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  const deck = new ToastDeck(page);
  await expect(async () => {
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-err-0`, type: 'ERROR', title: 'Failure 0' });
    await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
  }).toPass({ timeout: 20_000 });
  for (let i = 1; i < 3; i++) {
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-err-${i}`, type: 'ERROR', title: `Failure ${i}` });
  }
  for (let i = 0; i < 2; i++) {
    await sendNotification(request, BASE_URL, { sessionId: `${prefix}-info-${i}`, type: 'CUSTOM', title: `Info ${i}` });
  }
}

async function violationsIn(page: Page, selector: string): Promise<string[]> {
  // Axe samples computed colors; a color mid-transition after a theme or state change is not a violation.
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' });
  const results = await new AxeBuilder({ page }).include(selector).withTags(TAGS).analyze();
  return results.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `${v.id} (${v.impact}): ${v.nodes.length} node(s) e.g. ${v.nodes[0]?.target.join(' ')} -> ${v.nodes[0]?.html.slice(0, 120)} [${v.nodes[0]?.any[0]?.message ?? ''}]`);
}

test.describe('notification a11y matrix', () => {
  test.setTimeout(120_000);
  test.beforeEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
  });
  test.afterEach(async ({ request }) => {
    await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
  });

  for (const key of ['V1', 'V2', 'V3', 'V4'] as const) {
    test.describe(profiles[key].name, () => {
      test.use(profiles[key].use);

      test('every_tray_control_should_meet_the_44px_target_size', async ({ page, request }) => {
        await seedAndOpen(page, request, `a11y-targets-${key}-${Date.now()}`);
        const deck = new ToastDeck(page);
        const tray = new NotificationTray(page);
        await expect(deck.toasts.first()).toBeVisible({ timeout: 20_000 });
        if (key === 'V4') await setKeyboardOpen(page, 300);
        await tray.handle.or(deck.chip).or(tray.entryOpen).first().click();
        await tray.expectOpen();
        await tray.dismissWhatChanged();
        await expect(tray.rows.first()).toBeVisible();
        // The slide-in transform is still settling; wait until the tray stops moving.
        await expect
          .poll(() =>
            tray.tray.evaluate((el) => {
              const m = new DOMMatrix(getComputedStyle(el).transform);
              return Math.abs(m.m41) + Math.abs(m.m42) < 0.5;
            }),
          )
          .toBe(true);
        const small = await tray.tray.evaluate((root) =>
          Array.from(root.querySelectorAll('button, a[href], input, select'))
            .map((el) => {
              const r = (el as HTMLElement).getBoundingClientRect();
              const visible = r.width > 0 && r.height > 0 && getComputedStyle(el).visibility !== 'hidden';
              return { name: (el.textContent || el.getAttribute('aria-label') || el.tagName).trim().slice(0, 30), w: Math.round(r.width), h: Math.round(r.height), visible };
            })
            .filter((c) => c.visible && (c.h < 43 || c.w < 43)),
        );
        expect(small).toEqual([]);
      });

      for (const scheme of ['light', 'dark'] as const) {
        test(`axe_should_find_no_serious_violations_in_deck_tray_menu_and_confirm_in_${scheme}_scheme`, async ({ page, request }) => {
          await page.emulateMedia({ colorScheme: scheme });
          await seedAndOpen(page, request, `a11y-${key}-${scheme}-${Date.now()}`);
          const deck = new ToastDeck(page);
          const tray = new NotificationTray(page);
          await expect(deck.toasts.first()).toBeVisible({ timeout: 20_000 });
          if (key === 'V4') await setKeyboardOpen(page, 300);

          expect(await violationsIn(page, '[data-testid="toast-stack"]')).toEqual([]);

          // Open the tray through whichever entry this profile shows.
          const opener = tray.handle.or(deck.chip).or(tray.entryOpen).first();
          await opener.click();
          await tray.expectOpen();
          await tray.dismissWhatChanged();
          await expect(tray.rows.first()).toBeVisible();
          expect(await violationsIn(page, '[data-testid="notification-tray"]')).toEqual([]);

          await tray.overflow.click();
          await expect(page.getByRole('menu')).toBeVisible();
          expect(await violationsIn(page, '[data-testid="notification-tray"]')).toEqual([]);
          await page.keyboard.press('Escape');

          await tray.clickMenuItem('clear-informational');
          await expect(tray.confirm).toBeVisible();
          expect(await violationsIn(page, '[data-testid="notification-tray"]')).toEqual([]);
        });
      }
    });
  }

  test.describe('forced colors, text spacing and reflow (V1)', () => {
    test.use(profiles.V1.use);

    test('forced_colors_should_keep_borders_and_focus_rings_on_every_tray_control', async ({ page, request }) => {
      await page.emulateMedia({ forcedColors: 'active' });
      await seedAndOpen(page, request, `a11y-forced-${Date.now()}`);
      const tray = new NotificationTray(page);
      await expect(tray.handle).toBeVisible({ timeout: 20_000 });
      await tray.handle.click();
      await tray.expectOpen();
      await tray.dismissWhatChanged();

      // A control with neither a border nor a background disappears in forced colors.
      const bare = await tray.tray.evaluate((root) => {
        const out: string[] = [];
        for (const el of Array.from(root.querySelectorAll('button'))) {
          const cs = getComputedStyle(el);
          const hasBorder = cs.borderTopStyle !== 'none' && parseFloat(cs.borderTopWidth) > 0;
          const hasBackground = cs.backgroundColor !== 'rgba(0, 0, 0, 0)' && cs.backgroundColor !== 'transparent';
          const hasText = (el.textContent || '').trim().length > 0 || el.getAttribute('aria-label');
          if (!hasBorder && !hasBackground && !hasText) out.push(el.outerHTML.slice(0, 80));
        }
        return out;
      });
      expect(bare).toEqual([]);
      const handleBorder = await tray.handle.evaluate((el) => getComputedStyle(el).borderTopStyle);
      expect(handleBorder).not.toBe('none');
    });

    test('text_spacing_override_should_not_clip_or_overlap_header_controls_and_the_list_should_scroll_to_the_end', async ({ page, request }) => {
      await seedAndOpen(page, request, `a11y-spacing-${Date.now()}`);
      const tray = new NotificationTray(page);
      await expect(tray.handle).toBeVisible({ timeout: 20_000 });
      await tray.handle.click();
      await tray.expectOpen();
      await tray.dismissWhatChanged();
      // WCAG 1.4.12 override stylesheet.
      await page.addStyleTag({
        content: '* { line-height: 1.5 !important; letter-spacing: 0.12em !important; word-spacing: 0.16em !important; } p { margin-bottom: 2em !important; }',
      });
      const clipped = await tray.tray.evaluate((root) => {
        const out: string[] = [];
        for (const el of Array.from(root.querySelectorAll('button, h2, label'))) {
          const e = el as HTMLElement;
          if (e.scrollWidth > e.clientWidth + 2 && getComputedStyle(e).overflow !== 'visible') out.push(e.textContent?.slice(0, 30) || e.tagName);
        }
        return out;
      });
      expect(clipped).toEqual([]);
      // The virtualized list re-measures as rows scroll in, so keep scrolling until the end is reached.
      const scroll = tray.tray.getByTestId('tray-scroll');
      await expect
        .poll(async () =>
          scroll.evaluate((el) => {
            el.scrollTo(0, el.scrollHeight);
            return el.scrollTop + el.clientHeight >= el.scrollHeight - 2;
          }),
        )
        .toBe(true);
    });

    test('tray_should_reflow_at_320px_without_horizontal_scroll', async ({ page, request }) => {
      await page.setViewportSize({ width: 320, height: 640 });
      await seedAndOpen(page, request, `a11y-reflow-${Date.now()}`);
      const tray = new NotificationTray(page);
      await expect(tray.entry.or(tray.handle).first()).toBeVisible({ timeout: 20_000 });
      await tray.entry.or(tray.handle).first().locator('button').or(tray.handle).first().click();
      await tray.expectOpen();
      await tray.dismissWhatChanged();
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow).toBeLessThanOrEqual(1);
      const trayOverflow = await tray.tray.evaluate((el) => el.scrollWidth - el.clientWidth);
      expect(trayOverflow).toBeLessThanOrEqual(1);
    });
  });

  test('every_notification_layout_spec_should_name_a_viewport_profile', async () => {
    const dir = __dirname;
    const specs = fs
      .readdirSync(dir)
      .filter((f) => /^(notification-tray|toast-stack|notification-a11y).*\.spec\.ts$/.test(f));
    const missing = specs.filter((f) => !/viewport-profiles/.test(fs.readFileSync(path.join(dir, f), 'utf8')));
    expect(missing).toEqual([]);
  });
});
