// @feature ui:header-nav, accessibility
/**
 * Task 7.1a: writes the Next-build baselines the Phase 7 specs compare against
 * (fixtures/route-titles.json and fixtures/next-baseline.json). It does nothing unless
 * CAPTURE_NEXT_FIXTURES=1, and must only be run against the Next build:
 *   CAPTURE_NEXT_FIXTURES=1 npx playwright test tools/capture-next-fixtures.spec.ts --project=chromium
 */
import * as fs from 'fs';
import * as path from 'path';
import { test, expect, type Page } from '@playwright/test';
import { ROUTE_META } from '../../../web-app/src/app/route-meta';
import { AppShellPage } from '../pages/AppShellPage';
import { measureNotFound, measureScrollPolicy } from '../pages/BaselineProbes';
import { baseUrl } from '../helpers/base-url';

const FIXTURES = path.join(__dirname, '..', 'fixtures');
const BASE_URL = baseUrl();

const QUIET_MS = 1500;

/** Path and title once neither has changed for QUIET_MS (guarded and auth routes redirect client-side). */
async function settledLocation(page: Page): Promise<{ path: string; title: string }> {
  const read = async () => ({ path: new URL(page.url()).pathname.replace(/(.)\/$/, '$1'), title: await page.title() });
  let last = await read();
  let since = Date.now();
  await expect
    .poll(
      async () => {
        const now = await read();
        if (now.path !== last.path || now.title !== last.title) {
          last = now;
          since = Date.now();
        }
        return Date.now() - since >= QUIET_MS;
      },
      { timeout: 30_000, intervals: [250] },
    )
    .toBe(true);
  return last;
}

test.describe('capture Next baselines', () => {
  test.skip(process.env.CAPTURE_NEXT_FIXTURES !== '1', 'set CAPTURE_NEXT_FIXTURES=1 to (re)capture');
  test.setTimeout(300_000);

  test('route titles and behaviour baselines', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();

    const titles: Record<string, string> = {};
    const redirects: Record<string, string> = {};
    for (const route of Object.keys(ROUTE_META)) {
      await page.goto(`${BASE_URL}${route}`, { waitUntil: 'load' });
      await expect(page.getByRole('main').first()).toBeVisible();
      const settled = await settledLocation(page);
      titles[route] = settled.title;
      if (settled.path !== route) redirects[route] = settled.path;
    }
    fs.writeFileSync(path.join(FIXTURES, 'route-titles.json'), `${JSON.stringify(titles, null, 2)}\n`);

    await page.goto(`${BASE_URL}/history`, { waitUntil: 'domcontentloaded' });
    await expect(shell.drawerNav).toBeVisible();
    const tabPresses = await shell.tabToSkipLink();
    const skipLink = { tabPresses, ...(await shell.activateSkipLink()) };

    const ariaCurrent = await shell.navLink(/history/i).getAttribute('aria-current');
    const baseline = {
      capturedFrom: 'next',
      redirects,
      skipLink,
      historyNavAriaCurrent: ariaCurrent,
      notFound: await measureNotFound(page),
      scrollPolicy: await measureScrollPolicy(page),
    };
    fs.writeFileSync(path.join(FIXTURES, 'next-baseline.json'), `${JSON.stringify(baseline, null, 2)}\n`);
  });
});
