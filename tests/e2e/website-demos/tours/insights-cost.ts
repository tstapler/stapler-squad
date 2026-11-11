// @feature website-demos
// Seeded by the `insights` scenario: Claude JSONL transcripts under <test-dir>/claude-projects, which the server aggregates into spend and findings.
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Page } from '@playwright/test';
import type { Scene } from '../scene';

const FINDINGS = '[data-testid="findings-panel"]';
const ACTIVITY = '[data-testid="activity-breakdown-table"]';
// Annotation selectors run through querySelector, so they must be plain CSS (no :has-text).
const MODEL_VIEW_TOGGLE = '[aria-label="Model over time view"]';
const MODEL_OVER_TIME = 'h2:has-text("Spend by Model Over Time")';

// Vertical wheel scroll only: scrollIntoView would also shift the wide page sideways and hide the nav.
async function scrollDownTo(page: Page, selector: string): Promise<void> {
  const top = await page.locator(selector).first().evaluate((el) => el.getBoundingClientRect().top);
  await page.mouse.move(700, 400);
  await page.mouse.wheel(0, Math.max(0, top - 80));
  await page.waitForFunction((sel) => (document.querySelector(sel)?.getBoundingClientRect().top ?? 1e9) < 200, selector);
}

export const scene: Scene = {
  name: 'insights-cost',
  title: 'See where the spend goes',
  landing: { path: '/insights', landmark: FINDINGS },
  quotes: ['Insights', 'Spend by Model Over Time', 'Waste Findings'],
  seeded: 'db',
  scenes: async (page, clip) => {
    await page.goto('/insights');
    await page.getByRole('heading', { name: 'Insights', level: 1 }).waitFor({ timeout: 30_000 });
    await clip.ready();
    await caption(page, 'Daily spend and totals for the last week');
    await hold(page, 800);

    await page.getByRole('heading', { name: 'Spend by Model Over Time' }).scrollIntoViewIfNeeded();
    await caption(page, 'Spend by model, over time');
    await hold(page, 800);

    await page.getByRole('heading', { name: 'Waste Findings' }).scrollIntoViewIfNeeded();
    await caption(page, 'Findings point at where agents wasted time or money');
    await hold(page, 800);

    await page.locator(ACTIVITY).first().scrollIntoViewIfNeeded();
    await caption(page, 'Break activity down by session');
    await hold(page, 800);
  },
  shots: [
    { name: 'insights-cost-1', route: '/insights', landmark: 'h1:has-text("Insights")' },
    { name: 'insights-cost-2', route: '/insights', landmark: MODEL_OVER_TIME, prepare: (page) => scrollDownTo(page, MODEL_VIEW_TOGGLE), annotations: [{ selector: MODEL_VIEW_TOGGLE, n: 1 }] },
    { name: 'insights-cost-3', route: '/insights', landmark: FINDINGS, annotations: [{ selector: FINDINGS, n: 1 }] },
    { name: 'insights-cost-4', route: '/insights', landmark: ACTIVITY, prepare: (page) => scrollDownTo(page, ACTIVITY), annotations: [{ selector: ACTIVITY, n: 1 }] },
  ],
};
