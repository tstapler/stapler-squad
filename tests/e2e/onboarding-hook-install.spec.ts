// @feature hooks:status, hooks:install
// Validates the onboarding hook-install step:
//   1. Force the onboarding modal to show (clear the onboarded flag)
//   2. Advance to the final "Enable Claude Code hooks" step
//   3. Confirm the two install toggles render and GetHookStatus loads
//   4. Confirm the user can finish onboarding without installing
//
// Intentionally does NOT click "Install" — that would mutate the runner's real
// ~/.claude/settings.json. GetHookStatus is read-only and safe to exercise.
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

test.describe('onboarding-hook-install', () => {
  test.beforeEach(async ({ page }) => {
    // Ensure the onboarding modal triggers on load.
    await page.addInitScript((key) => {
      try {
        window.localStorage.removeItem(key);
      } catch {
        /* ignore */
      }
    }, ONBOARDED_KEY);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  });

  test('e2e:hooks-install - offers hook install on the final onboarding step', async ({ page }) => {
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: 15000 });

    // Advance to the final step (1→2→3→4→5).
    for (let i = 0; i < 4; i++) {
      await dialog.getByRole('button', { name: 'Next' }).click();
    }

    // The hooks step is shown with both toggles.
    await expect(page.getByRole('heading', { name: 'Enable Claude Code hooks' })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: /Enable rule enforcement/ })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: /Enable notifications/ })).toBeVisible();

    // The user can always finish without installing.
    await page.getByRole('button', { name: 'Get started' }).click();
    await expect(dialog).toBeHidden();
  });
});
