// @feature notification:scheduled-prompts, notification:peer-help, notification:remind-me, notification:error-exits
/**
 * Stories 4.6, 4.10 and 4.11 (operator surface for agent wake triggers and peer help): the
 * Background tab's Scheduled prompts section, its kill-switch banner, the peer-help rows with
 * their Answer for / Cancel request exits, and Remind me with Undo. WakeTriggerService is
 * fulfilled in the browser (see pages/ScheduledPromptsPage.ts for why); everything the
 * operator sees and touches is the real UI at the desktop and phone profiles.
 */
import { test, expect, type Page } from '@playwright/test';
import { BackgroundSegment, NotificationTray, openNotificationTray, clickWhenScrollIdle } from './pages/NotificationPanel';
import { ScheduledPromptsFake, ScheduledPromptsSection, type FakeTrigger } from './pages/ScheduledPromptsPage';
import { profiles } from './helpers/viewport-profiles';
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

async function openBackground(page: Page): Promise<{ tray: NotificationTray; section: ScheduledPromptsSection }> {
  const tray = await openNotificationTray(page, BASE_URL);
  await new BackgroundSegment(page).tab.click();
  return { tray, section: new ScheduledPromptsSection(page) };
}

const trigger = (id: string, ownerTitle: string, over: Partial<FakeTrigger> = {}): FakeTrigger => ({ id, ownerTitle, inMinutes: 25, ...over });

