// @feature website-demos
// Seeded by the `fanout` scenario (tests/demo/scenario/fanout.go): three variants of one prompt.
import { expect } from '@playwright/test';
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const ROUTE = '/fanout?groupId=demo-pricing';
const COMPARISON = '[data-testid="fanout-comparison"]';
const variant = (name: string) => `section[aria-label="${name}"]`;

export const scene: Scene = {
  name: 'fanout-compare',
  title: 'Try three approaches, keep the best',
  landing: { path: '/fanout', landmark: COMPARISON },
  quotes: ['Compare variants', 'Pick as winner'],
  seeded: 'db',
  scenes: async (page, clip) => {
    await page.goto(ROUTE);
    await expect(page.locator('[data-testid^="fanout-column-"]')).toHaveCount(3, { timeout: 30_000 });
    await clip.ready();
    await caption(page, 'One prompt, three variants, each in its own worktree');
    await hold(page, 800);

    await page.locator(variant('pricing-cache-variant-c')).scrollIntoViewIfNeeded();
    await caption(page, 'Read the diffs side by side');
    await hold(page, 800);

    const pick = page.getByRole('region', { name: 'pricing-cache-variant-b' }).getByRole('button', { name: /pick as winner/i });
    await pick.click();
    await expect(page.getByRole('button', { name: /confirm: merge this variant/i })).toBeVisible();
    await caption(page, 'Pick a winner; nothing merges until you confirm');
    await hold(page, 800); // never confirm: the demo must not merge anything
  },
  shots: [
    {
      name: 'fanout-compare-1',
      route: ROUTE,
      landmark: COMPARISON,
      annotations: [
        { selector: variant('pricing-cache-variant-a'), n: 1 },
        { selector: variant('pricing-cache-variant-b'), n: 2 },
        { selector: variant('pricing-cache-variant-c'), n: 3 },
      ],
    },
    { name: 'fanout-compare-2', route: ROUTE, landmark: variant('pricing-cache-variant-c') },
    {
      name: 'fanout-compare-3',
      route: ROUTE,
      landmark: COMPARISON,
      prepare: async (page) => {
        await page.getByRole('region', { name: 'pricing-cache-variant-b' }).getByRole('button', { name: /pick as winner/i }).click();
        await expect(page.getByRole('button', { name: /confirm: merge this variant/i })).toBeVisible();
      },
    },
  ],
};
