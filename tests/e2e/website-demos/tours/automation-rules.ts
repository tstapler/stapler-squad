// @feature website-demos
// Seeded by the `automation` scenario (route fixtures for tagging rules, workflows and triggers).
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const TRIGGERS = '[data-testid="triggers-panel"]';
const WORKFLOWS_TITLE = 'h1:has-text("Workflows")';
const TAGGING_TAB = '[data-testid="tagging-rules-tab"]';
const TAGGING_PANEL = '[data-testid="tagging-rules-panel"]';

export const scene: Scene = {
  name: 'automation-rules',
  title: 'Let rules do the routine work',
  landing: { path: '/triggers', landmark: TRIGGERS },
  quotes: ['Triggers', 'Workflows', 'Tagging Rules'],
  seeded: 'route',
  scenes: async (page, clip) => {
    await page.goto('/triggers');
    await page.locator(TRIGGERS).waitFor({ timeout: 30_000 });
    await clip.ready();
    await caption(page, 'Triggers start sessions on a schedule or an event');
    await hold(page, 800);

    await page.goto('/workflows');
    await page.locator(WORKFLOWS_TITLE).waitFor();
    await caption(page, 'Workflows define the stages a piece of work moves through');
    await hold(page, 800);

    await page.goto('/rules');
    await page.locator(TAGGING_TAB).click();
    await page.locator(TAGGING_PANEL).waitFor();
    await caption(page, 'Tagging rules label new sessions for you');
    await hold(page, 800);
  },
  shots: [
    { name: 'automation-rules-1', route: '/triggers', landmark: TRIGGERS, annotations: [{ selector: TRIGGERS, n: 1 }] },
    { name: 'automation-rules-2', route: '/workflows', landmark: WORKFLOWS_TITLE },
    {
      name: 'automation-rules-3',
      route: '/rules',
      landmark: TAGGING_TAB,
      annotations: [{ selector: TAGGING_TAB, n: 1 }],
      prepare: async (page) => {
        await page.locator(TAGGING_TAB).click();
        await page.locator(TAGGING_PANEL).waitFor();
      },
    },
  ],
};
