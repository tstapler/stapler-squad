// The website's demo clips. `name` is the file name (<name>.webm) and is mapped to a
// feature card in website/scripts/build-features.mjs (DEMO_CLIP_BY_HEADING); `title`
// is the title-card text. Add a clip here and the deploy
// (.github/workflows/deploy-website.yml) records and ships it in the Pages artifact.
import { caption, typeSlowly, type ClipSpec } from './recorder';

export type Clip = ClipSpec;

const SEARCH = 'input[aria-label="Search sessions"]';

export const CLIPS: Clip[] = [
  {
    name: 'session-dashboard',
    title: 'Every agent, one dashboard',
    scenes: async (page, clip) => {
      await page.goto('/');
      await page.waitForSelector(SEARCH, { timeout: 30_000 });
      clip.ready();
      await page.mouse.move(1240, 700); // park the pointer off the content
      await page.waitForTimeout(800);
      await caption(page, 'Six agents, grouped by category, in one view');

      await typeSlowly(page, SEARCH, 'payment');
      await page.waitForTimeout(700);
      await caption(page, 'Search narrows the list as you type');
      await page.locator(SEARCH).first().clear();

      const groupBy = page.locator('select[aria-label="Group sessions by"]').first();
      if (await groupBy.isVisible({ timeout: 2000 }).catch(() => false)) {
        await groupBy.selectOption('tag');
        await page.waitForTimeout(800);
        await caption(page, 'Regroup by tag, branch, path, or status');
        await groupBy.selectOption('category');
      }
      await page.waitForTimeout(600);
    },
  },
  {
    name: 'auto-approval-rules',
    title: 'Automate the predictable',
    scenes: async (page, clip) => {
      await page.goto('/rules');
      await page.getByText('Approval Rules').first().waitFor({ timeout: 30_000 });
      clip.ready();
      await page.waitForTimeout(600);
      const builtIn = page.getByRole('button', { name: /built.in/i }).first();
      if (await builtIn.isVisible({ timeout: 3000 }).catch(() => false)) {
        await builtIn.click();
        await page.waitForTimeout(600);
      }
      await caption(page, 'Built-in rules approve the safe, routine commands');

      const all = page.getByRole('button', { name: /^all/i }).first();
      if (await all.isVisible({ timeout: 2000 }).catch(() => false)) await all.click();
      const add = page.getByRole('button', { name: /add custom rule/i }).first();
      await add.click();
      await page.waitForTimeout(500);
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
      await page.waitForTimeout(500);
    },
  },
];
