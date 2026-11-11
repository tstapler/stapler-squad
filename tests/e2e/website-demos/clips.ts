// @feature website-demos
// Legacy, non-tour demo clips. Tours live one per file in tours/<id>.ts and are found by
// glob (scene.ts discoverScenes); this list only holds clips no tour owns yet. `name` is
// the file name (<name>.webm), mapped to a feature card in website/scripts/build-features.mjs
// (DEMO_CLIP_BY_HEADING); `title` is the title-card text.
import { expect } from '@playwright/test';
import { caption, typeSlowly, type ClipSpec } from './recorder';
import { hold } from './hold';

export type Clip = ClipSpec;

const SEARCH = 'input[aria-label="Search sessions"]';

export const CLIPS: Clip[] = [
  {
    name: 'session-dashboard',
    title: 'Every agent, one dashboard',
    scenes: async (page, clip) => {
      await page.goto('/');
      await page.waitForSelector(SEARCH, { timeout: 30_000 });
      await clip.ready();
      await page.mouse.move(1240, 700); // park the pointer off the content
      await hold(page, 800);
      await caption(page, 'Six agents, grouped by category, in one view');

      await typeSlowly(page, SEARCH, 'payment');
      await hold(page, 700);
      await caption(page, 'Search narrows the list as you type');
      await page.locator(SEARCH).first().clear();

      const groupBy = page.locator('select[aria-label="Group sessions by"]').first();
      if (await groupBy.isVisible({ timeout: 2000 }).catch(() => false)) {
        await groupBy.selectOption('tag');
        await expect(groupBy).toHaveValue('tag');
        await hold(page, 800);
        await caption(page, 'Regroup by tag, branch, path, or status');
        await groupBy.selectOption('category');
      }
      await hold(page, 600);
    },
  },
  {
    name: 'auto-approval-rules',
    title: 'Automate the predictable',
    scenes: async (page, clip) => {
      await page.goto('/rules');
      await page.getByText('Approval Rules').first().waitFor({ timeout: 30_000 });
      await clip.ready();
      await hold(page, 600);
      const builtIn = page.getByRole('button', { name: /built.in/i }).first();
      if (await builtIn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await builtIn.click();
        await hold(page, 600);
      }
      await caption(page, 'Built-in rules approve the safe, routine commands');

      const all = page.getByRole('button', { name: /^all/i }).first();
      if (await all.isVisible({ timeout: 2000 }).catch(() => false)) await all.click();
      const add = page.getByRole('button', { name: /add custom rule/i }).first();
      await add.click();
      await page.locator('[data-testid="form-name-input"]').first().waitFor();
      // data-testid / label locators, not placeholders: the form has been
      // redesigned once already and placeholder text is the first thing to change.
      await typeSlowly(page, '[data-testid="form-name-input"]', 'Allow git log', 90);
      await typeSlowly(page, '[data-testid="form-tool-name-input"]', 'Bash', 90);
      for (const [label, value] of [['Programs', 'git'], ['Subcommands (allow-list)', 'log']]) {
        await typeSlowly(page, `label:has-text("${label}") input:not([type="hidden"])`, value, 90);
        await page.keyboard.press('Enter');
      }
      await caption(page, 'Teach it what is always safe');
      await page.getByRole('button', { name: /cancel/i }).first().click(); // never persist demo input
      await hold(page, 500);
    },
  },
];
