import { expect, type Locator, type Page } from '@playwright/test';

/**
 * The Background tab's Scheduled prompts section and peer-help rows (Stories 4.6, 4.10, 4.11).
 *
 * The shared test-mode instance boots with agent_wake_triggers off, so no wake dispatcher runs
 * and a peer-help row can only be created by an agent's MCP call. WakeTriggerService is
 * therefore fulfilled in the browser from `ScheduledPromptsFake`; the tray, the tab and every
 * component are real.
 */

const SERVICE = '**/api/session.v1.WakeTriggerService';
const ACTION_RPC = '**/api/session.v1.SessionService/InvokeNotificationAction';

export interface FakeTrigger {
  id: string;
  ownerTitle: string;
  status?: 'pending' | 'missed';
  endReason?: string;
  triggerType?: string;
  condition?: string;
  /** Minutes from now; omit for an event trigger. */
  inMinutes?: number;
  operatorReminder?: boolean;
}

export interface FakePeer {
  id: string;
  state?: 'waiting' | 'answered' | 'not_sent' | 'cancelled';
  stateReason?: string;
  escalated?: boolean;
  answeredBy?: string;
  answer?: string;
}

export class ScheduledPromptsFake {
  triggers: FakeTrigger[] = [];
  peers: FakePeer[] = [];
  enabled = true;
  /** What the next CancelWakeTrigger answers; 'error' makes it fail with a 503. */
  cancelOutcome: 'CANCELLED' | 'ALREADY_FIRED' | 'error' = 'CANCELLED';
  /** What the next peer-help action answers. */
  actionOutcome: 'APPLIED' | 'NOT_SENT' | 'ALREADY_RESOLVED' = 'APPLIED';
  listFails = false;
  reminderOutcome: 'CREATED' | 'CAP_EXCEEDED' | 'PAST_TIME' = 'CREATED';
  calls = { list: 0, cancel: [] as string[], actions: [] as string[], reminders: 0, flagWrites: 0 };

  private triggerJson(t: FakeTrigger) {
    const now = Date.now();
    return {
      id: t.id,
      ownerSessionUuid: `uuid-${t.id}`,
      ownerTitle: t.ownerTitle,
      ownerExists: true,
      triggerType: t.triggerType ?? 'at',
      condition: t.condition ?? '',
      actionKind: t.operatorReminder ? 'notify_operator' : 'steer_text',
      status: t.status ?? 'pending',
      endReason: t.endReason ?? '',
      message: 'check on the build',
      createdAt: new Date(now - 12 * 60_000).toISOString(),
      nextFireAt: t.inMinutes === undefined ? undefined : new Date(now + t.inMinutes * 60_000).toISOString(),
      operatorReminder: t.operatorReminder ?? false,
    };
  }

  private peerJson(p: FakePeer) {
    return {
      id: p.id,
      askerSessionUuid: 'uuid-asker',
      askerTitle: 'review:pr-1042',
      answererSessionUuid: 'uuid-peer',
      answererTitle: 'diagnose:abc',
      question: 'Which approach for the retry logic?',
      state: p.state ?? 'waiting',
      stateReason: p.stateReason ?? '',
      answer: p.answer ?? '',
      answeredBy: p.answeredBy ?? '',
      escalated: p.escalated ?? false,
      createdAt: new Date(Date.now() - 3 * 60_000).toISOString(),
    };
  }

  async install(page: Page): Promise<void> {
    await page.route(`${SERVICE}/ListWakeTriggers`, async (route) => {
      this.calls.list += 1;
      if (this.listFails) {
        return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) });
      }
      return route.fulfill({ json: { triggers: this.triggers.map((t) => this.triggerJson(t)), enabled: this.enabled } });
    });
    await page.route(`${SERVICE}/ListPeerHelpRequests`, (route) => route.fulfill({ json: { requests: this.peers.map((p) => this.peerJson(p)) } }));
    await page.route(`${SERVICE}/CancelWakeTrigger`, async (route) => {
      const { id } = (route.request().postDataJSON() ?? {}) as { id: string };
      this.calls.cancel.push(id);
      if (this.cancelOutcome === 'error') {
        return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) });
      }
      if (this.cancelOutcome === 'CANCELLED') this.triggers = this.triggers.filter((t) => t.id !== id);
      return route.fulfill({
        json: { outcome: `WAKE_CANCEL_OUTCOME_${this.cancelOutcome}`, deliveredAt: new Date(Date.UTC(2026, 9, 19, 13, 40)).toISOString() },
      });
    });
    await page.route(`${SERVICE}/CreateOperatorReminder`, (route) => {
      this.calls.reminders += 1;
      const outcome = `REMINDER_OUTCOME_${this.reminderOutcome}`;
      return route.fulfill({ json: { outcome, trigger: this.reminderOutcome === 'CREATED' ? this.triggerJson({ id: 'remind-1', ownerTitle: 'review', operatorReminder: true, inMinutes: 60 }) : undefined } });
    });
    await page.route(ACTION_RPC, (route) => {
      const body = (route.request().postDataJSON() ?? {}) as { actionKind?: string };
      if (body.actionKind !== 'ACTION_KIND_ANSWER_GUIDANCE' && body.actionKind !== 'ACTION_KIND_CANCEL_PEER_HELP') return route.continue();
      this.calls.actions.push(`${body.actionKind}:${this.actionOutcome}`);
      return route.fulfill({ json: { outcome: `ACTION_OUTCOME_${this.actionOutcome}` } });
    });
    await page.route('**/api/session.v1.SessionService/UpdateFeatureFlag', async (route) => {
      const body = (route.request().postDataJSON() ?? {}) as { name?: string; enabled?: boolean };
      if (body.name !== 'agent_wake_triggers') return route.continue();
      this.calls.flagWrites += 1;
      this.enabled = body.enabled === true;
      return route.fulfill({ json: { flag: { name: body.name, enabled: this.enabled, scopes: [] } } });
    });
  }
}

/** Locators for the section and its rows; all data-testid or role based. */
export class ScheduledPromptsSection {
  readonly section: Locator;
  readonly rows: Locator;
  readonly paused: Locator;
  readonly empty: Locator;
  readonly showAll: Locator;
  readonly peerRows: Locator;
  readonly announcer: Locator;

  constructor(private readonly page: Page) {
    this.section = page.getByTestId('scheduled-prompts');
    this.rows = page.getByTestId('scheduled-prompt-row');
    this.paused = page.getByTestId('scheduled-paused');
    this.empty = page.getByTestId('scheduled-empty');
    this.showAll = page.getByTestId('scheduled-show-all');
    this.peerRows = page.getByTestId('peer-help-row');
    this.announcer = page.getByTestId('announcer-polite');
  }

  row(owner: string): Locator {
    return this.rows.filter({ hasText: owner });
  }

  async expectLoaded(): Promise<void> {
    await expect(this.section).toBeVisible();
  }
}
