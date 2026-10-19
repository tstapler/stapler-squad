// @feature notification:inline-actions, notification:invoke-action, notification:get-history, approval:resolve, guidance-request:answer, backlog:transition
/**
 * Story 2.9: every inline action resolves from the tray row against the real isolated server.
 *
 * Pending state is real, not mocked: approvals come from the PermissionRequest hook, guidance from
 * CreateGuidanceRequest, backlog decisions from the e2e-local debug endpoints, each paired with the
 * SendNotification history row the descriptor builder reads. Only the desktop_only case fulfills a
 * descriptor (the isolated server is the local listener and cannot produce a remote one).
 *
 * Approve, Send back and Mark done wait 3 s behind Undo (design/ux.md 2.4); Deny takes the inline
 * reason step. ANSWER_QUESTION on a visible session is covered after Story 3.4 lands.
 */
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import {
  answerGuidanceViaRpc,
  backlogItemStatus,
  seedApproval,
  seedBacklogDecision,
  resolveApprovalViaRpc,
  seedGuidance,
} from './helpers/notification-action-seed';
import {
  INLINE_ACTIONS_FLAG,
  InlineActionRow,
  ToastDeck,
  actionLatencies,
  installActionLatencyProbe,
  openNotificationTray,
  setFeatureFlag,
} from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
// Any toast about a failed or refused action; unrelated toasts (a new approval) may be on screen.
const ERROR_COPY = /couldn.t|failed|error|isn.t valid|not sent/i;
const BACKLOG_FLAG = 'backlog';
const HOLD_TIMEOUT = 12_000;
const unique = (tag: string) => `${tag}-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;

test.describe.configure({ mode: 'serial' });

let sessionId = '';

test.beforeAll(async ({ request }) => {
  const client = new SessionClient(BASE_URL);
  const session = await client.createSession({ title: unique('inline-actions'), path: '/tmp', program: 'bash' });
  sessionId = session.id;
  await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, true);
  await setFeatureFlag(request, BASE_URL, BACKLOG_FLAG, true); // GetBacklogItem, used to read the decided status back
});

test.afterAll(async ({ request }) => {
  // The shared server outlives this file; other specs expect the legacy Approve/Deny buttons.
  await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, false);
  await setFeatureFlag(request, BASE_URL, BACKLOG_FLAG, false);
});

for (const key of ['V1', 'V2'] as const) {
  test.describe(`inline actions (${profiles[key].name})`, () => {
    test.use(profiles[key].use);

    test(`approve_should_resolve_the_hook_with_allow_after_one_click_when_${key}`, async ({ page, request }) => {
      const approval = await seedApproval(request, BASE_URL, sessionId, unique('approve-cmd'));
      await openNotificationTray(page, BASE_URL);
      const row = new InlineActionRow(page, approval.command);
      await row.reveal();

      await row.press(row.approve);
      // The row's own result line, or the legacy badge when the stream's resolved event lands first.
      await expect(row.row).toContainText(/Approved/, { timeout: HOLD_TIMEOUT });
      await expect(row.approve).toHaveCount(0);
      expect(await approval.decision).toMatchObject({ behavior: 'allow' });
    });

    test(`deny_should_resolve_the_hook_with_deny_and_the_reason_when_${key}`, async ({ page, request }) => {
      const approval = await seedApproval(request, BASE_URL, sessionId, unique('deny-cmd'));
      await openNotificationTray(page, BASE_URL);
      const row = new InlineActionRow(page, approval.command);
      await row.reveal();

      await row.press(row.deny);
      await row.row.getByTestId('deny-reason').fill('not now');
      await row.press(row.sendDenial);
      await expect(row.row).toContainText(/Denied/);
      await expect(row.deny).toHaveCount(0);
      expect(await approval.decision).toMatchObject({ behavior: 'deny' });
    });

    test(`backlog_decision_should_send_the_item_back_to_ready_after_one_click_when_${key}`, async ({ page, request }) => {
      const decision = await seedBacklogDecision(request, BASE_URL, unique('decide-item'));
      await openNotificationTray(page, BASE_URL);
      const row = new InlineActionRow(page, decision.title);
      await row.reveal();

      // Mark done needs a PASS review verdict; Send back is the decision a bare review item accepts.
      await row.press(row.option('Send back'));
      await expect(row.status).toHaveText('Decided: Send back', { timeout: HOLD_TIMEOUT });
      await expect.poll(() => backlogItemStatus(request, BASE_URL, decision.itemId)).toBe('ready');
    });

    test(`guidance_answer_should_record_the_chosen_option_after_one_click_when_${key}`, async ({ page, request }) => {
      const guidance = await seedGuidance(request, BASE_URL, unique('Which database?'), ['Postgres', 'SQLite']);
      await openNotificationTray(page, BASE_URL);
      const row = new InlineActionRow(page, guidance.question);
      await row.reveal();

      await row.press(row.option('SQLite'));
      await expect(row.status).toHaveText('Answered: SQLite');
    });
  });
}

test.describe('inline actions (stale and degraded paths)', () => {
  test.use(profiles.V1.use);

  test('click_should_read_already_handled_without_an_alert_when_the_question_was_answered_elsewhere', async ({ page, request }) => {
    const guidance = await seedGuidance(request, BASE_URL, unique('Ship it?'), ['Yes', 'No']);
    await openNotificationTray(page, BASE_URL);
    const row = new InlineActionRow(page, guidance.question);
    await row.reveal();
    await expect(row.option('Yes')).toBeVisible();

    await answerGuidanceViaRpc(request, BASE_URL, guidance.id, 'No');
    await row.press(row.option('Yes'));

    await expect(row.status).toContainText('Already handled');
    await expect(new ToastDeck(page).toasts.filter({ hasText: ERROR_COPY })).toHaveCount(0);
  });
});

test.describe('inline actions (phone touch)', () => {
  test.use(profiles.V2.use);

  test('approve_should_tap_through_with_deny_first_and_44px_targets_when_phone', async ({ page, request }) => {
    const approval = await seedApproval(request, BASE_URL, sessionId, unique('tap-cmd'));
    await openNotificationTray(page, BASE_URL);
    const row = new InlineActionRow(page, approval.command);
    await row.reveal();

    const [deny, approve] = await Promise.all([row.deny.boundingBox(), row.approve.boundingBox()]);
    expect(deny!.y).toBeLessThan(approve!.y); // the thumb's first target is the safe one
    expect(Math.min(deny!.height, approve!.height)).toBeGreaterThanOrEqual(44);

    await row.press(row.approve, { tap: true });
    await expect(row.row).toContainText(/Approved/, { timeout: HOLD_TIMEOUT });
    expect(await approval.decision).toMatchObject({ behavior: 'allow' });
  });
});

test.describe('inline actions (descriptor and flag degradation)', () => {
  test.use(profiles.V1.use);

  test('tier_b_answer_should_render_disabled_with_the_desktop_reason_and_send_nothing_when_the_descriptor_is_desktop_only', async ({ page, request }) => {
    const guidance = await seedGuidance(request, BASE_URL, unique('Pick a branch?'), ['main']);
    // The isolated server is the local listener, so the remote-listener descriptor is fulfilled.
    await page.route('**/api/session.v1.SessionService/GetNotificationHistory', async (route) => {
      const response = await route.fetch();
      const body = (await response.json()) as { notifications?: { title?: string; actions?: object[] }[] };
      for (const n of body.notifications ?? []) {
        if (n.title !== guidance.question) continue;
        n.actions = [
          {
            kind: 'ACTION_KIND_ANSWER_QUESTION',
            label: 'Answer',
            tier: 'ACTION_TIER_B',
            available: false,
            unavailableReason: 'desktop_only',
            targetKeyType: 'ACTION_TARGET_KEY_QUESTION_ID',
            targetKey: 'q-desktop-only',
            sessionId: 'desktop-only-session',
            options: [{ label: 'Yes' }, { label: 'No' }],
          },
          { kind: 'ACTION_KIND_OPEN', label: 'Open', tier: 'ACTION_TIER_A', available: true, sessionId: 'desktop-only-session' },
        ];
      }
      await route.fulfill({ response, json: body });
    });
    const invokes: string[] = [];
    page.on('request', (r) => {
      if (r.url().includes('InvokeNotificationAction')) invokes.push(r.url());
    });

    await openNotificationTray(page, BASE_URL);
    const row = new InlineActionRow(page, guidance.question);
    await row.reveal();

    await expect(row.unavailableReason).toHaveText('Answer from desktop');
    await expect(row.option('Yes')).toHaveAttribute('aria-disabled', 'true');
    await row.option('Yes').press('Enter'); // aria-disabled stays focusable, so the key is the way to try it
    await expect(row.status).toHaveCount(0);
    expect(invokes).toEqual([]);
  });

  test('flag_flipped_off_during_an_action_should_settle_in_place_without_a_reload_and_next_render_should_be_legacy', async ({ page, request }) => {
    const first = await seedApproval(request, BASE_URL, sessionId, unique('flip-cmd-1'));
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => (release = resolve));
    let seen: () => void = () => {};
    const requestSeen = new Promise<void>((resolve) => (seen = resolve));
    await page.route('**/api/session.v1.SessionService/InvokeNotificationAction', async (route) => {
      seen();
      await gate;
      await route.continue();
    });

    await openNotificationTray(page, BASE_URL);
    await page.evaluate(() => ((window as unknown as { __noReload: boolean }).__noReload = true));
    const urlBefore = page.url();
    const row = new InlineActionRow(page, first.command);
    await row.reveal();
    await row.press(row.approve);
    await requestSeen; // the 3 s hold elapsed and the request is now in flight

    await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, false);
    release();

    // Applied, or degraded to Open session; never an error, a toast or a reload.
    await expect(row.row).toContainText(/Approved|This action is turned off\. Open session\./, { timeout: HOLD_TIMEOUT });
    test.info().annotations.push({ type: 'settled-as', description: (await row.row.innerText()).includes('turned off') ? 'FLAG_OFF copy' : 'applied' });
    await expect(new ToastDeck(page).toasts.filter({ hasText: ERROR_COPY })).toHaveCount(0);
    expect(await page.evaluate(() => (window as unknown as { __noReload?: boolean }).__noReload)).toBe(true);
    expect(page.url()).toBe(urlBefore);

    // The next render: the server now sends no inline descriptors, so the legacy buttons return.
    const second = await seedApproval(request, BASE_URL, sessionId, unique('flip-cmd-2'));
    await openNotificationTray(page, BASE_URL);
    const next = new InlineActionRow(page, second.command);
    await next.reveal();
    await expect(next.row.getByRole('button', { name: '✓ Approve' })).toBeVisible();
    await expect(next.approve).toHaveCount(0);

    // Leave no hook request pending, and the flag as the next test expects it.
    await resolveApprovalViaRpc(request, BASE_URL, second.approvalId, 'allow');
    await resolveApprovalViaRpc(request, BASE_URL, first.approvalId, 'allow').catch(() => {}); // already settled when the action applied
    await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, true);
  });
});

test.describe('inline actions (round-trip latency)', () => {
  test.use(profiles.V1.use);
  const latencySessionIds: string[] = [];

  test.afterAll(async () => {
    const client = new SessionClient(BASE_URL);
    await Promise.all(latencySessionIds.map((id) => client.deleteSession(id, true).catch(() => {})));
  });

  test('approve_round_trip_p95_should_stay_under_one_second_over_20_approvals', async ({ page, request }) => {
    test.setTimeout(240_000);
    const runId = unique('lat');
    // One session per approval: a session's approvals group into one card.
    const approvals = [];
    for (let i = 0; i < 20; i++) {
      const created = await new SessionClient(BASE_URL).createSession({ title: `${runId}-s${i}`, path: '/tmp', program: 'sleep 3600' });
      latencySessionIds.push(created.id);
      approvals.push(await seedApproval(request, BASE_URL, created.id, `${runId}-${i}`));
    }

    await installActionLatencyProbe(page);
    const tray = await openNotificationTray(page, BASE_URL);
    // Approve whichever card of this run still has the buttons.
    await tray.tray.getByRole('searchbox', { name: 'Search notifications' }).fill(runId);
    const row = new InlineActionRow(page, runId);
    for (let i = 0; i < approvals.length; i++) {
      await row.press(row.approve.first());
      await expect.poll(async () => (await actionLatencies(page)).length, { message: `latency sample ${i + 1}`, timeout: HOLD_TIMEOUT }).toBe(i + 1);
    }

    const sorted = [...(await actionLatencies(page))].sort((a, b) => a - b);
    const p95 = sorted[Math.ceil(0.95 * sorted.length) - 1];
    test.info().annotations.push({ type: 'approve-latency-ms', description: `n=${sorted.length} p50=${Math.round(sorted[Math.floor(sorted.length / 2)])} p95=${Math.round(p95)} max=${Math.round(sorted[sorted.length - 1])}` });
    expect(sorted).toHaveLength(20);
    // Soft on CI: shared runners add noise the localhost budget (REQ-19) does not allow for.
    if (process.env.CI) {
      test.info().annotations.push({ type: 'soft', description: 'p95 < 1000 ms is advisory on CI' });
      expect.soft(p95).toBeLessThan(1000);
    } else {
      expect(p95).toBeLessThan(1000);
    }
  });
});
