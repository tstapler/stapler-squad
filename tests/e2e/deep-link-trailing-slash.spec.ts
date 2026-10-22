// @feature session:deep-link, notification:push-click-handoff, backlog:item-detail
/**
 * Story 7.4a/7.4b: links that outsiders hold (notifications, Slack, bookmarks) keep working with
 * and without a trailing slash and with query strings. Runs unchanged on Next and Bun.
 * Next baseline recorded here: `?window=` is stamped on `/` only (useWindowUrlSync), so a client
 * navigation to /history drops it and returning home stamps a window id again.
 */
import { test, expect } from '@playwright/test';
import { AppShellPage } from './pages/AppShellPage';
import { createBacklogItemDirect, disableBacklogFeatureFlag, enableBacklogFeatureFlag } from './pages/BacklogMutations';
import { SessionClient } from './helpers/session-client';
import titles from './fixtures/route-titles.json';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const TITLES = titles as Record<string, string>;
const WINDOW_PARAM = /[?&]window=([^&]+)/;

test.describe('deep links and trailing slashes', () => {
  test.beforeEach(async ({ page }) => {
    await new AppShellPage(page).skipOnboarding();
  });

  for (const [url, route] of [
    ['/rules', '/rules'],
    ['/rules/', '/rules'],
    ['/rules?prefill=x', '/rules'],
    ['/rules/?prefill=x', '/rules'],
    ['/settings/jules/', '/settings/jules'],
  ]) {
    test(`hard load of ${url} shows the right page`, async ({ page }) => {
      await page.goto(`${BASE_URL}${url}`, { waitUntil: 'domcontentloaded' });
      await expect(page).toHaveTitle(TITLES[route]);
      await expect(page.getByRole('main').first()).toBeVisible();
    });
  }

  test('/rules keeps its canonical URL (no trailing slash added)', async ({ page }) => {
    await page.goto(`${BASE_URL}/rules`, { waitUntil: 'domcontentloaded' });
    await expect(page).toHaveTitle(TITLES['/rules']);
    expect(page.url()).toBe(`${BASE_URL}/rules`);
  });

  test('a notification link opens the session pane', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const session = await client.createSession({ title: `deep-link-${Date.now()}`, path: '/tmp', program: 'bash' });
    try {
      await page.goto(`${BASE_URL}/?session=${session.id}&tab=terminal&notification=n1`, { waitUntil: 'domcontentloaded' });
      await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached({ timeout: 20_000 });
      expect(page.url()).toContain(`session=${session.id}`);
    } finally {
      await client.deleteSession(session.id, true);
    }
  });

  test.describe('backlog link', () => {
    test.beforeAll(async ({ request }) => {
      await enableBacklogFeatureFlag(request);
    });
    test.afterAll(async ({ request }) => {
      await disableBacklogFeatureFlag(request);
    });

    test('/backlog?item=ID opens the backlog item', async ({ page, request }) => {
      const itemId = await createBacklogItemDirect(request, { title: `deep-link-item-${Date.now()}` });
      await page.addInitScript(() => localStorage.setItem('stapler-squad:backlog-onboarded', 'true'));
      await page.goto(`${BASE_URL}/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });
      await expect(page.getByTestId('backlog-item-id')).toBeVisible();
    });
  });

  test('a push-click handoff keeps both query params', async ({ page }) => {
    await page.goto(`${BASE_URL}/`, { waitUntil: 'domcontentloaded' });
    await expect(page).toHaveURL(WINDOW_PARAM);
    const target = '/?session=abc&window=w1';
    await page.evaluate((url) => {
      navigator.serviceWorker.dispatchEvent(new MessageEvent('message', { data: { type: 'notification-click', url } }));
    }, target);
    await expect(page).toHaveURL(/session=abc/);
    await expect(page).toHaveURL(/window=/);
  });

  test('?window= is stamped on the home page, dropped on /history, stamped again on return', async ({ page }) => {
    const shell = new AppShellPage(page);
    await page.setViewportSize({ width: 1400, height: 900 });
    await shell.goto('/');
    await expect(page).toHaveURL(WINDOW_PARAM);

    await expect(shell.drawerNav).toBeVisible();
    await shell.navLink(/history/i).click({ force: true });
    await expect(page).toHaveURL(/\/history/);
    expect(page.url()).not.toMatch(WINDOW_PARAM);

    await shell.navLink(/sessions/i).click({ force: true });
    // Next re-stamps a window id on return; it is not guaranteed to be the one it left with.
    await expect(page).toHaveURL(WINDOW_PARAM);
  });

  test('an Unauthenticated RPC triggers a full-document navigation to /login', async ({ page }) => {
    await page.route('**/session.v1.SessionService/ListSessions', (route) =>
      route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'unauthenticated', message: 'not signed in' }),
      }),
    );
    const loginDocument = page.waitForRequest(
      (req) => new URL(req.url()).pathname === '/login' && req.resourceType() === 'document',
    );
    await page.goto(`${BASE_URL}/`, { waitUntil: 'domcontentloaded' });
    const request = await loginDocument;
    const response = await request.response();
    expect(response?.status()).toBe(200);
  });
});
