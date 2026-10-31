// @feature terminal-render, session-stream-terminal
/**
 * ADR-033: with the terminal_ws_pool flag on, StreamTerminal runs over the shared
 * /api/ws/terminal-mux socket instead of one WebSocket per terminal; with it off the
 * per-stream socket is unchanged.
 */
import { test, expect, type Page, type WebSocket } from '@playwright/test';
import { flagEnabled, setFlag } from './helpers/feature-flag';
import { SessionClient } from './helpers/session-client';
import { ShellTabsPage } from './pages/ShellTabsPage';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const FIXTURE_SESSION_TITLE = 'terminal-ws-pool-e2e-fixture';
const MUX_FRAME_DATA = 3;

function trackSockets(page: Page): { muxFrames: () => number; perStream: () => number } {
  let muxDataFrames = 0;
  let perStream = 0;
  page.on('websocket', (ws: WebSocket) => {
    if (/\/api\/ws\/terminal-mux(\?|$)/.test(ws.url())) {
      ws.on('framereceived', (f) => {
        const payload = f.payload;
        if (typeof payload !== 'string' && payload.length > 0 && payload[0] === MUX_FRAME_DATA) muxDataFrames++;
      });
    } else if (/session\.v1\.SessionService\/StreamTerminal(\?|$)/.test(ws.url())) {
      perStream++;
    }
  });
  return { muxFrames: () => muxDataFrames, perStream: () => perStream };
}

async function openFixtureSession(page: Page, poolFlag: '1' | '0'): Promise<void> {
  await page.addInitScript((flag) => {
    localStorage.setItem('stapler-squad:onboarded', 'true');
    // The persisted pre-load value arms the pool for the first stream, before GetFeatureFlags returns.
    localStorage.setItem('ssq:flag:terminal_ws_pool', flag);
  }, poolFlag);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForSelector('[data-testid="session-card"], [data-testid="session-row"]', { timeout: 15000 });
  await new ShellTabsPage(page).openSessionByTitle(FIXTURE_SESSION_TITLE);
  await expect(page.getByRole('textbox', { name: 'Terminal input' }).first()).toBeAttached({ timeout: 10000 });
}

test.beforeAll(async () => {
  const client = new SessionClient(BASE_URL);
  const sessions = await client.listSessions();
  if (!sessions.some((s) => s.title === FIXTURE_SESSION_TITLE)) {
    await client.createSession({ title: FIXTURE_SESSION_TITLE, path: '/tmp', program: 'bash' });
  }
});

test.describe('terminal-ws-pool', () => {
  test.describe.configure({ mode: 'serial' });
  let initialFlag = false;
  test.beforeAll(async ({ request }) => {
    initialFlag = await flagEnabled(request, 'terminal_ws_pool');
  });
  test.afterAll(async ({ request }) => {
    await setFlag(request, 'terminal_ws_pool', initialFlag);
  });

  test('terminalStream_should_UseSharedMuxSocket_When_PoolFlagIsOn', async ({ page, request }) => {
    await setFlag(request, 'terminal_ws_pool', true);
    const sockets = trackSockets(page);
    await openFixtureSession(page, '1');
    await expect.poll(() => sockets.muxFrames(), { timeout: 15000 }).toBeGreaterThan(0);
    // The first DATA frame is only the Connect response header; keystroke echo proves output flows through the pool.
    await expect.poll(() => sockets.muxFrames(), { timeout: 15000 }).toBeGreaterThan(2);
    const beforeTyping = sockets.muxFrames();
    const terminalInput = page.getByRole('textbox', { name: 'Terminal input' }).first();
    await terminalInput.focus();
    await page.keyboard.type('echo pool-e2e-marker\n');
    await expect.poll(() => sockets.muxFrames(), { timeout: 15000 }).toBeGreaterThan(beforeTyping);
    expect(sockets.perStream()).toBe(0);
  });

  test('terminalStream_should_UsePerStreamSocket_When_PoolFlagIsOff', async ({ page, request }) => {
    await setFlag(request, 'terminal_ws_pool', false);
    const sockets = trackSockets(page);
    await openFixtureSession(page, '0');
    await expect.poll(() => sockets.perStream(), { timeout: 15000 }).toBeGreaterThan(0);
    expect(sockets.muxFrames()).toBe(0);
  });
});
