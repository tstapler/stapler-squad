// @feature website-demos
// One still per feature card on the website (website/src/content/feature-media.json maps each
// card heading to the shot name here; website/tests/unit/card-shots.test.ts keeps the two in
// step). A new card needs one entry here and one in feature-media.json, nothing else. Needs
// every tour scenario seeded (record-website-demos.sh --scenario all). Nothing is persisted.
import type { Page } from '@playwright/test';
import type { ShotListSpec, ShotSpec } from '../scene';

const SEARCH = 'input[aria-label="Search sessions"]';
const CARD = '[data-testid="session-card"]';
const GROUP_BY = 'select[aria-label="Group sessions by"]';
const QUEUE = '[data-testid="review-queue"]';
const ADD_RULE = '[data-testid="add-rule-button"]';
const RULE_NAME = '[data-testid="form-name-input"]';
const RULES_PAGE = 'text=Approval Rules';
const ANALYTICS = 'h2:has-text("Approval Analytics")';
const NOTIFICATIONS = '[data-testid="notifications-content"]';
const FANOUT = '[data-testid="fanout-comparison"]';
const SELECTED_TAB = '[role="tab"][aria-selected="true"]';
const REFACTOR = 'refactor-session-store';

/** Opens a seeded session and one of its tabs (terminal, diff, browser). */
const openTab = (tab: RegExp) => async (page: Page) => {
  await page.locator(CARD).filter({ hasText: REFACTOR }).first().click();
  await page.getByRole('tab', { name: tab }).first().click();
  await page.locator(SELECTED_TAB).first().waitFor();
};

interface CardShot {
  /** The card heading in docs/features.md. */
  heading: string;
  spec: ShotSpec;
}

export const CARD_SHOTS: readonly CardShot[] = [
  { heading: 'Dashboard', spec: { name: '01-dashboard', route: '/', landmark: CARD } },
  { heading: 'Live Terminal Streaming', spec: { name: 'live-terminal', route: '/', landmark: CARD, prepare: openTab(/terminal/i) } },
  { heading: 'Browser Element Inspector', spec: { name: 'browser-inspector', route: '/', landmark: CARD, prepare: openTab(/browser/i) } },
  { heading: 'Diff Viewer', spec: { name: 'diff-viewer', route: '/', landmark: CARD, prepare: openTab(/diff/i) } },
  { heading: 'Notifications', spec: { name: 'notifications', route: '/notifications', landmark: NOTIFICATIONS } },
  {
    heading: 'Instant Search',
    spec: {
      name: '02-search-payment',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(SEARCH).first().fill('payment');
      },
    },
  },
  {
    heading: 'Status Filtering',
    spec: {
      name: '03-needs-approval',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.getByRole('checkbox', { name: 'Show only sessions needing approval' }).check();
      },
    },
  },
  {
    heading: 'Tag-Based Grouping',
    spec: {
      name: '04-group-by-tag',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(GROUP_BY).first().selectOption('tag');
      },
    },
  },
  {
    heading: 'Workspace Switcher',
    spec: {
      name: 'workspace-switcher',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator('[aria-label^="Current workspace"]').first().click();
        await page.getByRole('listbox', { name: 'Available workspaces' }).waitFor();
      },
    },
  },
  {
    heading: 'Bulk Actions',
    spec: {
      name: 'bulk-actions',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.getByRole('button', { name: 'Enter select mode' }).click();
        await page.getByRole('toolbar', { name: 'Bulk session actions' }).waitFor();
      },
    },
  },
  { heading: 'Review Queue', spec: { name: '05-review-queue', route: '/review-queue', landmark: QUEUE } },
  { heading: 'Auto-Approval Rules', spec: { name: '06-rules', route: '/rules', landmark: RULES_PAGE } },
  {
    heading: 'Custom Rules',
    spec: {
      name: '07-add-rule',
      route: '/rules',
      landmark: RULES_PAGE,
      prepare: async (page) => {
        await page.locator(ADD_RULE).first().click();
        await page.locator(RULE_NAME).first().fill('Allow git log');
      },
    },
  },
  {
    heading: 'Approval Analytics',
    spec: {
      name: 'approval-analytics',
      route: '/rules',
      landmark: RULES_PAGE,
      prepare: async (page) => {
        await page.locator(ANALYTICS).first().scrollIntoViewIfNeeded();
      },
    },
  },
  { heading: 'History Search', spec: { name: 'history-search', route: '/history', landmark: 'h1:has-text("History")' } },
  { heading: 'Logs Viewer', spec: { name: 'logs-viewer', route: '/logs', landmark: 'h1:has-text("Application Logs")' } },
  { heading: 'Config Viewer', spec: { name: 'config-viewer', route: '/config', landmark: 'h2' } },
  { heading: 'Parallel Fan-Out', spec: { name: 'parallel-fan-out', route: '/fanout?groupId=demo-pricing', landmark: FANOUT } },
  {
    // Each seeded session sits on its own branch in its own worktree; grouping by branch shows that.
    heading: 'Isolated Git Workspaces',
    spec: {
      name: '08-final',
      route: '/',
      landmark: CARD,
      prepare: async (page) => {
        await page.locator(GROUP_BY).first().selectOption('branch');
      },
    },
  },
];

export const shotlist: ShotListSpec = { name: 'cards', shots: CARD_SHOTS.map((c) => c.spec) };
