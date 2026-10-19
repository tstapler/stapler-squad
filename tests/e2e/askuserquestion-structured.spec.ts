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
import os from 'os';
import path from 'path';
import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { postToolUseInTerminal, readGuidanceRow, seedQuestionCard } from './helpers/notification-action-seed';
import { INLINE_ACTIONS_FLAG, QuestionCard, openNotificationTray, setFeatureFlag } from './pages/NotificationPanel';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const STRUCTURED_FLAG = 'askuserquestion_structured';
const FAKE_PANE = path.resolve(__dirname, 'fixtures/fake-question-pane.sh');
const HOOK = 'ask_user_question_hook';
// The captured dialog's own question and labels (session/testdata/reply_dialogs/question_single_select_3opts.txt).
const DIALOG_QUESTION = { text: 'Which color should the spike use?', header: 'Color', options: ['Red', 'Green', 'Blue'] };
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

/** A session whose pane shows the fixture dialog; `keyFile` receives the key it is sent. */
async function fakePaneSession(client: SessionClient, title: string): Promise<{ title: string; keyFile: string }> {
  const keyFile = path.join(os.tmpdir(), `ssq-fake-pane-${title}.key`);
  await client.createSession({ title, path: '/tmp', program: `bash ${FAKE_PANE} ${keyFile}` });
  return { title, keyFile };
}

const receivedKey = (keyFile: string) => (fs.existsSync(keyFile) ? fs.readFileSync(keyFile, 'utf8') : '');

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
});
