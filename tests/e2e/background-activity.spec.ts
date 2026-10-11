// @feature notification-tray, notification:background-activity, session-detail
/**
 * Story 5.4 (Background activity in the tray): BA-2, BA-4, BA-5, BA-8, BA-9, T-E2-19.
 *
 * The shared test-mode instance cannot create `Hidden: true` sessions (see
 * hidden-session-view.spec.ts), so ListSessions{hiddenOnly} and GetSession are fulfilled
 * with fabricated hidden sessions while the failures themselves are real notifications
 * sent through SendNotification. A main-list ListSessions is passed through untouched.
 */
import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { BackgroundSegment, NotificationTray, ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

interface HiddenFixture {
  id: string;
  title: string;
  status: string;
  updatedAt?: string;
}

const hidden = (id: string, status = 'SESSION_STATUS_ACTIVE', updatedAt = new Date().toISOString()): HiddenFixture => ({
  id,
  title: id,
  status,
  updatedAt,
});

interface Calls {
  hiddenPolls: number;
  mainListIncludesHidden: number;
}

/** Serves ListSessions{hiddenOnly} from `current()`; everything else is the real server. */
async function fakeHiddenSessions(page: Page, current: () => HiddenFixture[] | 'error'): Promise<Calls> {
  const calls: Calls = { hiddenPolls: 0, mainListIncludesHidden: 0 };
  await page.route('**/api/session.v1.SessionService/ListSessions', async (route) => {
    const body = (route.request().postDataJSON() ?? {}) as { hiddenOnly?: boolean; includeHidden?: boolean };
    if (!body.hiddenOnly) {
      if (body.includeHidden) calls.mainListIncludesHidden += 1;
      return route.continue();
    }
    calls.hiddenPolls += 1;
    const result = current();
    if (result === 'error') {
      return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) });
    }
    const sessions = result.map((s) => ({ ...s, program: 'claude', path: '/tmp/e2e-background', branch: 'main', hidden: true }));
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ sessions }) });
  });
  await page.route('**/api/session.v1.SessionService/GetSession', async (route) => {
    const { id } = (route.request().postDataJSON() ?? {}) as { id?: string };
    const session = current() === 'error' ? undefined : (current() as HiddenFixture[]).find((s) => s.id === id);
    if (!session) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ code: 'not_found', message: 'gone' }) });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ session: { ...session, program: 'claude', path: '/tmp/e2e-background', branch: 'main', hidden: true } }),
    });
  });
  return calls;
}

