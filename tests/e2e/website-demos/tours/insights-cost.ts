// @feature website-demos
// Seeded by the `insights` scenario (route fixture: insights are computed server-side, not seedable over RPC).
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const FINDINGS = '[data-testid="findings-panel"]';
const ACTIVITY = '[data-testid="activity-breakdown-table"]';
const MODEL_OVER_TIME = 'h2:has-text("Spend by Model Over Time")';

export const scene: Scene = {
  name: 'insights-cost',
  title: 'See where the spend goes',
  landing: { path: '/insights', landmark: FINDINGS },
  quotes: ['Insights', 'Spend by Model Over Time', 'Waste Findings'],
  seeded: 'route',
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
    { name: 'insights-cost-2', route: '/insights', landmark: MODEL_OVER_TIME, annotations: [{ selector: MODEL_OVER_TIME, n: 1 }] },
    { name: 'insights-cost-3', route: '/insights', landmark: FINDINGS, annotations: [{ selector: FINDINGS, n: 1 }] },
    { name: 'insights-cost-4', route: '/insights', landmark: ACTIVITY, annotations: [{ selector: ACTIVITY, n: 1 }] },
  ],
};
