// @feature ui:lazy-loading
/**
 * Story 7.2c (PR 7b, Bun only; Story 2.7's guard). A route chunk 404ing after a deploy must not
 * drop unsaved input: web-app/src/lib/lazy/stale-chunk.ts reloads once per build unless the user
 * has unsaved text, in which case it shows a banner instead. The Next build has no equivalent
 * lazy-chunk recovery path (its own chunk 404 is a hard navigation failure), so this spec is
 * Bun-only.
 */
import { test, expect } from '@playwright/test';
import { isBunBuild } from './helpers/web-build';
import { AppShellPage } from './pages/AppShellPage';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const BANNER_TEXT = 'A new version is available. Reload when ready.';

/** The page's own chunk is named page-<hash>.js; shared/vendor chunks are index-<hash>.js, never matched. */
async function fail404NextPageChunk(page: import('@playwright/test').Page) {
  await page.route('**/_next/static/chunks/page-*.js', (route) => route.fulfill({ status: 404, body: 'not found' }));
}

test.describe('stale chunk recovery (Bun only)', () => {
  test.skip(!isBunBuild(), 'recoverFromChunkError only exists on the Bun build (web-app/src/lib/lazy/impl.bun.tsx)');

  test('unsaved omnibar text survives a 404 chunk: banner shown, no reload', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.evaluate(() => {
      (window as unknown as { __marker: string }).__marker = 'alive';
    });

    await page.getByRole('button', { name: 'Create new session' }).first().click();
    const input = page.getByRole('combobox', { name: /session source input/i });
    await expect(input).toBeVisible();
    await input.fill('/tmp/unsaved-session-path');

    await fail404NextPageChunk(page);
    // The omnibar's full-screen backdrop sits above the nav link, so a real pointer click would hit
    // the backdrop instead; call the DOM click() method directly so the link's handler still fires.
    await page.evaluate(() => (document.querySelector<HTMLAnchorElement>('[data-testid=drawer-nav] a[href="/rules"]'))?.click());

    await expect(page.getByRole('status').filter({ hasText: BANNER_TEXT })).toBeVisible();
    // No reload: the marker set before navigation and the typed text are both intact.
    expect(await page.evaluate(() => (window as unknown as { __marker?: string }).__marker)).toBe('alive');
    await expect(input).toHaveValue('/tmp/unsaved-session-path');
  });

  test('a 404 chunk with no unsaved input reloads exactly once', async ({ page }) => {
    const shell = new AppShellPage(page);
    await shell.skipOnboarding();
    await page.goto(`${BASE_URL}/logs`, { waitUntil: 'domcontentloaded' });
    await expect(shell.drawerNav).toBeVisible();

    // 'load' fires only on a real page load, never on the same-document SPA route change that
    // triggers the 404, so it isolates location.reload() calls from that first click.
    let reloads = 0;
    page.on('load', () => reloads++);
    await fail404NextPageChunk(page);
    await shell.navLink(/rules/i).click({ force: true });

    // The reload re-requests the real (unmocked) chunk and lands on the target route.
    await expect(page).toHaveURL(/\/rules/);
    await expect(shell.drawerNav).toBeVisible();
    expect(reloads, 'exactly one reload, not a loop').toBe(1);
  });
});
