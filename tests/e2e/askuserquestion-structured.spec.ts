// @feature session:guidance-structured, notification:get-history, notification:invoke-action, guidance-request:get, guidance-request:list-saved
/**
 * Epic 3 question cards, end to end on the isolated server.
 *
 * A "fake pane" session (fixtures/fake-question-pane.sh) draws the captured Claude Code dialog in a real tmux
 * pane on the run's private socket and records the one key it receives, so the keystroke path (InvokeNotificationAction,
 * AnswerRouter, PaneReplyCore, DialogMatch, ack detector) is real; only the AskUserQuestion hook that would register the
 * question is replaced by the e2e-local seed endpoint.
 */
import fs from 'fs';
import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { DIALOG_QUESTION, fakePaneSession, postToolUseInTerminal, readGuidanceRow, seedQuestionCard } from './helpers/notification-action-seed';
import { INLINE_ACTIONS_FLAG, QuestionCard, openNotificationTray, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();
const STRUCTURED_FLAG = 'askuserquestion_structured';
const HOOK = 'ask_user_question_hook';
const ACK_TIMEOUT = 30_000;
const unique = (tag: string) => `${tag}-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;

test.describe.configure({ mode: 'serial' });
test.setTimeout(120_000);

test.beforeAll(async ({ request }) => {
  await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, true);
  await setFeatureFlag(request, BASE_URL, STRUCTURED_FLAG, true);
});

test.afterAll(async ({ request }) => {
  // The shared server outlives this file; other specs expect the legacy buttons.
  await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, false);
  await setFeatureFlag(request, BASE_URL, STRUCTURED_FLAG, false);
});

const receivedKey = (keyFile: string) => (fs.existsSync(keyFile) ? fs.readFileSync(keyFile, 'utf8') : '');
/** The tabbed fake pane records one key name per line. */
const receivedKeys = (keyFile: string) => receivedKey(keyFile).split('\n').filter(Boolean);

// The measured two-question dialog (session/testdata/reply_dialogs/claude_auq_twoq_*.txt) and the recorded keys for Redis + [A, B].
const TABBED_QUESTIONS = [
  { text: 'Which cache?', header: 'Cache', multiSelect: false, options: ['Redis', 'Memcached', 'None'] },
  { text: 'Which letters?', header: 'Letters', multiSelect: true, options: ['A', 'B', 'C'] },
];
const NINE_KEYS = ['Enter', 'Enter', 'Down', 'Enter', 'Down', 'Down', 'Down', 'Enter', 'Enter'];

/** A tabbed-dialog fake pane, its hook question card revealed in the tray and Redis, A, B picked. */
async function pickedTabbedCard(page: Page, request: APIRequestContext, mode: 'tabbed' | 'tabbed-drift') {
  const { title, keyFile } = await fakePaneSession(new SessionClient(BASE_URL), unique(mode), mode);
  await seedQuestionCard(request, BASE_URL, { sessionId: title, source: HOOK, questions: TABBED_QUESTIONS });
  await openNotificationTray(page, BASE_URL);
  const card = new QuestionCard(page, title);
  await card.reveal();
  await expect(card.other).toHaveCount(0); // keys cannot carry typed text
  await card.press(card.group(/Which cache\?/).getByRole('button', { name: 'Redis' }));
  for (const letter of ['A', 'B']) await card.press(card.group(/Which letters\?/).getByRole('checkbox', { name: letter }));
  return { card, keyFile };
}

for (const key of ['V1', 'V2'] as const) {
  test.describe(`hook question answered from the tray (${profiles[key].name})`, () => {
    test.use(profiles[key].use);

    test(`click_should_send_exactly_one_key_and_read_answered_once_the_dialog_is_gone_when_${key}`, async ({ page, request }) => {
      const client = new SessionClient(BASE_URL);
      const { title, keyFile } = await fakePaneSession(client, unique('fake-pane'));
      await seedQuestionCard(request, BASE_URL, { sessionId: title, source: HOOK, questions: [{ ...DIALOG_QUESTION, multiSelect: false }] });
      await openNotificationTray(page, BASE_URL);
      const card = new QuestionCard(page, title);
      await card.reveal();

      await expect(card.other).toHaveCount(0); // a digit cannot carry typed text
      await expect(card.deliveryText).toHaveCount(0);
      const tap = key === 'V2';
      await card.press(card.option('Red'), { tap });

      await expect.poll(() => receivedKey(keyFile), { message: 'the fake pane received the digit' }).toBe('1');
      await expect(card.deliveryText).toHaveText(/Sent - waiting for the agent|Answered: Red/);
      await expect(card.deliveryText).toHaveText('Answered: Red', { timeout: ACK_TIMEOUT });
      expect(receivedKey(keyFile)).toBe('1'); // still exactly one key
      await expect(card.row.locator('[data-action-option]').first()).toHaveAttribute('aria-disabled', 'true');
    });
  });
}

test.describe('question cards (desktop)', () => {
  test.use(profiles.V1.use);

  test('answer_in_the_terminal_should_flip_an_open_card_to_already_handled_without_moving_focus', async ({ page, request }) => {
    const client = new SessionClient(BASE_URL);
    const { title } = await fakePaneSession(client, unique('terminal-answer'));
    await seedQuestionCard(request, BASE_URL, { sessionId: title, source: HOOK, questions: [{ ...DIALOG_QUESTION, multiSelect: false }] });
    await openNotificationTray(page, BASE_URL);
    const card = new QuestionCard(page, title);
    await card.reveal();
    const option = card.option('Green');
    await option.focus();

    await postToolUseInTerminal(request, BASE_URL, title);

    await expect(card.deliveryText).toHaveText('Already handled in the session. No action needed.', { timeout: 15_000 });
    await expect(option).toBeFocused();
    await expect(option).toHaveAttribute('aria-disabled', 'true');
  });

  test('retry_anyway_should_reverify_the_dialog_and_write_one_more_key_when_the_first_answer_left_it_showing', async ({ page, request }) => {
    const client = new SessionClient(BASE_URL);
    const { title, keyFile } = await fakePaneSession(client, unique('sticky-pane'), 'sticky');
    await seedQuestionCard(request, BASE_URL, { sessionId: title, source: HOOK, questions: [{ ...DIALOG_QUESTION, multiSelect: false }] });
    await openNotificationTray(page, BASE_URL);
    const card = new QuestionCard(page, title);
    await card.reveal();

    await card.press(card.option('Red'));

    await expect.poll(() => receivedKey(keyFile), { message: 'the first digit reached the pane' }).toBe('1');
    await expect(card.deliveryText).toHaveText(/couldn't confirm it arrived/);
    await card.press(card.row.getByRole('button', { name: 'Retry anyway' }));
    await expect(card.row.getByTestId('delivery-confirm')).toContainText('Retry anyway?');
    await card.press(card.row.getByTestId('delivery-confirm').getByRole('button', { name: 'Retry', exact: true }));

    await expect.poll(() => receivedKey(keyFile), { message: 'exactly one more digit after the re-verify' }).toBe('11');
    await expect(card.deliveryText).toHaveText('Answered: Red', { timeout: ACK_TIMEOUT });
    expect(receivedKey(keyFile)).toBe('11');
  });

  test('multi_question_agent_card_should_validate_inline_then_store_one_formatted_answer', async ({ page, request }) => {
    const client = new SessionClient(BASE_URL);
    const session = await client.createSession({ title: unique('agent-q'), path: '/tmp', program: 'bash' });
    const cache = unique('Which cache?');
    const regions = unique('Which regions?');
    const seeded = await seedQuestionCard(request, BASE_URL, {
      sessionId: session.title,
      source: 'agent_mcp',
      questions: [
        { text: cache, options: ['Redis', 'Memcached'] },
        { text: regions, multiSelect: true, options: ['A', 'B', 'C'] },
      ],
    });
    await openNotificationTray(page, BASE_URL);
    const card = new QuestionCard(page, session.title);
    await card.reveal();
    await expect(card.row.getByLabel('Other').first()).toBeVisible(); // a stored answer can carry text

    await card.press(card.group(new RegExp(cache.replace('?', '\\?'))).getByRole('button', { name: 'Redis' }));
    // Submit stays activatable while incomplete (aria-disabled, not disabled) so it can name what is missing.
    await card.press(card.submit, { force: true });
    const second = card.row.getByTestId('question-group-1');
    await expect(second).toHaveAttribute('aria-invalid', 'true');
    await expect(card.row.getByTestId('question-error-1')).toHaveText('Choose at least one option');
    expect(await second.evaluate((el) => el.contains(document.activeElement))).toBe(true);

    await card.press(second.getByRole('checkbox', { name: 'A' }));
    await card.press(second.getByRole('checkbox', { name: 'C' }));
    await expect(card.row.getByTestId('selected-count')).toHaveText('2 selected');
    await card.press(card.submit);

    await expect(card.deliveryText).toContainText('Answered:');
    await expect.poll(async () => (await readGuidanceRow(request, BASE_URL, seeded.rowId)).answer).toBe(`${cache}: Redis\n${regions}: A, C`);
  });

  test('answer_for_a_stopped_session_should_read_saved_and_list_under_saved_for_later', async ({ page, request }) => {
    const client = new SessionClient(BASE_URL);
    const session = await client.createSession({ title: unique('stopped-q'), path: '/tmp', program: 'bash' });
    const question = unique('Ship it?');
    await seedQuestionCard(request, BASE_URL, { sessionId: session.title, source: 'agent_mcp', questions: [{ text: question, options: ['Yes', 'No'] }] });
    await client.pauseSession(session.id);
    await openNotificationTray(page, BASE_URL);
    const card = new QuestionCard(page, session.title);
    await card.reveal();

    await card.press(card.row.getByRole('button', { name: 'Yes' }));

    await expect(card.deliveryText).toHaveText('Your answer is saved. The agent can read it once you resume the session.');
    const saved = page.getByTestId('saved-for-later');
    await expect(saved.getByRole('button', { name: /Saved for later \(\d+\)/ })).toHaveAttribute('aria-expanded', 'false');
    await saved.getByRole('button', { name: /Saved for later/ }).click();
    await expect(saved.getByTestId('saved-for-later-list')).toContainText(`${question} - Yes`);
    await expect(saved.getByRole('link', { name: 'Open session' }).first()).toBeVisible();
  });

  test('tabbed_answer_should_type_the_recorded_nine_keys_and_read_answered_once_the_dialog_is_gone', async ({ page, request }) => {
    const { card, keyFile } = await pickedTabbedCard(page, request, 'tabbed');

    await card.press(card.submit);

    await expect.poll(() => receivedKeys(keyFile), { message: 'the pane received the recorded key sequence', timeout: ACK_TIMEOUT }).toEqual(NINE_KEYS);
    await expect(card.deliveryText).toHaveText('Answered: Redis; A, B', { timeout: ACK_TIMEOUT });
    expect(receivedKeys(keyFile)).toEqual(NINE_KEYS); // still exactly nine
  });

  test('tabbed_answer_should_stop_and_pin_an_aborted_card_when_the_pane_changes_mid_sequence', async ({ page, request }) => {
    const { card, keyFile } = await pickedTabbedCard(page, request, 'tabbed-drift');

    await card.press(card.submit);

    await expect(card.deliveryText).toHaveText(/Part of your answer was sent, then the prompt changed/, { timeout: ACK_TIMEOUT });
    expect(receivedKeys(keyFile)).toEqual(NINE_KEYS.slice(0, 3)); // no key after the frame check failed
    await expect(card.submit).toHaveAttribute('aria-disabled', 'true');
    await expect(card.row.getByRole('link', { name: 'View Session' })).toBeVisible(); // the tray's Open session control
    await expect(card.row.getByTestId('action-delivery-retry')).toHaveCount(0);
    await expect(card.row.getByTestId('action-delivery-retryAnyway')).toHaveCount(0);
    await card.press(card.row.getByTestId('action-delivery-dismissConfirm'));
    await expect(card.row.getByTestId('delivery-confirm')).toContainText('Dismiss without finishing?');
  });
});
