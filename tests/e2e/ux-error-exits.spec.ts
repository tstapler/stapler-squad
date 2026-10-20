// @feature notification:error-exits, notification:background-activity, notification:inline-actions, notification:invoke-action, session:guidance-structured, notification:scheduled-prompts, notification:peer-help, notification:remind-me
/**
 * UX-AC55 (design/ux.md section 10, plan Story 5.3): every error or terminal state in the error tables of
 * sections 1.3, 2.3, 3.3, 4.3, 5.3 and 6.3 offers an exit control, so the operator is never left with only a message.
 *
 * One table drives the spec. Every ux.md row has an id in UX_ROWS and exactly one disposition:
 *   - COVERED: the state is reached in the isolated instance and each listed exit control is asserted visible and enabled.
 *   - NO_EXIT_BY_DESIGN: ux.md's Exit cell names no control (none, "-", auto-recover), so there is nothing to assert.
 *   - MANUAL_ONLY_STATES: the state cannot be reached in the isolated instance; the reason is recorded, nothing is faked.
 * `ux_md_error_tables_should_match_the_rows_this_spec_knows` re-reads ux.md, so a row added, removed or reordered there
 * fails the spec until the table is updated.
 *
 * `reach` records how a state is produced. 'real' means the app and server produced it; 'stubbed-response' means the
 * real UI was driven and only the server's typed answer was fulfilled in the browser (the sibling specs do the same for
 * descriptors and wake-trigger RPCs). The server-side producers of those answers are Go tests, not this spec.
 */
