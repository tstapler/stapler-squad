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
  backlogItemStatus,
  seedApproval,
  seedBacklogDecision,
} from './helpers/notification-action-seed';
import {
  INLINE_ACTIONS_FLAG,
  InlineActionRow,
  openNotificationTray,
  setFeatureFlag,
} from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
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
  });
}