for (const key of ['V1', 'V2'] as const) {
  test.describe(`agent wake triggers (${profiles[key].name})`, () => {
    test.use(profiles[key].use);
    let fake: ScheduledPromptsFake;

    test.beforeEach(async ({ page }) => {
      fake = new ScheduledPromptsFake();
      await fake.install(page);
    });

    test('ux_ac35_36_scheduled_prompts_should_list_rows_and_confirm_inline_before_cancelling_with_a_polite_announcement', async ({ page }) => {
      fake.triggers = [trigger('w1', 'diagnose:abc'), trigger('w2', 'review:pr-1042', { triggerType: 'ci_event', condition: 'when PR #901 (o/r) CI passes', inMinutes: undefined })];
      const { section } = await openBackground(page);

      await section.expectLoaded();
      await expect(section.rows).toHaveCount(2);
      await expect(section.row('review:pr-1042')).toContainText('prompts again when PR #901 (o/r) CI passes');

      const first = section.row('diagnose:abc');
      await clickWhenScrollIdle(page, first.getByRole('button', { name: 'Cancel prompt' }));
      await expect(first).toContainText('Cancel this scheduled prompt? The agent will not be re-prompted.');
      await expect(first.getByRole('button', { name: 'Yes, cancel' })).toBeFocused();
      await clickWhenScrollIdle(page, first.getByRole('button', { name: 'Keep' }));
      expect(fake.calls.cancel).toEqual([]);

      await clickWhenScrollIdle(page, first.getByRole('button', { name: 'Cancel prompt' }));
      await clickWhenScrollIdle(page, first.getByRole('button', { name: 'Yes, cancel' }));
      await expect(section.rows).toHaveCount(1);
      await expect(section.announcer).toHaveText('Prompt cancelled');
      expect(fake.calls.cancel).toEqual(['w1']);
    });

    test('ux_ac37_scheduled_prompts_should_say_already_fired_without_an_alert_when_the_cancel_races', async ({ page }) => {
      fake.triggers = [trigger('w1', 'diagnose:abc')];
      fake.cancelOutcome = 'ALREADY_FIRED';
      const { section } = await openBackground(page);

      const row = section.row('diagnose:abc');
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Cancel prompt' }));
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Yes, cancel' }));

      await expect(row).toContainText(/Already fired at .+\./);
      await expect(page.getByRole('alert').filter({ hasText: /fired|cancel/i })).toHaveCount(0);
    });

    test('ux_ac55_scheduled_prompts_should_offer_retry_when_loading_or_cancelling_fails', async ({ page }) => {
      fake.listFails = true;
      const { section } = await openBackground(page);

      const failed = page.getByTestId('scheduled-prompts-error');
      await expect(failed).toContainText("Couldn't load scheduled prompts.");
      fake.listFails = false;
      fake.triggers = [trigger('w1', 'diagnose:abc')];
      await clickWhenScrollIdle(page, failed.getByRole('button', { name: 'Retry' }));
      await expect(section.rows).toHaveCount(1);

      fake.cancelOutcome = 'error';
      const row = section.row('diagnose:abc');
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Cancel prompt' }));
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Yes, cancel' }));
      await expect(row).toContainText("Couldn't cancel.");
      fake.cancelOutcome = 'CANCELLED';
      await clickWhenScrollIdle(page, row.getByRole('button', { name: 'Retry' }));
      await expect(section.rows).toHaveCount(0);
    });

    test('scheduled_prompts_should_cap_at_20_rows_with_show_all_and_mark_missed_rows_with_cancel_and_open_owner', async ({ page }) => {
      fake.triggers = [
        trigger('missed-1', 'review:pr-9', { status: 'missed', endReason: 'expired' }),
        ...Array.from({ length: 24 }, (_, i) => trigger(`w${i}`, `session-${i}`, { inMinutes: i + 1 })),
      ];
      const { section } = await openBackground(page);

      await expect(section.rows).toHaveCount(20);
      const missed = section.row('review:pr-9');
      await expect(missed).toContainText('Missed: it expired before it could be delivered');
      await expect(missed.getByRole('button', { name: 'Cancel prompt' })).toBeVisible();
      await expect(missed.getByRole('link', { name: 'Open owner' })).toBeVisible();
      await clickWhenScrollIdle(page, section.showAll);
      await expect(section.rows).toHaveCount(25);
    });

    test('ux_ac38_scheduled_prompts_should_show_paused_banner_and_resume_after_an_inline_confirm', async ({ page }) => {
      fake.enabled = false;
      fake.triggers = [trigger('w1', 'diagnose:abc')];
      const { section } = await openBackground(page);

      await expect(section.paused).toContainText('Scheduled prompts paused. Agents will not be prompted automatically.');
      await expect(section.paused.getByRole('link', { name: 'Open flag settings' })).toHaveAttribute('href', /\/settings\/features\/?$/);
      await expect(section.row('diagnose:abc')).toContainText('Paused');
      await expect(section.row('diagnose:abc').getByRole('button', { name: 'Cancel prompt' })).toBeVisible();

      await clickWhenScrollIdle(page, section.paused.getByRole('button', { name: 'Resume scheduled prompts' }));
      expect(fake.calls.flagWrites).toBe(0);
      await clickWhenScrollIdle(page, section.paused.getByRole('button', { name: 'Yes, resume' }));
      await expect(section.paused).toHaveCount(0);
      expect(fake.calls.flagWrites).toBe(1);
    });

    test('ux_ac31_peer_help_should_wait_quietly_with_open_links_and_no_action_buttons', async ({ page }) => {
      fake.peers = [{ id: 'p1' }];
      await openBackground(page);

      const waiting = page.getByTestId('peer-help-groups').getByTestId('peer-help-row').first();
      await expect(waiting).toContainText('review:pr-1042 asked diagnose:abc');
      await expect(waiting).toContainText('Waiting for diagnose:abc');
      await expect(waiting.getByRole('link', { name: 'Open asker' })).toBeVisible();
      await expect(waiting.getByRole('button')).toHaveCount(0);
    });

    test('ux_ac32_peer_help_should_offer_answer_for_cancel_request_and_open_with_retry_on_error', async ({ page }) => {
      fake.peers = [{ id: 'p1', escalated: true }];
      await openBackground(page);

      const stalled = page.getByTestId('peer-help-groups').getByTestId('peer-help-row').first();
      await expect(stalled).toContainText('no reply in 10m');
      await expect(stalled.getByRole('button', { name: 'Answer for diagnose:abc' })).toBeVisible();
      await expect(stalled.getByRole('link', { name: 'Open' })).toBeVisible();

      fake.actionOutcome = 'NOT_SENT';
      await clickWhenScrollIdle(page, stalled.getByRole('button', { name: 'Cancel request' }));
      await clickWhenScrollIdle(page, stalled.getByRole('button', { name: 'Yes, cancel' }));
      await expect(stalled).toContainText("Couldn't cancel the request.");
      fake.actionOutcome = 'APPLIED';
      await clickWhenScrollIdle(page, stalled.getByRole('button', { name: 'Retry' }));
      await expect.poll(() => fake.calls.actions.filter((a) => a.endsWith('APPLIED')).length).toBe(1);
    });

    test('ux_ac33_34_peer_help_should_show_not_sent_with_a_plain_reason_and_answered_rows_under_recent', async ({ page }) => {
      fake.peers = [
        { id: 'p1', state: 'not_sent', stateReason: 'peer no longer exists' },
        { id: 'p2', state: 'answered', answeredBy: 'diagnose:abc', answer: 'Use B' },
      ];
      await openBackground(page);

      await expect(page.getByTestId('peer-help-groups').getByTestId('peer-help-state').filter({ hasText: 'Not sent: peer no longer exists' })).toBeVisible();
      const recent = page.getByTestId('peer-help-recent');
      await expect(recent.getByTestId('peer-help-state')).toContainText('Answered by diagnose:abc');
    });
  });
}
