// @feature session-reply-card, session:reply-to-question, notification:deep-link
/**
 * Story 5.6 (audited Reply to a pending question): RP-1, RP-2, RP-3, RP-4, RP-9, RP-10,
 * RP-13, RP-14, RP-15, RP-17, RP-18 in a real browser.
 *
 * The shared test-mode instance cannot create hidden sessions or a live AskUserQuestion
 * dialog, so ListSessions/GetSession, the notification history (a question record carrying
 * the metadata the server stamps) and ReplyToPendingQuestion are fulfilled with fabricated
 * responses. The server-side guarantees (one digit, DialogMatch, audit before write, lease,
 * rate limit) are covered by the Go tests in server/services and session.
 */
import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const HIDDEN_ID = 'e2e-hidden-reply-session';
const QUESTION_NOTIFICATION = 'e2e-question-1';

const hiddenSession = {
  id: HIDDEN_ID,
  title: 'review:reply01',
  status: 'SESSION_STATUS_ACTIVE',
  program: 'claude',
  path: '/tmp/e2e-hidden-reply',
  branch: 'main',
  hidden: true,
};

type Metadata = Record<string, string>;

const SINGLE: Metadata = {
  question_id: 'q-e2e-1',
  question_shape: 'single',
  question_options: JSON.stringify(['Red', 'Green', 'Blue']),
};

async function fakeHiddenSession(page: Page, metadata: Metadata): Promise<void> {
  // ListSessions and WatchSessions stay with the real server: the card is disabled while the
  // session stream is not live (offline), so the stream must be real. The hidden session is
  // reached through the GetSession fallback of the deep link.
  await page.route('**/api/session.v1.SessionService/GetSession', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ session: hiddenSession }) }),
  );
  await page.route('**/api/session.v1.SessionService/GetNotificationHistory', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        notifications: [
          {
            id: QUESTION_NOTIFICATION,
            sessionId: HIDDEN_ID,
            sessionName: hiddenSession.title,
            title: 'Claude has a question',
            message: 'Which color should the spike use?',
            notificationType: 'NOTIFICATION_TYPE_INPUT_REQUIRED',
            priority: 'NOTIFICATION_PRIORITY_URGENT',
            createdAt: new Date().toISOString(),
            isRead: false,
            isPendingDecision: true,
            metadata,
          },
        ],
        totalCount: 1,
      }),
    }),
  );
}

interface ReplyCall {
  sessionId: string;
  questionId: string;
  replyText: string;
  replyId: string;
}

/** Fulfils ReplyToPendingQuestion with `outcome` and records each request. */
async function fakeReply(page: Page, outcome: string, retryAfterSeconds = 0): Promise<ReplyCall[]> {
  const calls: ReplyCall[] = [];
  await page.route('**/api/session.v1.SessionService/ReplyToPendingQuestion', (route) => {
    calls.push(route.request().postDataJSON() as ReplyCall);
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ outcome, retryAfterSeconds }),
    });
  });
  return calls;
}

async function open(page: Page): Promise<void> {
  await page.addInitScript(() => localStorage.setItem('stapler-squad:onboarded', 'true'));
  await page.goto(`${BASE_URL}/?session=${HIDDEN_ID}&tab=terminal&notification=${QUESTION_NOTIFICATION}&reply=1`, {
    waitUntil: 'domcontentloaded',
  });
  await expect(page.getByTestId('readonly-banner')).toBeVisible();
}