async function storedFor(request: APIRequestContext, sessionId: string): Promise<number> {
  const response = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetNotificationHistory`, {
    headers: { 'Content-Type': 'application/json' },
    data: { limit: 500 },
  });
  const body = (await response.json()) as { notifications?: { sessionId?: string }[] };
  return (body.notifications ?? []).filter((n) => n.sessionId === sessionId).length;
}

async function seedFailure(request: APIRequestContext, sessionId: string, title = 'Review failed'): Promise<void> {
  await sendNotification(request, BASE_URL, { sessionId, type: 'ERROR', title, message: '3 tests failing' });
  // The subscriber coalesces for ~500ms before persisting.
  await expect.poll(() => storedFor(request, sessionId), { timeout: 10_000 }).toBeGreaterThan(0);
}

async function openTray(page: Page): Promise<NotificationTray> {
  const tray = new NotificationTray(page);
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  const deck = new ToastDeck(page);
  await tray.handle.or(deck.chip).or(tray.entryOpen).first().click();
  await tray.expectOpen();
  await tray.dismissWhatChanged();
  return tray;
}

for (const key of ['V1', 'V2'] as const) {
  test.describe(`background activity (${profiles[key].name})`, () => {
    test.use(profiles[key].use);

    test.beforeEach(async ({ page, request }) => {
      await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
      await page.addInitScript((k) => localStorage.setItem(k, 'true'), ONBOARDED_KEY);
    });
    test.afterEach(async ({ request }) => {
      await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
    });

    test('background_row_should_appear_leave_on_open_and_main_list_should_exclude_hidden_when_failure_seeded', async ({ page, request }) => {
      const id = `review:bg-${key}-${Date.now()}`;
      const calls = await fakeHiddenSessions(page, () => [hidden(id), hidden(`${id}-ok`, 'SESSION_STATUS_STOPPED')]);
      await seedFailure(request, id);

      const tray = await openTray(page);
      const background = new BackgroundSegment(page);
      // BA-2: collapsed segment, 0 polls.
      expect(calls.hiddenPolls).toBe(0);

      await background.tab.click();
      await expect.poll(() => calls.hiddenPolls).toBe(1);
      await expect(background.tab).toContainText('Background (1)');
      const row = background.rows.filter({ hasText: id });
      await expect(row).toHaveCount(1);
      await expect(row.getByTestId('notification-background-chip')).toHaveText('Background');
      await expect(row).toContainText('FAILED');
      await expect(background.summary).toContainText('1 completed OK today');

      // BA-4: one tap opens the read-only view. The link carries the deep-link params; the
      // app may rewrite the URL to its own window id afterwards, so the banner is the proof.
      const view = row.getByTestId('notification-view-output');
      await expect(view).toHaveAttribute('href', /tab=terminal&notification=/);
      await view.click();
      await expect(page.getByTestId('readonly-banner')).toBeVisible();

      // BA-9: the record is read, so the row is gone and the badge dropped.
      await expect.poll(async () => {
        await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
        const t = new NotificationTray(page);
        await t.handle.or(new ToastDeck(page).chip).or(t.entryOpen).first().click();
        await new BackgroundSegment(page).tab.click();
        return new BackgroundSegment(page).rows.filter({ hasText: id }).count();
      }, { timeout: 20_000 }).toBe(0);
      await expect(tray.tray).toBeVisible();

      // BA-8: nothing in the app ever asked the main list for hidden sessions.
      expect(calls.mainListIncludesHidden).toBe(0);
    });

    test('deleted_hidden_session_row_should_stay_as_no_longer_available_until_read', async ({ page, request }) => {
      const id = `review:bg-gone-${key}-${Date.now()}`;
      let present = true;
      const calls = await fakeHiddenSessions(page, () => (present ? [hidden(id)] : []));
      await seedFailure(request, id);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect.poll(() => calls.hiddenPolls).toBeGreaterThan(0);
      const row = background.rows.filter({ hasText: id });
      await expect(row).toHaveCount(1);
      await expect(row).not.toContainText('Session no longer available');

      present = false;
      await background.refresh.click();
      await expect(row).toContainText('Session no longer available');
      await expect(row).toContainText('3 tests failing');
      await expect(row.getByTestId('notification-view-output')).toBeVisible();
    });

    test('error_should_show_retry_keep_stale_rows_and_recover_when_retry_succeeds', async ({ page, request }) => {
      const id = `review:bg-err-${key}-${Date.now()}`;
      let mode: 'ok' | 'error' = 'ok';
      await fakeHiddenSessions(page, () => (mode === 'ok' ? [hidden(id)] : 'error'));
      await seedFailure(request, id);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      const row = background.rows.filter({ hasText: id });
      await expect(row).toHaveCount(1);

      mode = 'error';
      await background.refresh.click();
      await expect(background.error).toContainText('Could not load background activity.');
      await expect(background.stale).toContainText('Showing data from');
      await expect(row).toHaveCount(1);

      mode = 'ok';
      await background.error.getByRole('button', { name: 'Retry' }).click();
      await expect(background.error).toHaveCount(0);
      await expect(row).toHaveCount(1);
    });

    test('hidden_session_without_failure_should_produce_no_row_and_show_the_healthy_state', async ({ page }) => {
      const id = `review:bg-ok-${key}-${Date.now()}`;
      await fakeHiddenSessions(page, () => [hidden(id, 'SESSION_STATUS_STOPPED')]);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect(background.emptyHealthy).toContainText('Nothing needs attention in the background.');
      await expect(background.emptyHealthy).toContainText('completed OK today');
      await expect(background.rows.filter({ hasText: id })).toHaveCount(0);
    });

    test('ba7_should_pass_axe_and_target_size_on_background', async ({ page, request }) => {
      const id = `review:bg-a11y-${key}-${Date.now()}`;
      await fakeHiddenSessions(page, () => [hidden(id), hidden(`${id}-ok`, 'SESSION_STATUS_STOPPED')]);
      await seedFailure(request, id);

      const tray = await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect(background.rows.filter({ hasText: id })).toHaveCount(1);
      // The slide-in transform is still settling; wait until the tray stops moving.
      await expect
        .poll(() =>
          tray.tray.evaluate((el) => {
            const m = new DOMMatrix(getComputedStyle(el).transform);
            return Math.abs(m.m41) + Math.abs(m.m42) < 0.5;
          }),
        )
        .toBe(true);
      await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' });

      const small = await background.rows.first().evaluate((row) =>
        Array.from(row.querySelectorAll('button, a[href]'))
          .map((el) => {
            const r = (el as HTMLElement).getBoundingClientRect();
            return { name: (el.textContent || '').trim(), w: Math.round(r.width), h: Math.round(r.height) };
          })
          .filter((c) => c.h < 43 || c.w < 43),
      );
      expect(small).toEqual([]);

      const results = await new AxeBuilder({ page })
        .include('[data-testid="notification-tray"]')
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
        .analyze();
      const serious = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
      expect(serious.map((v) => `${v.id}: ${v.nodes[0]?.html.slice(0, 100)}`)).toEqual([]);
    });

    test('tray_row_of_a_hidden_session_should_carry_background_chip_and_view_output_testid', async ({ page, request }) => {
      const id = `review:bg-row-${key}-${Date.now()}`;
      await fakeHiddenSessions(page, () => [hidden(id)]);
      await seedFailure(request, id);

      const tray = await openTray(page);
      const row = tray.rows.filter({ hasText: id });
      await expect(row).toHaveCount(1);
      await expect(row.getByTestId('notification-background-chip')).toHaveText('Background');
      const view = row.getByTestId('notification-view-output');
      await expect(view).toHaveText('View output');
      await expect(view).toHaveAttribute('href', new RegExp(`tab=terminal&notification=`));
      await expect(row.getByTestId('notification-view-session')).toHaveCount(0);
    });
  });
}
