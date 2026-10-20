// @feature auth:passkey-login, ui:static-assets
/**
 * Story 7.13a (lockout guard): with remote auth on and no session, /login must be able to load
 * every static asset it references. A bundler naming change that moves an asset outside the
 * auth-exempt prefixes would redirect that asset to /login and lock every user out.
 *
 * Passkey auth is attached only to the remote HTTPS listener, so this starts a dedicated server
 * with `remoteAuth` and drives it with TLS errors ignored (private CA). Observed on Next: the
 * login page's RPC fetches (/api/...) answer 401 by design, and Next's <Link> RSC prefetches
 * (<route>/index.txt, fetch type) answer 302 -> /login; neither is an asset. Every other request is
 * a static asset and must be 2xx with no redirect.
 *
 * Set RECORD_LOGIN_REQUESTS=1 to rewrite server/middleware/testdata/login-requests.json (input
 * for the Go allowlist walk, Story 6.8).
 */
import * as fs from 'fs';
import * as path from 'path';
import { test, expect } from '@playwright/test';
import { TestServer } from './helpers/test-server';

const REQUESTS_FIXTURE = path.join(__dirname, '..', '..', 'server', 'middleware', 'testdata', 'login-requests.json');
const API_PREFIX = '/api/';
/** fetch/xhr are data requests (RPCs, Next RSC prefetches); everything else is a loaded asset. */
const DATA_TYPES = new Set(['fetch', 'xhr']);
const MIN_ASSETS = 5;
const SERVER_BOOT_TIMEOUT_MS = 180_000;
const TEST_TIMEOUT_MS = 120_000;

interface Recorded {
  path: string;
  type: string;
  status: number;
  location: string;
}

test.describe('login page static assets (remote auth, no session)', () => {
  test.describe.configure({ mode: 'serial', timeout: TEST_TIMEOUT_MS });

  let server: TestServer;

  test.beforeAll(async () => {
    test.setTimeout(SERVER_BOOT_TIMEOUT_MS); // a cold test-mode boot has taken 30-45s
    server = new TestServer({
      port: 0,
      testDir: `/tmp/stapler-squad-test-login-${process.pid}`,
      seedSessions: 0,
      liveSeedSessions: 0,
      remoteAuth: true,
    });
    await server.start();
  });

  test.afterAll(async () => {
    await server?.stop();
  });

  test('every static asset /login needs loads 2xx with no redirect to /login', async ({ browser }) => {
    const context = await browser.newContext({ ignoreHTTPSErrors: true });
    const page = await context.newPage();
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

    const login = await page.goto(`${server.getRemoteBaseUrl()}/login`, { waitUntil: 'load' });
    expect(login?.status()).toBe(200);
    await expect(page.getByRole('main').first()).toBeVisible();
    // Dynamic-import chunks, fonts and images land after load; wait until the static-asset list is
    // quiet. The page keeps retrying its 401 RPCs, so API requests are not part of "quiet".
    const isAsset = (r: Recorded) => !r.path.startsWith(API_PREFIX) && !DATA_TYPES.has(r.type);
    const staticCount = () => seen.filter(isAsset).length;
    let count = -1;
    await expect
      .poll(async () => {
        const settled = staticCount() === count;
        count = staticCount();
        return settled;
      }, { intervals: [750], timeout: 20_000 })
      .toBe(true)
      // A slow host may still be streaming late chunks; assert on what has loaded rather than fail here.
      .catch(() => undefined);

    const assets = seen.filter((r) => isAsset(r) && r.path !== '/login');
    expect(assets.length, 'the login page loads static assets').toBeGreaterThan(MIN_ASSETS);

    const redirected = assets.filter((r) => r.status >= 300 && r.status < 400);
    expect(redirected.map((r) => `${r.status} ${r.path} -> ${r.location}`), 'assets redirected (to /login)').toEqual([]);
    const notOk = assets.filter((r) => r.status < 200 || r.status >= 300);
    expect(notOk.map((r) => `${r.status} ${r.path}`), 'non-2xx static assets').toEqual([]);

    // Next's <Link> prefetches each route's RSC payload (<route>/index.txt, type fetch). With no
    // session those answer 302 -> /login. They are not assets the login page needs, so they are
    // recorded here instead of failing the guard.
    const prefetches = seen.filter((r) => !r.path.startsWith(API_PREFIX) && DATA_TYPES.has(r.type));
    test.info().annotations.push({
      type: 'next-rsc-prefetch',
      description: `${prefetches.length} non-API fetches; ${prefetches.filter((r) => r.status === 302).length} redirected to /login`,
    });

    if (process.env.RECORD_LOGIN_REQUESTS === '1') {
      const paths = [...new Set(assets.map((r) => `${r.type} ${r.path}`))].sort();
      fs.mkdirSync(path.dirname(REQUESTS_FIXTURE), { recursive: true });
      fs.writeFileSync(REQUESTS_FIXTURE, `${JSON.stringify({ capturedFrom: 'next', requests: paths }, null, 2)}\n`);
    }
    await context.close();
  });
});
