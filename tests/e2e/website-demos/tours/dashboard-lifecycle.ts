// @feature website-demos
// Tour 1 (hero): the whole session lifecycle on one dashboard. Needs the `default`, `dashboard`
// and `approvals` scenarios seeded (record-website-demos.sh --scenario all). Nothing is
// persisted: the omnibar is opened and dismissed without creating a session.
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const SEARCH = 'input[aria-label="Search sessions"]';
const CARD = '[data-testid="session-card"]';
const NEW_SESSION = 'button[aria-label="Create new session (⌘K)"]';
const OMNIBAR = '[data-testid="omnibar-modal"]';
const SOURCE_INPUT = '[role="combobox"][aria-label="Session source input"]';
const SELECTED_TAB = '[role="tab"][aria-selected="true"]';

export const scene: Scene = {
  name: 'dashboard-lifecycle',
  title: 'Every agent, one dashboard',
  landing: { path: '/', landmark: CARD },
  quotes: ['checkout-retry-logic'],
  scenes: async (page, clip) => {
    await page.goto('/');
    await page.locator(CARD).first().waitFor({ timeout: 30_000 });
    await clip.ready();
    await page.mouse.move(1240, 700); // park the pointer off the content
    await hold(page, 800);
    await caption(page, 'Every agent you run, in one list');

    await page.locator(NEW_SESSION).first().click();
    await page.locator(OMNIBAR).waitFor();
    await caption(page, 'Start a new agent from the omnibar');
    await page.keyboard.press('Escape');
    await page.locator(OMNIBAR).waitFor({ state: 'hidden' });

    await page.locator(CARD).filter({ hasText: 'refactor-session-store' }).first().click();
    await page.getByRole('tab', { name: /terminal/i }).first().click();
    await caption(page, 'Watch the live terminal of any running agent');

    await page.getByRole('tab', { name: /diff/i }).first().click();
    await caption(page, 'Review its changes, then pause or resume it');
    await hold(page, 600);
  },
  shots: [
    {
      name: 'dashboard-lifecycle-1',
      route: '/',
      landmark: CARD,
      annotations: [
        { selector: CARD, n: 1 },
        { selector: SEARCH, n: 2 },
      ],
    },
    {
      name: 'dashboard-lifecycle-2',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(NEW_SESSION).first().click();
        await page.locator(OMNIBAR).waitFor();
      },
      annotations: [{ selector: SOURCE_INPUT, n: 1 }],
    },
    {
      name: 'dashboard-lifecycle-3',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(CARD).filter({ hasText: 'refactor-session-store' }).first().click();
        await page.getByRole('tab', { name: /terminal/i }).first().click();
        await page.locator(SELECTED_TAB).first().waitFor();
      },
      annotations: [{ selector: SELECTED_TAB, n: 1 }],
    },
    {
      name: 'dashboard-lifecycle-4',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(CARD).filter({ hasText: 'checkout-retry-logic' }).first().click();
        await page.getByRole('tab', { name: /diff/i }).first().click();
        await page.locator(SELECTED_TAB).first().waitFor();
      },
      annotations: [{ selector: SELECTED_TAB, n: 1 }],
    },
  ],
};
