// @feature website-demos
// Tour 4: the backlog pipeline from idea to done, seen as a board and as flow metrics. The
// `backlog` feature flag is off by default, so the recorder enables and reads it back first
// (`flags`); `flagsEnabled` mirrors it for the website's flag-note check. Needs the `backlog`
// scenario (five items across stages). The clip starts and ends on /backlog/flow.
import { expect } from '@playwright/test';
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

export const flagsEnabled: string[] = ['backlog'];

const VIEWS_NAV = 'nav[aria-label="Backlog views"]';
const FLOW_MARKER = '[data-testid="flow-as-of"]';

export const scene: Scene = {
  name: 'backlog-pipeline',
  title: 'From idea to done',
  flags: flagsEnabled,
  landing: { path: '/backlog/flow', landmark: FLOW_MARKER },
  scenes: async (page, clip) => {
    await page.goto('/backlog/flow');
    await page.locator(FLOW_MARKER).first().waitFor({ timeout: 30_000 });
    await clip.ready();
    await page.mouse.move(1240, 700);
    await hold(page, 800);
    await caption(page, 'Flow shows how work moves through the pipeline');

    await page.locator(VIEWS_NAV).getByRole('link', { name: 'Board' }).click();
    await page.getByRole('heading', { name: /backlog/i }).first().waitFor();
    await caption(page, 'The board groups items by stage, from idea to done');

    await page.locator(VIEWS_NAV).getByRole('link', { name: 'List' }).click();
    await page.getByText('Cache product search facets').first().waitFor();
    await caption(page, 'The list holds every item, with its priority');

    await page.locator(VIEWS_NAV).getByRole('link', { name: 'Flow' }).click();
    await page.locator(FLOW_MARKER).first().waitFor();
    await expect(page).toHaveURL(/\/backlog\/flow/);
    await hold(page, 800);
  },
  shots: [
    {
      name: 'backlog-pipeline-1',
      route: '/backlog/flow',
      landmark: FLOW_MARKER,
      annotations: [{ selector: FLOW_MARKER, n: 1 }],
    },
    {
      name: 'backlog-pipeline-2',
      route: '/backlog/board',
      landmark: VIEWS_NAV,
      annotations: [{ selector: VIEWS_NAV, n: 1 }],
    },
    {
      name: 'backlog-pipeline-3',
      route: '/backlog',
      landmark: VIEWS_NAV,
      annotations: [{ selector: VIEWS_NAV, n: 1 }],
    },
  ],
};