import fs from 'fs';
import path from 'path';
import { test, expect, type APIRequestContext, type Locator, type Page } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import { DIALOG_QUESTION, answerGuidanceViaRpc, fakePaneSession, postToolUseInTerminal, seedApproval, seedGuidance, seedQuestionCard } from './helpers/notification-action-seed';
import { fakeHiddenSessions, hidden, seedFailureAt, type HiddenFixture } from './pages/HiddenSessionsFake';
import {
  BackgroundSegment,
  INLINE_ACTIONS_FLAG,
  InlineActionRow,
  QuestionCard,
  clickWhenScrollIdle,
  openNotificationTray,
  setFeatureFlag,
} from './pages/NotificationPanel';
import { ScheduledPromptsFake, ScheduledPromptsSection, type FakeTrigger } from './pages/ScheduledPromptsPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const STRUCTURED_FLAG = 'askuserquestion_structured';
const HOOK = 'ask_user_question_hook';
const UX_MD = path.resolve(__dirname, '../../project_plans/notification-actions-and-agent-messaging/design/ux.md');
const unique = (tag: string) => `${tag}-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
const STOPPED = 'SESSION_STATUS_STOPPED';

// ---------------------------------------------------------------------------
// The ux.md inventory: [id, lowercase start of the row's first cell with backticks removed]
// ---------------------------------------------------------------------------

type SectionId = '1.3' | '2.3' | '3.3' | '4.3' | '5.3' | '6.3';

const UX_ROWS: Record<SectionId, ReadonlyArray<readonly [string, string]>> = {
  '1.3': [
    ['1.3-poll-fails', 'session list poll fails'],
    ['1.3-offline', 'offline'],
    ['1.3-flag-off', 'flag off'],
    ['1.3-no-longer-failure', 'failure row for a session whose outcome'],
    ['1.3-unknown-role', 'hidden session with unknown role'],
    ['1.3-stopped-unknown', 'stopped hidden session whose outcome is unknown'],
    ['1.3-over-100-rows', '>100 rows'],
    ['1.3-failure-and-decision', 'row has failure and a pending decision'],
    ['1.3-first-load', 'first load, no cached data'],
    ['1.3-server-unreachable', 'server unreachable while the browser is online'],
  ],
  '2.3': [
    ['2.3-applied', 'applied'],
    ['2.3-already-resolved', 'already_resolved'],
    ['2.3-stale', 'stale'],
    ['2.3-not-sent', 'not_sent'],
    ['2.3-send-indeterminate', 'send_indeterminate'],
    ['2.3-request-timeout', 'request timeout'],
    ['2.3-reconnecting', 'reconnecting'],
    ['2.3-rate-limited', 'rate_limited'],
    ['2.3-forbidden', 'forbidden'],
    ['2.3-flag-off', 'flag_off'],
    ['2.3-invalid', 'invalid'],
    ['2.3-transient-network-failure', 'transient network failure'],
    ['2.3-duplicate-tap', 'duplicate tap'],
    ['2.3-unknown-action-kind', 'unknown action kind'],
    ['2.3-session-gone', 'session gone, no durable record'],
    ['2.3-actionable-unread', 'actionable unread item'],
  ],
  '3.3': [
    ['3.3-stale-agent-moved-on', 'stale: agent moved on'],
    ['3.3-answered-elsewhere', 'answered elsewhere'],
    ['3.3-session-stopped', 'session stopped, durable'],
    ['3.3-session-archived', 'session archived'],
    ['3.3-session-gone', 'session gone, no durable row'],
    ['3.3-transient-send-failure', 'transient send failure'],
    ['3.3-delivery-failure-not-sent', 'delivery failure to live pane'],
    ['3.3-delivery-uncertain', 'delivery uncertain'],
    ['3.3-unacknowledged', 'unacknowledged'],
    ['3.3-duplicate-submit', 'duplicate submit'],
    ['3.3-terminal-answer-race', 'terminal-answer race'],
    ['3.3-offline', 'offline (r11)'],
    ['3.3-over-8-options', '>8 options'],
    ['3.3-unrenderable-options', 'unrenderable options'],
    ['3.3-layout-drift-canary', 'layout-drift canary'],
    ['3.3-multi-question-aborted', 'multi-question aborted'],
    ['3.3-multi-select-unmeasured', 'multi-select unmeasured'],
    ['3.3-validation-no-option', 'validation: no option'],
    ['3.3-long-option-label', 'option label very long'],
  ],
  '4.3': [
    ['4.3-undeliverable', 'request accepted, then undeliverable'],
    ['4.3-peer-stopped', 'peer stopped'],
    ['4.3-wrong-answerer', 'wrong answerer'],
    ['4.3-answered-by-peer', 'operator answers after'],
    ['4.3-cancel-fails', 'cancel fails'],
    ['4.3-flag-off', 'flag off'],
  ],
  '5.3': [
    ['5.3-none-pending', 'none pending'],
    ['5.3-cancel-race', 'cancel race'],
    ['5.3-cancel-failed', 'cancel failed'],
    ['5.3-paused', 'agent_wake_triggers turned off'],
    ['5.3-missed', 'prompt missed after a restart'],
    ['5.3-owner-archived', 'owner session archived'],
    ['5.3-list-failed', 'list fetch failed'],
    ['5.3-many-rows', 'many rows'],
  ],
  '6.3': [
    ['6.3-create-fails', 'create fails'],
    ['6.3-cap-exceeded', 'flag off / cap exceeded'],
    ['6.3-time-in-past', 'picked time in past'],
    ['6.3-session-reaped', 'session reaped before fire'],
    ['6.3-duplicate-tap', 'duplicate tap'],
  ],
};

/** ux.md's Exit cell names no control (none, "-", auto-recover, fix-and-resubmit), so UX-AC55 asks for nothing. */
const NO_EXIT_BY_DESIGN: Readonly<Record<string, string>> = {
  '1.3-offline': 'Exit cell: auto-recover on reconnect; Open stays available but is not the row exit',
  '1.3-flag-off': 'Exit cell: none needed (legacy empty state)',
  '1.3-no-longer-failure': 'Exit cell: "-" (the pinned row moves to No failures reported on the next merge)',
  '2.3-applied': 'Exit cell: none (terminal success)',
  '2.3-already-resolved': 'Exit cell: none (the row moves to Recent)',
  '2.3-reconnecting': 'Exit cell: auto-recover; the 30 s "Can\'t reach the server. Retry" is the shared banner asserted by 1.3-server-unreachable',
  '2.3-duplicate-tap': 'Exit cell: "-" (idempotent second tap)',
  '3.3-answered-elsewhere': 'Exit cell: reload only when the API value is missing; no standing control',
  '3.3-duplicate-submit': 'Exit cell: "-" (idempotent)',
  '3.3-terminal-answer-race': 'Exit cell: "-" (the card flips to handled by live update)',
  '3.3-offline': 'Exit cell: auto-retry on reconnect',
  '3.3-over-8-options': 'Exit cell: "-" (a native select)',
  '3.3-validation-no-option': 'Exit cell: fix and resubmit (the inline error names the group; Submit is the only control)',
  '3.3-long-option-label': 'Exit cell: "-" (labels wrap)',
  '4.3-wrong-answerer': 'Exit cell: "-" (audit only, no UI change)',
  '4.3-answered-by-peer': 'Exit cell: none',
  '4.3-flag-off': 'Exit cell: "-" (rows hidden)',
  '5.3-none-pending': 'Exit cell: "-" (empty state)',
  '5.3-cancel-race': 'Exit cell: none (read-only "Already fired at")',
  '6.3-session-reaped': 'Exit cell: none (the reminder notification offers no Open by design)',
  '6.3-duplicate-tap': 'Exit cell: "-" (idempotent)',
};

/** States this isolated instance cannot reach without faking the producer. Not asserted; revisit on a real-device pass. */
export const MANUAL_ONLY_STATES: Readonly<Record<string, string>> = {
  '3.3-layout-drift-canary': 'The canary trips on a DialogMatch mismatch share above 50% over at least 5 attempts in 1 h across sessions (plan.md Task 3.3h, line 192), and only cards created afterwards degrade to read-only; five drifted answers on the isolated server would be a staged metric, not the real trigger',
  '3.3-multi-select-unmeasured': 'The unmeasured shape is the S1c FAIL fallback (plan.md Story 3.5); this build ships the measured multi-question path (the tabbed fixture answers it), so no state in this instance renders it',
  '3.3-unrenderable-options': 'NewQuestionPayload rejects an empty question list (session/domain/guidance_request_source.go:196), so seed-question cannot create a card with nothing to render, and the only other route is a stubbed descriptor',
};

// ---------------------------------------------------------------------------
// Exit controls
// ---------------------------------------------------------------------------

interface Exit {
  /** Accessible name; a string matches whole, as a button or a link. */
  name: string | RegExp;
}

const named = (name: string): Exit => ({ name: new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) });
const RETRY = named('Retry');
const DISMISS = named('Dismiss');
/** The tray's own Open link reads "View Session"; the action row's reads "Open session"; a Background row's reads "View output". */
const OPEN: Exit = { name: /^(Open session|View Session|View output|Open)$/ };

interface Ctx {
  page: Page;
  request: APIRequestContext;
  /** Run when the test ends, pass or fail (releases routes held open). */
  cleanup: Array<() => void | Promise<void>>;
}

interface ExitRow {
  id: string;
  reach: 'real' | 'stubbed-response';
  /** Gets the app into the state and returns the element that must hold the exits. */
  open: (ctx: Ctx) => Promise<Locator>;
  exits: Exit[];
  /** Test timeout for a state that needs real server time to arrive; defaults to the file's. */
  timeoutMs?: number;
}

async function expectExits(scope: Locator, row: ExitRow): Promise<void> {
  for (const exit of row.exits) {
    const control = scope.getByRole('button', { name: exit.name }).or(scope.getByRole('link', { name: exit.name })).first();
    await expect(control, `${row.id}: exit control ${exit.name}`).toBeVisible();
    await expect(control, `${row.id}: exit control ${exit.name} enabled`).toBeEnabled();
  }
}

// ---------------------------------------------------------------------------
// Section 1.3: Background feed
// ---------------------------------------------------------------------------

async function backgroundOf(page: Page): Promise<BackgroundSegment> {
  await openNotificationTray(page, BASE_URL);
  const background = new BackgroundSegment(page);
  await background.tab.click();
  return background;
}

const stoppedSessions = (prefix: string, n: number): HiddenFixture[] =>
  Array.from({ length: n }, (_, i) => hidden(`review:${prefix}-${i}`, STOPPED));

/** A failed hidden session on screen, then the hidden-session poll starts failing. */
async function failingPoll(ctx: Ctx, tag: string): Promise<BackgroundSegment> {
  const id = unique(`review:${tag}`);
  let mode: 'ok' | 'error' = 'ok';
  await fakeHiddenSessions(ctx.page, () => (mode === 'ok' ? [hidden(id)] : 'error'));
  await seedFailureAt(ctx.request, BASE_URL, id);
  const background = await backgroundOf(ctx.page);
  await expect(background.rows.filter({ hasText: id })).toHaveCount(1);
  mode = 'error';
  await clickWhenScrollIdle(ctx.page, background.refresh);
  return background;
}

const BACKGROUND_ROWS: ExitRow[] = [
  {
    id: '1.3-poll-fails',
    reach: 'stubbed-response',
    exits: [RETRY],
    open: async (ctx) => {
      const background = await failingPoll(ctx, 'bg-poll');
      await expect(background.error).toContainText("Couldn't refresh background sessions. Showing data from");
      return background.error;
    },
  },
  {
    id: '1.3-failure-and-decision',
    reach: 'stubbed-response',
    exits: [{ name: /^Needs you \(\d+\): open the Needs you item/ }],
    open: async ({ page, request }) => {
      const id = unique('review:bg-decision');
      await fakeHiddenSessions(page, () => [hidden(id)]);
      await seedFailureAt(request, BASE_URL, id);
      await seedApproval(request, BASE_URL, id, unique('decide-cmd'));
      const background = await backgroundOf(page);
      const row = background.rows.filter({ hasText: id });
      await expect(row).toHaveCount(1);
      return row;
    },
  },
  {
    id: '1.3-unknown-role',
    reach: 'stubbed-response',
    exits: [OPEN],
    open: async ({ page, request }) => {
      const id = unique('mystery-bg');
      await fakeHiddenSessions(page, () => [hidden(id)]);
      await seedFailureAt(request, BASE_URL, id);
      const background = await backgroundOf(page);
      const row = background.rows.filter({ hasText: id });
      await expect(row).toHaveCount(1);
      await expect(row).toContainText('other');
      return row;
    },
  },
  {
    id: '1.3-stopped-unknown',
    reach: 'stubbed-response',
    exits: [OPEN],
    open: async ({ page }) => {
      await fakeHiddenSessions(page, () => stoppedSessions(unique('bg-unk'), 1));
      const background = await backgroundOf(page);
      await expect(background.stateChips).toHaveText(['Stopped, outcome unknown']);
      return background.rows.first();
    },
  },
  {
    id: '1.3-over-100-rows',
    reach: 'stubbed-response',
    exits: [named('Show all (101)')],
    open: async ({ page }) => {
      await fakeHiddenSessions(page, () => stoppedSessions(unique('bg-cap'), 101));
      const background = await backgroundOf(page);
      await expect(background.rows).toHaveCount(100);
      return page.getByTestId('notification-tray');
    },
  },
  {
    id: '1.3-first-load',
    reach: 'stubbed-response',
    exits: [RETRY],
    open: async ({ page, cleanup }) => {
      let release: () => void = () => {};
      const gate = new Promise<void>((resolve) => (release = resolve));
      cleanup.push(release);
      await page.route('**/api/session.v1.SessionService/ListSessions', async (route) => {
        const body = (route.request().postDataJSON() ?? {}) as { hiddenOnly?: boolean };
        if (!body.hiddenOnly) return route.continue();
        await gate;
        return route.abort();
      });
      const background = await backgroundOf(page);
      await expect(background.connectivity).toContainText('Still loading.', { timeout: 15_000 });
      return background.connectivity;
    },
  },
  {
    id: '1.3-server-unreachable',
    reach: 'stubbed-response',
    exits: [RETRY],
    open: async (ctx) => {
      await ctx.page.clock.install();
      const background = await failingPoll(ctx, 'bg-down');
      await expect(background.connectivity).toContainText('Reconnecting to the server.');
      await ctx.page.clock.fastForward(31_000);
      await expect(background.connectivity).toContainText("Can't reach the server.");
      return background.error;
    },
  },
];

// ---------------------------------------------------------------------------
// Section 2.3: inline actions on a notification row (the typed ActionOutcome)
// ---------------------------------------------------------------------------

type OutcomeStub = { json: object } | 'abort' | 'hang';

/** A standalone guidance request, answered over RPC when the test ends: the server caps pending ones at 5. */
async function trackedGuidance(ctx: Ctx, question: string, options: string[]) {
  const guidance = await seedGuidance(ctx.request, BASE_URL, question, options);
  ctx.cleanup.push(() => answerGuidanceViaRpc(ctx.request, BASE_URL, guidance.id, options[0]).catch(() => {}));
  return guidance;
}

/**
 * A guidance row with two options; the first option is pressed through the real row and the server's
 * InvokeNotificationAction answer is replaced by `stub`. Returns the tray row.
 */
async function pressedWithAnswer(ctx: Ctx, stub: OutcomeStub, opts: { clock?: boolean } = {}): Promise<Locator> {
  const { page, cleanup } = ctx;
  const guidance = await trackedGuidance(ctx, unique('Ship it?'), ['Yes', 'No']);
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => (release = resolve));
  cleanup.push(release);
  await page.route('**/api/session.v1.SessionService/InvokeNotificationAction', async (route) => {
    if (stub === 'abort') return route.abort('failed');
    if (stub === 'hang') {
      await gate;
      return route.abort();
    }
    return route.fulfill({ json: stub.json });
  });
  if (opts.clock) await page.clock.install();
  await openNotificationTray(page, BASE_URL);
  const row = new InlineActionRow(page, guidance.question);
  await row.reveal();
  await row.press(row.option('Yes'));
  return row.row;
}

const outcome = (name: string, extra: object = {}): OutcomeStub => ({ json: { outcome: `ACTION_OUTCOME_${name}`, ...extra } });

const stubbed = (id: string, stub: OutcomeStub, exits: Exit[], opts: { clock?: boolean; ready?: (row: Locator) => Promise<void> } = {}): ExitRow => ({
  id,
  reach: 'stubbed-response',
  exits,
  open: async (ctx) => {
    const row = await pressedWithAnswer(ctx, stub, { clock: opts.clock });
    await opts.ready?.(row);
    return row;
  },
});

const INLINE_ROWS: ExitRow[] = [
  stubbed('2.3-stale', outcome('STALE', { staleReason: 'dialog_replaced' }), [OPEN], {
    ready: (row) => expect(row).toContainText('The session is now asking something else.'),
  }),
  stubbed('2.3-not-sent', outcome('NOT_SENT'), [RETRY, OPEN], {
    ready: (row) => expect(row).toContainText('Not sent. Nothing was typed into the session.'),
  }),
  stubbed('2.3-send-indeterminate', outcome('SEND_INDETERMINATE'), [OPEN, named('Retry anyway')], {
    ready: (row) => expect(row).toContainText("Sent, but we couldn't confirm it arrived."),
  }),
  stubbed('2.3-request-timeout', 'hang', [RETRY, OPEN], {
    clock: true,
    ready: async (row) => {
      await row.page().clock.fastForward(10_500);
      await expect(row.getByTestId('pending-notice')).toContainText('Still sending...');
      // The transport's own 30 s deadline fails the request at the same moment the row would print "No response";
      // the row then reads one or the other, and both carry Retry and Open.
      await row.page().clock.fastForward(20_000);
      await expect(row).toContainText(/No response from the server\.|Couldn't send your action\./);
    },
  }),
  stubbed('2.3-rate-limited', outcome('RATE_LIMITED', { retryAfterSeconds: 2 }), [{ name: /^Yes$/ }], {
    ready: (row) => expect(row).toContainText('Too many actions. Try again in'),
  }),
  stubbed('2.3-forbidden', outcome('FORBIDDEN', { detail: 'desktop_only' }), [OPEN], {
    ready: (row) => expect(row).toContainText('Answer from desktop'),
  }),
  stubbed('2.3-flag-off', outcome('FLAG_OFF'), [OPEN], {
    ready: (row) => expect(row).toContainText('This action is turned off.'),
  }),
  stubbed('2.3-invalid', outcome('INVALID'), [named('Refresh')], {
    ready: (row) => expect(row).toContainText("That action isn't valid anymore."),
  }),
  stubbed('2.3-transient-network-failure', 'abort', [RETRY, DISMISS], {
    ready: (row) => expect(row).toContainText("Couldn't send your action."),
  }),
  stubbed('2.3-session-gone', outcome('STALE', { staleReason: 'session_ended' }), [OPEN], {
    ready: (row) => expect(row).toContainText('Session ended before this was answered.'),
  }),
  {
    id: '2.3-unknown-action-kind',
    reach: 'stubbed-response',
    exits: [OPEN],
    open: async (ctx) => {
      const { page } = ctx;
      const guidance = await trackedGuidance(ctx, unique('Mystery action?'), ['Yes']);
      await page.route('**/api/session.v1.SessionService/GetNotificationHistory', async (route) => {
        const response = await route.fetch();
        const body = (await response.json()) as { notifications?: { title?: string; actions?: object[] }[] };
        for (const n of body.notifications ?? []) {
          if (n.title === guidance.question) {
            n.actions = [{ kind: 'ACTION_KIND_UNSPECIFIED', label: 'Mystery', tier: 'ACTION_TIER_A', available: true, sessionId: 'unknown-kind-session' }];
          }
        }
        await route.fulfill({ response, json: body });
      });
      await openNotificationTray(page, BASE_URL);
      const row = new InlineActionRow(page, guidance.question);
      await row.reveal();
      return row.row;
    },
  },
  {
    id: '2.3-actionable-unread',
    reach: 'real',
    exits: [OPEN, { name: /^Yes$/ }],
    open: async (ctx) => {
      const { page } = ctx;
      const guidance = await trackedGuidance(ctx, unique('Still open?'), ['Yes', 'No']);
      await openNotificationTray(page, BASE_URL);
      const row = new InlineActionRow(page, guidance.question);
      await row.reveal();
      return row.row;
    },
  },
];

// ---------------------------------------------------------------------------
// Section 3.3: question cards
// ---------------------------------------------------------------------------

const HOOK_QUESTION = [{ ...DIALOG_QUESTION, multiSelect: false }];

/** A fake-pane session, its hook question card revealed in the tray. */
async function hookCard(page: Page, request: APIRequestContext, mode = ''): Promise<{ card: QuestionCard; title: string }> {
  const { title } = await fakePaneSession(new SessionClient(BASE_URL), unique(mode || 'fake-pane'), mode);
  await seedQuestionCard(request, BASE_URL, { sessionId: title, source: HOOK, questions: HOOK_QUESTION });
  await openNotificationTray(page, BASE_URL);
  const card = new QuestionCard(page, title);
  await card.reveal();
  return { card, title };
}

/** An agent-created card (a stored answer) on a plain bash session. */
async function agentCard(page: Page, request: APIRequestContext): Promise<{ card: QuestionCard; session: { id: string; title: string } }> {
  const session = await new SessionClient(BASE_URL).createSession({ title: unique('agent-q'), path: '/tmp', program: 'bash' });
  await seedQuestionCard(request, BASE_URL, { sessionId: session.title, source: 'agent_mcp', questions: [{ text: unique('Ship it?'), options: ['Yes', 'No'] }] });
  await openNotificationTray(page, BASE_URL);
  const card = new QuestionCard(page, session.title);
  await card.reveal();
  return { card, session };
}

const stubInvoke = async (page: Page, stub: OutcomeStub) =>
  page.route('**/api/session.v1.SessionService/InvokeNotificationAction', (route) =>
    stub === 'abort' ? route.abort('failed') : stub === 'hang' ? route.abort() : route.fulfill({ json: stub.json }),
  );

const CARD_ROWS: ExitRow[] = [
  {
    id: '3.3-stale-agent-moved-on',
    reach: 'real',
    exits: [OPEN],
    open: async ({ page, request }) => {
      const { card, title } = await hookCard(page, request);
      await postToolUseInTerminal(request, BASE_URL, title);
      await expect(card.deliveryText).toHaveText('Already handled in the session. No action needed.', { timeout: 15_000 });
      return card.row;
    },
  },
  {
    id: '3.3-session-stopped',
    reach: 'real',
    exits: [OPEN],
    open: async ({ page, request }) => {
      const { card, session } = await agentCard(page, request);
      await new SessionClient(BASE_URL).pauseSession(session.id);
      await card.press(card.row.getByRole('button', { name: 'Yes' }));
      await expect(card.deliveryText).toHaveText('Your answer is saved. The agent can read it once you resume the session.');
      return card.row;
    },
  },
  {
    id: '3.3-session-archived',
    reach: 'real',
    exits: [OPEN],
    open: async ({ page, request }) => {
      const { card, session } = await agentCard(page, request);
      const archived = await request.post(`${BASE_URL}/api/session.v1.SessionService/ArchiveSession`, { headers: { 'Content-Type': 'application/json' }, data: { sessionId: session.id } });
      expect(archived.ok(), `ArchiveSession: ${archived.status()} ${await archived.text()}`).toBe(true);
      await card.press(card.row.getByRole('button', { name: 'Yes' }));
      await expect(card.deliveryText).toHaveText('This session is archived. Your answer will be saved but not delivered unless you resume it.');
      return card.row;
    },
  },
  {
    id: '3.3-session-gone',
    reach: 'real',
    exits: [OPEN, DISMISS],
    open: async ({ page, request }) => {
      const { card, title } = await hookCard(page, request);
      const sessions = await new SessionClient(BASE_URL).listSessions();
      await new SessionClient(BASE_URL).deleteSession(sessions.find((s) => s.title === title)!.id, true);
      await expect(card.deliveryText).toHaveText('Session ended before this was answered.', { timeout: 20_000 });
      return card.row;
    },
  },
  {
    id: '3.3-transient-send-failure',
    reach: 'stubbed-response',
    exits: [RETRY, DISMISS],
    open: async ({ page, request }) => {
      const { card } = await agentCard(page, request);
      await stubInvoke(page, 'abort');
      await card.press(card.row.getByRole('button', { name: 'Yes' }));
      await expect(card.deliveryText).toHaveText("Couldn't send your answer. Retry");
      return card.row;
    },
  },
  {
    id: '3.3-delivery-failure-not-sent',
    reach: 'stubbed-response',
    exits: [RETRY, OPEN],
    open: async ({ page, request }) => {
      const { card } = await hookCard(page, request);
      await stubInvoke(page, outcome('NOT_SENT'));
      await card.press(card.option('Red'));
      await expect(card.deliveryText).toContainText('Not sent. Nothing was typed into the session.');
      return card.row;
    },
  },
  {
    id: '3.3-delivery-uncertain',
    reach: 'real',
    exits: [OPEN, named('Retry anyway')],
    open: async ({ page, request }) => {
      const { card } = await hookCard(page, request, 'sticky');
      await card.press(card.option('Red'));
      await expect(card.deliveryText).toHaveText(/couldn't confirm it arrived/, { timeout: 30_000 });
      return card.row;
    },
  },
  {
    id: '3.3-unacknowledged',
    reach: 'real',
    exits: [OPEN, named('Retry anyway')],
    // The server's 60 s acknowledgement window has to elapse; nothing client-side shortens it.
    timeoutMs: 150_000,
    open: async ({ page, request }) => {
      const { card } = await hookCard(page, request, 'sticky');
      await card.press(card.option('Red'));
      await expect(card.deliveryText).toHaveText('Sent, but the question is still showing. Check the session before retrying.', { timeout: 100_000 });
      return card.row;
    },
  },
  {
    id: '3.3-multi-question-aborted',
    reach: 'real',
    exits: [OPEN, named('Dismiss')],
    open: async ({ page, request }) => {
      const questions = [
        { text: 'Which cache?', header: 'Cache', multiSelect: false, options: ['Redis', 'Memcached', 'None'] },
        { text: 'Which letters?', header: 'Letters', multiSelect: true, options: ['A', 'B', 'C'] },
      ];
      const { title } = await fakePaneSession(new SessionClient(BASE_URL), unique('tabbed-drift'), 'tabbed-drift');
      await seedQuestionCard(request, BASE_URL, { sessionId: title, source: HOOK, questions });
      await openNotificationTray(page, BASE_URL);
      const card = new QuestionCard(page, title);
      await card.reveal();
      await card.press(card.group(/Which cache\?/).getByRole('button', { name: 'Redis' }));
      for (const letter of ['A', 'B']) await card.press(card.group(/Which letters\?/).getByRole('checkbox', { name: letter }));
      await card.press(card.submit);
      await expect(card.deliveryText).toHaveText(/Part of your answer was sent, then the prompt changed/, { timeout: 30_000 });
      return card.row;
    },
  },
];

// ---------------------------------------------------------------------------
// Sections 4.3, 5.3 and 6.3: peer help, Scheduled prompts, Remind me
// ---------------------------------------------------------------------------

const trigger = (id: string, ownerTitle: string, over: Partial<FakeTrigger> = {}): FakeTrigger => ({ id, ownerTitle, inMinutes: 25, ...over });

/** The Background tab with WakeTriggerService fulfilled from `fake`. */
async function wakeSurface(page: Page, fake: ScheduledPromptsFake): Promise<ScheduledPromptsSection> {
  await fake.install(page);
  await backgroundOf(page);
  return new ScheduledPromptsSection(page);
}

const peerRow = (page: Page) => page.getByTestId('peer-help-groups').getByTestId('peer-help-row').first();

/** Cancel (confirmed) on the first scheduled prompt, which the fake answers with an error. */
async function failedCancel(page: Page, fake: ScheduledPromptsFake, owner: string): Promise<Locator> {
  fake.triggers = [trigger('w1', owner)];
  fake.cancelOutcome = 'error';
  const section = await wakeSurface(page, fake);
  const row = section.row(owner);
  await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Cancel prompt' }));
  await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Yes, cancel' }));
  await expect(row).toContainText("Couldn't cancel.");
  return row;
}

/** A finished (stopped) hidden session's row, with Remind me offered because wake triggers are on. */
async function remindMeRow(page: Page, fake: ScheduledPromptsFake): Promise<Locator> {
  await page.route('**/api/session.v1.SessionService/GetFeatureFlags', async (route) => {
    const response = await route.fetch();
    const body = (await response.json()) as { flags?: { name: string }[] };
    const flags = (body.flags ?? []).filter((f) => f.name !== 'agent_wake_triggers');
    flags.push({ name: 'agent_wake_triggers', enabled: true, scopes: [] } as { name: string });
    await route.fulfill({ response, json: { ...body, flags } });
  });
  await fakeHiddenSessions(page, () => stoppedSessions(unique('bg-remind'), 1));
  await wakeSurface(page, fake);
  const row = new BackgroundSegment(page).rows.first();
  await clickWhenScrollIdle(page, row.getByTestId('remind-me-button'));
  return row;
}

const WAKE_ROWS: ExitRow[] = [
  {
    id: '4.3-undeliverable',
    reach: 'stubbed-response',
    exits: [named('Open asker')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.peers = [{ id: 'p1', state: 'not_sent', stateReason: 'peer no longer exists' }];
      await wakeSurface(page, fake);
      const row = peerRow(page);
      await expect(row).toContainText('Not sent: peer no longer exists');
      return row;
    },
  },
  {
    id: '4.3-peer-stopped',
    reach: 'stubbed-response',
    exits: [named('Answer for diagnose:abc'), named('Cancel request'), named('Open')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.peers = [{ id: 'p1', escalated: true }];
      await wakeSurface(page, fake);
      const row = peerRow(page);
      await expect(row).toContainText('no reply in 10m');
      return row;
    },
  },
  {
    id: '4.3-cancel-fails',
    reach: 'stubbed-response',
    exits: [RETRY, DISMISS],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.peers = [{ id: 'p1', escalated: true }];
      fake.actionOutcome = 'NOT_SENT';
      await wakeSurface(page, fake);
      const row = peerRow(page);
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Cancel request' }));
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Yes, cancel' }));
      await expect(row).toContainText("Couldn't cancel the request.");
      return row;
    },
  },
  {
    id: '5.3-cancel-failed',
    reach: 'stubbed-response',
    exits: [RETRY, DISMISS],
    open: ({ page }) => failedCancel(page, new ScheduledPromptsFake(), 'diagnose:abc'),
  },
  {
    id: '5.3-paused',
    reach: 'stubbed-response',
    exits: [named('Resume scheduled prompts'), named('Open flag settings')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.enabled = false;
      fake.triggers = [trigger('w1', 'diagnose:abc')];
      const section = await wakeSurface(page, fake);
      await expect(section.paused).toContainText('Scheduled prompts paused.');
      return section.paused;
    },
  },
  {
    id: '5.3-missed',
    reach: 'stubbed-response',
    exits: [named('Cancel prompt'), named('Open owner')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.triggers = [trigger('m1', 'review:pr-9', { status: 'missed', endReason: 'expired' })];
      const section = await wakeSurface(page, fake);
      const row = section.row('review:pr-9');
      await expect(row).toContainText('Missed: it expired before it could be delivered');
      return row;
    },
  },
  {
    id: '5.3-owner-archived',
    reach: 'stubbed-response',
    exits: [named('Cancel prompt')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.triggers = [trigger('a1', 'review:pr-7', { ownerArchived: true })];
      const section = await wakeSurface(page, fake);
      const row = section.row('review:pr-7');
      await expect(row).toContainText('Owner archived. This prompt will not revive it.');
      return row;
    },
  },
  {
    id: '5.3-list-failed',
    reach: 'stubbed-response',
    exits: [RETRY],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.listFails = true;
      await wakeSurface(page, fake);
      const failed = page.getByTestId('scheduled-prompts-error');
      await expect(failed).toContainText("Couldn't load scheduled prompts.");
      return failed;
    },
  },
  {
    id: '5.3-many-rows',
    reach: 'stubbed-response',
    exits: [named('Show all (24)')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.triggers = Array.from({ length: 24 }, (_, i) => trigger(`w${i}`, `session-${i}`, { inMinutes: i + 1 }));
      const section = await wakeSurface(page, fake);
      await expect(section.rows).toHaveCount(20);
      return section.section;
    },
  },
  {
    id: '6.3-create-fails',
    reach: 'stubbed-response',
    exits: [RETRY, DISMISS],
    open: async ({ page }) => {
      const row = await remindMeRow(page, new ScheduledPromptsFake());
      await page.route('**/api/session.v1.WakeTriggerService/CreateOperatorReminder', (route) =>
        route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) }),
      );
      await clickWhenScrollIdle(page, row.getByTestId('remind-me-hour'));
      await expect(row).toContainText("Couldn't set the reminder.");
      return row;
    },
  },
  {
    id: '6.3-cap-exceeded',
    reach: 'stubbed-response',
    exits: [named('Scheduled prompts')],
    open: async ({ page }) => {
      const fake = new ScheduledPromptsFake();
      fake.reminderOutcome = 'CAP_EXCEEDED';
      const row = await remindMeRow(page, fake);
      await clickWhenScrollIdle(page, row.getByTestId('remind-me-hour'));
      const cap = row.getByTestId('remind-me-cap');
      await expect(cap).toContainText('Too many pending reminders.');
      return cap;
    },
  },
  {
    id: '6.3-time-in-past',
    reach: 'stubbed-response',
    exits: [named('Keep')],
    open: async ({ page }) => {
      const row = await remindMeRow(page, new ScheduledPromptsFake());
      await clickWhenScrollIdle(page, row.getByTestId('remind-me-pick'));
      await row.getByLabel('Reminder time').fill('2020-01-01T10:00');
      await clickWhenScrollIdle(page, row.getByTestId('remind-me-set-pick'));
      await expect(row).toContainText('Choose a time in the future');
      return row;
    },
  },
];

// ---------------------------------------------------------------------------
// The spec
// ---------------------------------------------------------------------------

const COVERED: ExitRow[] = [...BACKGROUND_ROWS, ...INLINE_ROWS, ...CARD_ROWS, ...WAKE_ROWS];

test.setTimeout(90_000);

test.beforeAll(async ({ request }) => {
  await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, true);
  await setFeatureFlag(request, BASE_URL, STRUCTURED_FLAG, true);
});

test.afterAll(async ({ request }) => {
  // The shared server outlives this file; other specs expect the legacy buttons.
  await setFeatureFlag(request, BASE_URL, INLINE_ACTIONS_FLAG, false);
  await setFeatureFlag(request, BASE_URL, STRUCTURED_FLAG, false);
});

/** First cells of the rows of the table under the `### <section> ` heading in ux.md. */
function uxTableFirstCells(markdown: string, section: SectionId): string[] {
  const lines = markdown.split('\n');
  const start = lines.findIndex((l) => l.startsWith(`### ${section} `));
  expect(start, `ux.md has a "### ${section}" heading`).toBeGreaterThanOrEqual(0);
  const cells: string[] = [];
  let inTable = false;
  for (const line of lines.slice(start + 1)) {
    if (line.startsWith('|')) {
      inTable = true;
      cells.push(line.split('|')[1].trim().replace(/`/g, '').toLowerCase());
    } else if (inTable) {
      break;
    } else if (line.startsWith('#')) {
      break;
    }
  }
  return cells.slice(2); // header and separator
}

test('ux_md_error_tables_should_match_the_rows_this_spec_knows', () => {
  const markdown = fs.readFileSync(UX_MD, 'utf8');
  for (const section of Object.keys(UX_ROWS) as SectionId[]) {
    const known = UX_ROWS[section];
    const actual = uxTableFirstCells(markdown, section);
    expect(actual.length, `ux.md ${section} has ${actual.length} rows, this spec lists ${known.length}`).toBe(known.length);
    known.forEach(([id, start], i) => expect(actual[i], `${id} is row ${i + 1} of ux.md ${section}`).toMatch(new RegExp(`^${start.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`)));
  }
});

test('every_ux_md_error_row_should_be_covered_or_declared_without_an_exit_or_manual_only', () => {
  const covered = new Set(COVERED.map((r) => r.id));
  const noExit = new Set(Object.keys(NO_EXIT_BY_DESIGN));
  const manual = new Set(Object.keys(MANUAL_ONLY_STATES));
  const all = Object.values(UX_ROWS).flatMap((rows) => rows.map(([id]) => id));

  expect(new Set(all).size, 'row ids are unique').toBe(all.length);
  const dispositions = (id: string) => [covered.has(id), noExit.has(id), manual.has(id)].filter(Boolean).length;
  expect(all.filter((id) => dispositions(id) !== 1), 'rows with no disposition, or with more than one').toEqual([]);
  expect([...covered, ...noExit, ...manual].filter((id) => !all.includes(id)), 'dispositions naming a row ux.md does not have').toEqual([]);
  expect(COVERED.every((r) => r.exits.length > 0), 'a covered row asserts at least one exit').toBe(true);
  expect(Object.values(MANUAL_ONLY_STATES).every((reason) => reason.length > 20), 'a manual-only row says why').toBe(true);
});

for (const row of COVERED) {
  test(`ux_ac55_${row.id}_should_offer_an_enabled_exit_control`, async ({ page, request }) => {
    if (row.timeoutMs) test.setTimeout(row.timeoutMs);
    const cleanup: Array<() => void | Promise<void>> = [];
    try {
      test.info().annotations.push({ type: 'reach', description: row.reach });
      const scope = await row.open({ page, request, cleanup });
      await expectExits(scope, row);
    } finally {
      for (const fn of cleanup) await fn();
    }
  });
}
