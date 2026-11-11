// @feature website-demos
// Tour 2: an agent waits for approval, you decide in the Review Queue, then teach the rules
// so it never asks again, and check the Approval Analytics. Needs the `default` and
// `approvals` scenarios (the pending `git push` terminal and three seeded rules). The custom
// rule form is cancelled, so nothing is persisted.
import { caption, typeSlowly } from '../recorder';
import { hold } from '../hold';
import { RULE_NAME, openNewRuleBuilder } from '../rules';
import type { Scene } from '../scene';

const QUEUE = '[data-testid="review-queue"]';
const RULES_TABLE = 'table';
const ANALYTICS_HEADING = 'h2:has-text("Approval Analytics")';
const ANALYTICS_WINDOW = '[role="group"][aria-label="Time window"]'; // annotations use document.querySelector, so no :has-text

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

    await openNewRuleBuilder(page);
    await typeSlowly(page, RULE_NAME, 'Allow git log', 90);
    await caption(page, 'Add a custom rule for what is always safe');
    await page.getByRole('dialog').getByRole('button', { name: /cancel/i }).click(); // never persist demo input

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
      landmark: RULES_TABLE,
      annotations: [{ selector: RULES_TABLE, n: 1 }],
    },
    {
      name: 'approval-review-3',
      route: '/rules',
      landmark: RULES_TABLE,
      prepare: openNewRuleBuilder,
      annotations: [{ selector: RULE_NAME, n: 1 }],
    },
    {
      name: 'approval-review-4',
      route: '/rules',
      landmark: ANALYTICS_HEADING,
      prepare: async (page) => {
        await page.locator(ANALYTICS_HEADING).first().scrollIntoViewIfNeeded();
      },
      annotations: [{ selector: ANALYTICS_WINDOW, n: 1 }],
    },
  ],
};
