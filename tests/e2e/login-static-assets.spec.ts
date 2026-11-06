// @feature auth:passkey-login, ui:static-assets
/**
 * Story 7.13a (lockout guard): with remote auth on and no session, /login must be able to load
 * every static asset it references. A bundler naming change that moves an asset outside the
 * auth-exempt prefixes would redirect that asset to /login and lock every user out.
 *
 * Passkey auth is attached only to the remote HTTPS listener, so this starts a dedicated server
 * with `remoteAuth` and drives it with TLS errors ignored (private CA). The login page's RPC
 * fetches (/api/...) answer 401 by design; every other request is a static asset and must be 2xx
 * with no redirect, and its path must be under an exempt prefix or in
 * server/middleware/testdata/public-allowlist.json (Story 5.9).
 */
import { test, expect, type Page } from '@playwright/test';
import { TestServer } from './helpers/test-server';
import allowlist from '../../server/middleware/testdata/public-allowlist.json';

const API_PREFIX = '/api/';
/** fetch/xhr are data requests (RPCs); everything else is a loaded asset. */
const DATA_TYPES = new Set(['fetch', 'xhr']);
const MIN_ASSETS = 5;
const SERVER_BOOT_TIMEOUT_MS = 180_000;
const TEST_TIMEOUT_MS = 120_000;
/** Always reachable unauthenticated, independent of the allowlist (Story 6.8's exempt-prefix set). */
const EXEMPT_PREFIXES = ['/_next/static/', '/login'];

interface Recorded {
  path: string;
  type: string;
  status: number;
  location: string;
}

const isAsset = (r: Recorded) => !r.path.startsWith(API_PREFIX) && !DATA_TYPES.has(r.type);

/** Drives /login on the given server and returns every 2xx-or-redirect asset it loaded. */
async function collectLoginAssets(page: Page, baseUrl: string): Promise<{ assets: Recorded[]; seen: Recorded[] }> {
  const seen: Recorded[] = [];
  page.on('response', (response) => {
    const url = new URL(response.url());
    seen.push({
      path: url.pathname,
      type: response.request().resourceType(),
      status: response.status(),
      location: response.headers()['location'] ?? '',
    });
  });

  const login = await page.goto(`${baseUrl}/login`, { waitUntil: 'load' });
  expect(login?.status()).toBe(200);
  await expect(page.getByRole('main').first()).toBeVisible();
  // Dynamic-import chunks, fonts and images land after load; wait until the static-asset list is
  // quiet. The page keeps retrying its 401 RPCs, so API requests are not part of "quiet".
  const staticCount = () => seen.filter(isAsset).length;
  let count = -1;
  await expect
    .poll(
      async () => {
        const settled = staticCount() === count;
        count = staticCount();
        return settled;
      },
      { intervals: [750], timeout: 20_000 },
    )
    .toBe(true)
    // A slow host may still be streaming late chunks; assert on what has loaded rather than fail here.
    .catch(() => undefined);

  return { assets: seen.filter((r) => isAsset(r) && r.path !== '/login'), seen };
}

/** Matches an allowlist entry's path, which may end in `/*` for a whole directory. */
function matchesAllowlistEntry(pathname: string, entryPath: string): boolean {
  if (entryPath.endsWith('/*')) return pathname.startsWith(`/${entryPath.slice(0, -2)}/`);
  return pathname === `/${entryPath}`;
}

function isExempt(pathname: string): boolean {
  if (EXEMPT_PREFIXES.some((prefix) => pathname.startsWith(prefix))) return true;
  return allowlist.some((entry) => matchesAllowlistEntry(pathname, entry.path));
}

test.describe('login page static assets (remote auth, no session)', () => {
  test.describe.configure({ mode: 'serial', timeout: TEST_TIMEOUT_MS });

  let server: TestServer;

  test.beforeAll(async () => {
    test.setTimeout(SERVER_BOOT_TIMEOUT_MS);
    server = new TestServer({
      port: 0,
      testDir: `/tmp/stapler-squad-test-login-bun-${process.pid}`,
      seedSessions: 0,
      liveSeedSessions: 0,
      remoteAuth: true,
    });
    await server.start();
  });

  test.afterAll(async () => {
    await server?.stop();
  });

  test('every static asset is 2xx with no redirect, and every requested path is exempt or allowlisted', async ({ browser }) => {
    const context = await browser.newContext({ ignoreHTTPSErrors: true });
    const page = await context.newPage();
    const { assets } = await collectLoginAssets(page, server.getRemoteBaseUrl());

    expect(assets.length, 'the login page loads static assets').toBeGreaterThan(MIN_ASSETS);
    const redirected = assets.filter((r) => r.status >= 300 && r.status < 400);
    expect(redirected.map((r) => `${r.status} ${r.path} -> ${r.location}`), 'assets redirected (to /login)').toEqual([]);
    const notOk = assets.filter((r) => r.status < 200 || r.status >= 300);
    expect(notOk.map((r) => `${r.status} ${r.path}`), 'non-2xx static assets').toEqual([]);

    const notExempt = assets.filter((r) => !isExempt(r.path)).map((r) => r.path);
    expect(notExempt, 'every requested path is under an exempt prefix or the allowlist').toEqual([]);

    await context.close();
  });
});
