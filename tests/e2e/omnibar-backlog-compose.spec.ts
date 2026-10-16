// @feature backlog:compose-widget, backlog:parse-item-intent
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { V1_DESKTOP, V2_PORTRAIT } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || process.env.BASE_URL || 'http://localhost:8544';

async function openCompose(page: import('@playwright/test').Page, typed: string) {
  // Suppress the app-wide onboarding walkthrough; its overlay intercepts clicks.
  await page.addInitScript(() => localStorage.setItem('stapler-squad:onboarded', 'true'));
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
  // "+" in the sessions header on desktop, the bottom-nav "New" tab on phone widths
  // (its button has no accessible name, so match the visible label).
  const isPhone = page.viewportSize()!.width < 768;
  const opener = isPhone
    ? page.getByText('New', { exact: true })
    : page.getByRole('button', { name: 'Create new session' });
  await opener.first().click();
  const input = page.getByRole('combobox', { name: /session source input/i });
  await expect(input).toBeVisible({ timeout: 10000 });
  await input.fill(typed);
  await page.getByText('Create Backlog Item (AI)').click();
  const textarea = page.getByTestId('backlog-compose-textarea');
  await expect(textarea).toBeVisible();
  return textarea;
}

for (const profile of [V1_DESKTOP, V2_PORTRAIT]) {
  test.describe(`Omnibar backlog compose widget — ${profile.name}`, () => {
    test.use(profile.use);

    test('pre-fills typed text, meets touch-target size, and has no WCAG AA violations', async ({ page }) => {
      const textarea = await openCompose(page, 'fix the flaky login test');
      await expect(textarea).toHaveValue('fix the flaky login test');

      // Visible, programmatically associated label.
      await expect(page.getByLabel('Describe the work')).toBeVisible();

      // Touch targets >= 44px and nothing overflows the viewport horizontally.
      const widget = page.getByTestId('backlog-compose-widget');
      const widgetBox = await widget.boundingBox();
      const textareaBox = await textarea.boundingBox();
      expect(widgetBox).not.toBeNull();
      expect(textareaBox).not.toBeNull();
      const viewportWidth = page.viewportSize()!.width;
      expect(widgetBox!.x + widgetBox!.width).toBeLessThanOrEqual(viewportWidth);
      // Textarea fills the widget's content width (widget padding is the only slack).
      expect(textareaBox!.width).toBeGreaterThan(widgetBox!.width * 0.8);

      for (const id of ['backlog-compose-attach', 'backlog-compose-cancel', 'backlog-compose-submit']) {
        const box = await page.getByTestId(id).boundingBox();
        expect(box, id).not.toBeNull();
        expect(box!.height, `${id} height`).toBeGreaterThanOrEqual(44);
        expect(box!.width, `${id} width`).toBeGreaterThanOrEqual(44);
      }

      // Native file picker input accepts images only.
      await expect(page.getByTestId('backlog-compose-attach-input')).toHaveAttribute('accept', 'image/*');

      const results = await new AxeBuilder({ page })
        .include('[data-testid="backlog-compose-widget"]')
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa'])
        .analyze();
      expect(
        results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => `${n.html.slice(0, 120)} — ${n.any[0]?.message ?? ''}`).join(' | ')}`),
      ).toEqual([]);
    });

    test('submit button is reachable by keyboard', async ({ page }) => {
      const textarea = await openCompose(page, 'keyboard path');
      await textarea.focus();
      // Tab order inside the widget: Attach image -> Cancel -> Create.
      await page.keyboard.press('Tab');
      await expect(page.getByTestId('backlog-compose-attach')).toBeFocused();
      await page.keyboard.press('Tab');
      await expect(page.getByTestId('backlog-compose-cancel')).toBeFocused();
      await page.keyboard.press('Tab');
      await expect(page.getByTestId('backlog-compose-submit')).toBeFocused();
    });
  });
}
