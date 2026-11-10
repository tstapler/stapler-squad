// @feature website-demos
// Tour 3: keep many agents manageable: search, group by tag, the board with swimlanes, and
// tagging rules. Needs the `default`, `dashboard` and `automation` scenarios (tags, categories,
// two tagging rules). Nothing is persisted.
import { caption, typeSlowly } from '../recorder';
import { hold } from '../hold';
import { GROUP_BY, ROW as CARD, SEARCH, groupBy } from '../dashboard';
import type { Scene } from '../scene';

const VIEW_BOARD = '[data-testid="session-view-mode-board"]';
const VIEW_LIST = '[data-testid="session-view-mode-list"]';
const BOARD_GROUPING = '[data-testid="board-grouping-select"]';
const TAGGING_TAB = '[data-testid="tagging-rules-tab"]';

export const scene: Scene = {
  name: 'organise-at-scale',
  title: 'Dozens of agents, still in order',
  landing: { path: '/', landmark: CARD },
  scenes: async (page, clip) => {
    await page.goto('/');
    await page.locator(SEARCH).first().waitFor({ timeout: 30_000 });
    await clip.ready();
    await page.mouse.move(1240, 700);
    await hold(page, 800);
    await typeSlowly(page, SEARCH, 'payment');
    await caption(page, 'Search narrows the list as you type');
    await page.locator(SEARCH).first().clear();

    await groupBy(page, 'tag');
    await caption(page, 'Group by tag; a session shows under each of its tags');

    await page.locator(VIEW_BOARD).first().click();
    await page.locator(BOARD_GROUPING).first().waitFor();
    await caption(page, 'The board lays agents out by status');
    await page.locator(VIEW_LIST).first().click();

    await page.goto('/rules');
    await page.locator(TAGGING_TAB).first().click();
    await caption(page, 'Tagging rules sort new sessions for you');
    await hold(page, 600);
  },
  shots: [
    {
      name: 'organise-at-scale-1',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await typeSlowly(page, SEARCH, 'payment');
      },
      annotations: [{ selector: SEARCH, n: 1 }],
    },
    {
      name: 'organise-at-scale-2',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await groupBy(page, 'tag');
      },
      annotations: [{ selector: GROUP_BY, n: 1 }],
    },
    {
      name: 'organise-at-scale-3',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(VIEW_BOARD).first().click();
        await page.locator(BOARD_GROUPING).first().waitFor();
      },
      annotations: [
        { selector: BOARD_GROUPING, n: 1 },
        { selector: VIEW_BOARD, n: 2 },
      ],
    },
    {
      name: 'organise-at-scale-4',
      route: '/rules',
      landmark: TAGGING_TAB,
      prepare: async (page) => {
        await page.locator(TAGGING_TAB).first().click();
      },
      annotations: [{ selector: TAGGING_TAB, n: 1 }],
    },
  ],
};
