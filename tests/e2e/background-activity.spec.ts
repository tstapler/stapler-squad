// @feature notification-tray, notification:background-activity, session-detail
/**
 * Story 5.4 (Background activity in the tray): BA-2, BA-4, BA-5, BA-8, BA-9, T-E2-19.
 * Stories 1.4/1.6: activity rows and roll-up for hidden sessions with no history, outcome chips,
 * the poll-failure banner, and the background_activity_feed kill switch.
 *
 * The shared test-mode instance cannot create `Hidden: true` sessions (see
 * hidden-session-view.spec.ts), so ListSessions{hiddenOnly} and GetSession are fulfilled
 * with fabricated hidden sessions while the failures themselves are real notifications
 * sent through SendNotification. A main-list ListSessions is passed through untouched.
 */
import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { BackgroundSegment, NotificationTray, ToastDeck, sendNotification } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const ONBOARDED_KEY = 'stapler-squad:onboarded';

interface HiddenFixture {
  id: string;
  title: string;
  status: string;
  updatedAt?: string;
  backgroundOutcome?: 'completed' | 'failed' | 'unknown';
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

/** Eleven stopped hidden sessions with no notification history; `outcome(i)` is what the server reports. */
function elevenStopped(prefix: string, outcome: (i: number) => 'completed' | 'unknown'): HiddenFixture[] {
  return Array.from({ length: 11 }, (_, i) => ({
    ...hidden(`review:${prefix}-${i}`, 'SESSION_STATUS_STOPPED'),
    backgroundOutcome: outcome(i),
  }));
}

/** The real GetFeatureFlags answer with background_activity_feed forced off. */
async function flagFeedOff(page: Page): Promise<void> {
  await page.route('**/api/session.v1.SessionService/GetFeatureFlags', async (route) => {
    const response = await route.fetch();
    const body = (await response.json()) as { flags?: { name: string }[] };
    const flags = (body.flags ?? []).filter((f) => f.name !== 'background_activity_feed');
    flags.push({ name: 'background_activity_feed', enabled: false, scopes: [] } as { name: string });
    await route.fulfill({ response, json: { ...body, flags } });
  });
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
  await tray.handle.or(deck.chip).or(tray.entryOpen).locator('visible=true').first().click();
  await tray.expectOpen();
  await tray.dismissWhatChanged();
  return tray;
}

/** Reloads and leaves the tray open, whether or not the app restored it open (a click would then close it). */
async function reopenTray(page: Page): Promise<void> {
  const tray = new NotificationTray(page);
  const entry = tray.handle.or(new ToastDeck(page).chip).or(tray.entryOpen).locator('visible=true').first();
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await expect
    .poll(async () => {
      if ((await tray.tray.getAttribute('data-state')) === 'open') return true;
      await entry.click();
      return false;
    }, { timeout: 15_000 })
    .toBe(true);
}

for (const key of ['V1', 'V2'] as const) {
  test.describe(`background activity (${profiles[key].name})`, () => {
    test.use(profiles[key].use);

    test.beforeEach(async ({ page }) => {
      await page.addInitScript((k) => localStorage.setItem(k, 'true'), ONBOARDED_KEY);
    });

    test('background_row_should_appear_leave_on_open_and_main_list_should_exclude_hidden_when_failure_seeded', async ({ page, request }) => {
      const id = `review:bg-${key}-${Date.now()}`;
      const calls = await fakeHiddenSessions(page, () => [hidden(id), hidden(`review:stopped-${key}-${Date.now()}`, 'SESSION_STATUS_STOPPED')]);
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
      await expect(background.rollup).toContainText('2 background sessions, 1 needs attention');
      await expect(background.stateChips).toHaveText(['Stopped, outcome unknown']);

      // BA-4: one tap opens the read-only view. The link carries the deep-link params; the
      // app may rewrite the URL to its own window id afterwards, so the banner is the proof.
      const view = row.getByTestId('notification-view-output');
      await expect(view).toHaveAttribute('href', /tab=terminal&notification=/);
      await view.click();
      await expect(page.getByTestId('readonly-banner')).toBeVisible();

      // BA-9: the record is read, so the row is gone and the badge dropped.
      await expect.poll(async () => {
        await reopenTray(page);
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
      await expect(background.error).toContainText("Couldn't refresh background sessions. Showing data from");
      await expect(row).toHaveCount(1);

      mode = 'ok';
      await background.error.getByRole('button', { name: 'Retry' }).click();
      await expect(background.error).toHaveCount(0);
      await expect(row).toHaveCount(1);
    });

    test('hidden_session_without_failure_should_show_one_unknown_row_and_a_rollup_that_never_says_healthy', async ({ page }) => {
      const id = `review:bg-ok-${key}-${Date.now()}`;
      await fakeHiddenSessions(page, () => [hidden(id, 'SESSION_STATUS_STOPPED')]);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect(background.rollup).toContainText('1 background session, no failures reported');
      await expect(background.rollup).not.toContainText(/healthy/i);
      await expect(background.rows.filter({ hasText: id })).toHaveCount(1);
      await expect(background.stateChips).toHaveText(['Stopped, outcome unknown']);
    });

    test('ux_ac1_should_show11RowsAndRollupWithoutHealthy_When_11HiddenNoHistory', async ({ page }) => {
      const stopped = elevenStopped(`bg-11-${key}-${Date.now()}`, (i) => (i < 10 ? 'completed' : 'unknown'));
      await fakeHiddenSessions(page, () => stopped);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect(background.rows).toHaveCount(11);
      await expect(background.rollup).toContainText('11 background sessions, no failures reported');
      await expect(background.rollup).not.toContainText(/healthy/i);
      await expect(background.stateChips.filter({ hasText: /^Finished$/ })).toHaveCount(10);
      await expect(background.stateChips.filter({ hasText: 'Stopped, outcome unknown' })).toHaveCount(1);
    });

    test('background_should_label_every_stopped_session_outcome_unknown_when_the_server_reports_no_outcome', async ({ page }) => {
      const stopped = elevenStopped(`bg-unk-${key}-${Date.now()}`, () => 'unknown');
      await fakeHiddenSessions(page, () => stopped);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect(background.rows).toHaveCount(11);
      await expect(background.stateChips).toHaveText(Array(11).fill('Stopped, outcome unknown'));
    });

    test('flag_off_should_show_the_legacy_empty_state_and_no_activity_rows_when_background_activity_feed_is_false', async ({ page }) => {
      const stopped = elevenStopped(`bg-off-${key}-${Date.now()}`, () => 'completed');
      await fakeHiddenSessions(page, () => stopped);
      await flagFeedOff(page);

      await openTray(page);
      const background = new BackgroundSegment(page);
      await background.tab.click();
      await expect(background.emptyHealthy).toContainText('Nothing needs attention in the background.');
      await expect(background.rows).toHaveCount(0);
    });

    test('ba7_should_pass_axe_and_target_size_on_background', async ({ page, request }) => {
      const id = `review:bg-a11y-${key}-${Date.now()}`;
      await fakeHiddenSessions(page, () => [hidden(id), hidden(`review:stopped-${key}-${Date.now()}`, 'SESSION_STATUS_STOPPED')]);
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