for (const key of ['V1', 'V2'] as const) {
  test.describe(`hidden-session reply card (${profiles[key].name})`, () => {
    test.use(profiles[key].use);

    test('rp1_rp2_should_show_one_numbered_option_button_per_label_and_no_text_field', async ({ page }) => {
      await fakeHiddenSession(page, SINGLE);
      await open(page);

      const card = page.getByTestId('reply-card');
      await expect(card).toBeVisible();
      await expect(card.getByRole('heading', { name: 'Claude has a question' })).toBeVisible();
      await expect(page.getByTestId('reply-option-1')).toHaveText('1. Red');
      await expect(page.getByTestId('reply-option-2')).toHaveText('2. Green');
      await expect(page.getByTestId('reply-option-3')).toHaveText('3. Blue');
      await expect(card.getByRole('textbox')).toHaveCount(0);
      await expect(page.getByTestId('readonly-banner-secondary')).toContainText("Reply to Claude's question below");

      // RP-9 / the 44px target rule.
      for (const n of [1, 2, 3]) {
        const box = await page.getByTestId(`reply-option-${n}`).boundingBox();
        expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
      }
      // Directly below the banner, above the terminal.
      const banner = await page.getByTestId('readonly-banner').boundingBox();
      const cardBox = await card.boundingBox();
      expect(cardBox!.y).toBeGreaterThanOrEqual(banner!.y + banner!.height - 1);
    });

    test('rp3_rp4_rp18_should_send_one_reply_for_two_quick_taps_and_show_the_receipt', async ({ page }) => {
      await fakeHiddenSession(page, SINGLE);
      const calls = await fakeReply(page, 'REPLY_OUTCOME_SENT');
      await open(page);

      await page.getByTestId('reply-option-2').dispatchEvent('click');
      await page.getByTestId('reply-option-3').dispatchEvent('click').catch(() => undefined);

      await expect(page.getByTestId('reply-receipt')).toContainText('Sent: 2. Green');
      await expect(page.getByTestId('reply-receipt')).toContainText('the question closed');
      await expect(page.getByTestId('reply-option-1')).toHaveCount(0);
      expect(calls).toHaveLength(1);
      expect(calls[0]).toMatchObject({ sessionId: HIDDEN_ID, questionId: 'q-e2e-1', replyText: '2' });
      expect(calls[0].replyId).toBeTruthy();
    });

    test('rp13_should_show_indeterminate_with_view_output_and_no_retry', async ({ page }) => {
      await fakeHiddenSession(page, SINGLE);
      await fakeReply(page, 'REPLY_OUTCOME_SEND_INDETERMINATE');
      await open(page);

      await page.getByTestId('reply-option-1').click();
      await expect(page.getByTestId('reply-status')).toHaveText('Sent? Check the terminal output to confirm');
      await expect(page.getByTestId('reply-retry')).toHaveCount(0);
      await expect(page.getByTestId('reply-view-output')).toBeVisible();
    });

    test('rp14_should_show_stale_prompt_copy_with_no_retry', async ({ page }) => {
      await fakeHiddenSession(page, SINGLE);
      await fakeReply(page, 'REPLY_OUTCOME_STALE_PROMPT');
      await open(page);

      await page.getByTestId('reply-option-1').click();
      await expect(page.getByTestId('reply-status')).toHaveText(
        'This question is no longer on screen. You may have answered it already. Check the terminal.',
      );
      await expect(page.getByTestId('reply-retry')).toHaveCount(0);
    });

    test('rp15_rp17_should_show_answer_in_the_terminal_with_no_buttons_for_unsupported_shapes', async ({ page }) => {
      await fakeHiddenSession(page, { question_shape: 'multi' });
      await open(page);
      await expect(page.getByTestId('reply-unavailable')).toHaveText('Answer in the terminal');
      await expect(page.getByTestId('reply-option-1')).toHaveCount(0);
    });

    test('rp17_should_say_reply_is_unavailable_when_the_session_hook_has_no_proof', async ({ page }) => {
      await fakeHiddenSession(page, { reply_unavailable: 'no_proof', question_shape: 'single' });
      await open(page);
      await expect(page.getByTestId('reply-unavailable')).toHaveText(
        'Reply unavailable for this session. Answer in the terminal.',
      );
      await expect(page.getByTestId('reply-option-1')).toHaveCount(0);
    });

    test('rp10_axe_should_find_no_serious_violations_in_the_choosing_and_sent_states', async ({ page }) => {
      await fakeHiddenSession(page, SINGLE);
      await fakeReply(page, 'REPLY_OUTCOME_SENT');
      await open(page);
      await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' });

      const scan = async () =>
        (await new AxeBuilder({ page }).include('[data-testid="reply-card"]').withTags(['wcag2a', 'wcag2aa', 'wcag22aa']).analyze())
          .violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
      expect(await scan()).toEqual([]);
      await page.getByTestId('reply-option-1').click();
      await expect(page.getByTestId('reply-receipt')).toBeVisible();
      expect(await scan()).toEqual([]);
    });
  });
}
