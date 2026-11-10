// @feature website-demos
// Tour 2: an agent waits for approval, you decide in the Review Queue, then teach the rules
// so it never asks again, and check the Approval Analytics. Needs the `default` and
// `approvals` scenarios (the pending `git push` terminal and three seeded rules). The custom
// rule form is cancelled, so nothing is persisted.
import { caption, typeSlowly } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const QUEUE = '[data-testid="review-queue"]';
const ADD_RULE = '[data-testid="add-rule-button"]';
const RULE_NAME = '[data-testid="form-name-input"]';
const ANALYTICS_HEADING = 'h2:has-text("Approval Analytics")';

export const scene: Scene = {
  name: 'approval-review',
  title: 'Approve once, automate the rest',
  landing: { path: '/review-queue', landmark: QUEUE },
  quotes: ['Ask before git push', 'Approval Analytics'],
  scenes: async (page, clip) => {
    await page.goto('/review-queue');
    await page.locator(QUEUE).first().waitFor({ timeout: 30_000 });
    await clip.ready();
    await page.mouse.move(1240, 700);
    await hold(page, 800);
    await caption(page, 'Agents that need a decision wait in the Review Queue');

    await page.goto('/rules');
    await page.getByText('Approval Rules').first().waitFor({ timeout: 30_000 });
    await hold(page, 600);
    await caption(page, 'Rules decide the routine commands for you');

    await page.locator(ADD_RULE).first().click();
    await page.locator(RULE_NAME).first().waitFor();
    await typeSlowly(page, RULE_NAME, 'Allow git log', 90);
    await caption(page, 'Add a custom rule for what is always safe');
    await page.getByRole('button', { name: /cancel/i }).first().click(); // never persist demo input

    await page.locator(ANALYTICS_HEADING).first().scrollIntoViewIfNeeded();
    await caption(page, 'Approval Analytics shows what your rules decided');
    await hold(page, 600);
  },
  shots: [
    {
      name: 'approval-review-1',
      route: '/review-queue',
      landmark: QUEUE,
      annotations: [{ selector: '[data-testid^="review-item-"]', n: 1 }],
    },
    {
      name: 'approval-review-2',
      route: '/rules',
      landmark: ADD_RULE,
      annotations: [
        { selector: 'table', n: 1 },
        { selector: ADD_RULE, n: 2 },
      ],
    },
    {
      name: 'approval-review-3',
      route: '/rules',
      landmark: ADD_RULE,
      prepare: async (page) => {
        await page.locator(ADD_RULE).first().click();
        await page.locator(RULE_NAME).first().waitFor();
      },
      annotations: [{ selector: RULE_NAME, n: 1 }],
    },
    {
      name: 'approval-review-4',
      route: '/rules',
      landmark: ANALYTICS_HEADING,
      prepare: async (page) => {
        await page.locator(ANALYTICS_HEADING).first().scrollIntoViewIfNeeded();
      },
      annotations: [{ selector: ANALYTICS_HEADING, n: 1 }],
    },
  ],
};
