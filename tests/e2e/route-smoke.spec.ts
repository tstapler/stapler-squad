// @feature ui:header-nav, accessibility
/**
 * Story 7.14: every route in RouteMeta (key set derived, not hard-coded) is loaded once without
 * and once with a trailing slash. Any page error, console error, failed or non-2xx same-origin
 * request, `Unexpected token <` (an HTML document served for a script) or missing <main> fails.
 * Console errors that Next itself produces are allow-listed in fixtures/route-smoke-allow.json,
 * one entry at a time with an issue link, so the same spec guards the Bun build later.
 */
import { test, expect, type ConsoleMessage } from '@playwright/test';
import { ROUTE_META } from '../../web-app/src/app/route-meta';
import { AppShellPage } from './pages/AppShellPage';
import { disableBacklogFeatureFlag, enableBacklogFeatureFlag } from './pages/BacklogMutations';
import allowList from './fixtures/route-smoke-allow.json';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const ORIGIN = new URL(BASE_URL).origin;

interface AllowEntry {
  /** Route the console error is tolerated on, or "*". */
  route: string;
  /** Substring of the console message. */
  contains: string;
  /** Tracking issue; an entry without one is invalid. */
  issue: string;
}
const ALLOW = allowList as AllowEntry[];

const isAllowed = (route: string, text: string) =>
  ALLOW.some((e) => (e.route === '*' || e.route === route) && text.includes(e.contains));

test.describe('route smoke', () => {
  test.setTimeout(60_000);

  test('every allow-list entry names an issue', () => {
    for (const entry of ALLOW) expect(entry.issue, JSON.stringify(entry)).toMatch(/^https:\/\/github\.com\/.+\/issues\/\d+$/);
  });

  // With the flag off the app still calls BacklogService RPCs that 404, on every route; with it on
  // the flag-guarded routes render their real pages, which is the stronger smoke.
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  for (const route of Object.keys(ROUTE_META)) {
    const variants = route === '/' ? [route] : [route, `${route}/`];
    for (const path of variants) {
      test(`loads ${path} cleanly`, async ({ page }) => {
        const problems: string[] = [];
        page.on('pageerror', (err) => problems.push(`pageerror: ${err.message}`));
        page.on('console', (msg: ConsoleMessage) => {
          if (msg.type() !== 'error') return;
          const text = msg.text();
          if (!isAllowed(route, text)) problems.push(`console.error: ${text}`);
        });
        page.on('requestfailed', (req) => {
          const failure = req.failure()?.errorText ?? '';
          // Navigations away abort in-flight streams; that is not a failed load.
          if (new URL(req.url()).origin === ORIGIN && !/ABORTED/i.test(failure)) problems.push(`requestfailed: ${req.url()} ${failure}`);
        });
        page.on('response', (res) => {
          if (new URL(res.url()).origin === ORIGIN && res.status() >= 400) problems.push(`HTTP ${res.status()}: ${res.url()}`);
        });

        const shell = new AppShellPage(page);
        await shell.skipOnboarding();
        await page.goto(`${BASE_URL}${path}`, { waitUntil: 'load' });
        await expect(shell.main).toBeVisible();
        // One frame after load so late hydration errors are reported before the verdict.
        await page.evaluate(() => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))));

        expect(problems.filter((p) => !/Unexpected token </.test(p)), `problems on ${path}`).toEqual([]);
        expect(problems.filter((p) => /Unexpected token </.test(p)), `HTML served as script on ${path}`).toEqual([]);
      });
    }
  }
});
