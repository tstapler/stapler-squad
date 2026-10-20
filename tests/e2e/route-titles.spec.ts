// @feature ui:header-nav, accessibility
/**
 * Story 7.1: every page keeps its title across a hard load and a client-side navigation.
 * fixtures/route-titles.json is the settled title captured from the Next build by
 * tools/capture-next-fixtures.spec.ts. Routes that redirect on the test server (flag-guarded
 * /backlog*, /login without auth, /config, /sessions/new) are recorded as their redirect
 * target's title; fixtures/next-baseline.json `redirects` lists them. The backlog flag is pinned
 * off because /settings/backlog-sources only has its own title when the flag is on.
 * Titles use auto-retrying toHaveTitle: on the Bun build React sets the title after navigation.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { disableBacklogFeatureFlag } from './pages/BacklogMutations';
import titles from './fixtures/route-titles.json';
import baseline from './fixtures/next-baseline.json';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const FIXTURE = titles as Record<string, string>;
const REDIRECTS = baseline.redirects as Record<string, string>;
const MIN_NAV_ROUTES = 5;

const normalise = (href: string) => new URL(href, BASE_URL).pathname.replace(/(.)\/$/, '$1');

test.describe('route titles', () => {
  test.beforeAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ page }) => {
    await new AppShellPage(page).skipOnboarding();
  });

  for (const [route, title] of Object.entries(FIXTURE)) {
    test(`hard load of ${route} has the fixture title`, async ({ page }) => {
      await page.goto(`${BASE_URL}${route}`, { waitUntil: 'domcontentloaded' });
      await expect(page).toHaveTitle(title);
    });
  }

  test('client-side click navigation lands on the fixture title for every nav link', async ({ page }) => {
    const shell = new AppShellPage(page);
    await page.setViewportSize({ width: 1400, height: 900 });
    await shell.goto('/review-queue');
    await expect(shell.drawerNav).toBeVisible();

    const links = shell.drawerNav.getByRole('link', { includeHidden: true });
    const currentHrefs = async () =>
      (await links.evaluateAll((all) => all.map((l) => l.getAttribute('href') ?? ''))).map(normalise);
    const reachable = (await currentHrefs()).filter((h) => h in FIXTURE);
    expect(reachable.length).toBeGreaterThan(MIN_NAV_ROUTES);

    for (const href of reachable) {
      // The nav re-renders as feature flags load, so resolve the index fresh for every click.
      await links.nth((await currentHrefs()).indexOf(href)).click({ force: true });
      await expect.poll(() => normalise(page.url())).toBe(REDIRECTS[href] ?? href);
      await expect(page, `title after clicking ${href}`).toHaveTitle(FIXTURE[href]);
    }
  });
});
