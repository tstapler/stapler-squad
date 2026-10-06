// @feature session:create, review-queue:get
/**
 * End-to-end coverage for the escalation-reasoning feature (Epic 6.1, AC8).
 *
 * Drives a real no-match escalation through the actual chain: HTTP hook ->
 * classifier -> ApprovalStore -> ReviewQueuePoller -> ReviewItem.Metadata ->
 * ReviewQueuePanel render. Unlike review-queue.spec.ts's UI-contract tests,
 * this spec creates a real session and posts directly to the hook endpoint
 * so the escalation-reason plumbing is proven end to end, not mocked.
 *
 * IMPORTANT — session identity: the review queue keys every ReviewItem by the
 * session's *title* (session/review_queue_poller.go's `ReviewItem.SessionID:
 * snap.Title`), not its UUID (`session.id` from CreateSession). Locators here
 * must use the title accordingly; using `session.id` never matches any
 * `review-item-*` testid.
 *
 * IMPORTANT — queue detection: this repo's ReviewQueuePoller only classifies
 * a session as ReasonApprovalPending by pattern-matching the session's
 * terminal PTY content (session/detection's NeedsApproval patterns, e.g.
 * "Do you want to proceed?") — there is no direct ApprovalStore -> queue
 * link. A real Claude Code process produces this text as a side effect of
 * printing its own permission dialog; a plain "bash" session does not, so
 * this spec writes the matching text via WriteToSession to simulate it.
 *
 * Prerequisites: test server started automatically by global-setup.ts.
 */

import { test, expect } from '@playwright/test';
import { SessionClient } from './helpers/session-client';
import type { ReviewQueue } from './helpers/session-client';
import { dismissOnboardingIfPresent } from './pages/OnboardingPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// A command guaranteed to match no seed classifier rule, so the hook falls
// through to the generic "No matching rule; escalated for manual review."
// no-match escalation path.
const UNMATCHED_COMMAND = 'totally-unmatched-cmd-xyz123 --flag';

/**
 * Poll the review queue until an item for `sessionTitle` appears. This is
 * session-scoped (not just a total-count check) so the assertion holds even
 * if another item is already sitting in the shared server's queue.
 */
async function waitForSessionInQueue(
  client: SessionClient,
  sessionTitle: string,
  timeoutMs = 20000,
): Promise<ReviewQueue> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const queue = await client.getReviewQueue();
    // protojson omits empty repeated fields, so `items` may be absent (not
    // just []) when the queue happens to be empty at poll time.
    if ((queue.items ?? []).some((item) => item.sessionId === sessionTitle)) {
      return queue;
    }
    if (Date.now() >= deadline) {
      throw new Error(
        `Session ${sessionTitle} did not appear in the review queue after ${timeoutMs}ms ` +
          `(queue had ${queue.totalItems} item(s))`,
      );
    }
    await new Promise((r) => setTimeout(r, 500));
  }
}

/**
 * Waits for the session's tmux-backed process to finish starting. WriteToSession
 * (send-keys) 500s with "cannot send keys to instance that has not been
 * started or is paused" if called while status is still Creating/Loading.
 */
async function waitForSessionStarted(
  client: SessionClient,
  sessionId: string,
  timeoutMs = 15000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const s = await client.getSession(sessionId);
    if (s.status !== 'SESSION_STATUS_CREATING' && s.status !== 'SESSION_STATUS_LOADING') {
      return;
    }
    if (Date.now() >= deadline) {
      throw new Error(`Session ${sessionId} still ${s.status} after ${timeoutMs}ms`);
    }
    await new Promise((r) => setTimeout(r, 300));
  }
}

/** Raw ConnectRPC JSON POST — mirrors SessionClient's private `rpc()` helper. */
async function rpc<T>(method: string, body: Record<string, unknown>): Promise<T> {
  const resp = await fetch(`${BASE_URL}/api/session.v1.SessionService/${method}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    throw new Error(`${method} failed: HTTP ${resp.status} ${await resp.text().catch(() => '')}`);
  }
  return resp.json() as Promise<T>;
}

/**
 * Writes text into the session's terminal PTY (as real keystrokes) so the
 * ReviewQueuePoller's content-pattern scanner detects a needs-approval
 * prompt, standing in for the permission dialog a real Claude Code process
 * would print. See file header for why this is necessary.
 */
async function simulateApprovalPrompt(sessionId: string): Promise<void> {
  await rpc('WriteToSession', {
    sessionId,
    input: "echo 'Do you want to proceed?'",
    pressEnter: true,
  });
}

/**
 * Safety-net cleanup used when the UI-driven deny click (the primary
 * cleanup path, per Task 6.1.1c) isn't reachable — e.g. an assertion failed
 * before the test ever navigated to /review-queue. Resolves the pending
 * approval directly via the ConnectRPC JSON endpoints so it is never left
 * for the ~4-minute server-side timeout to sweep up.
 */
async function resolvePendingApprovalIfAny(sessionId: string): Promise<void> {
  const { approvals } = await rpc<{ approvals?: Array<{ id: string }> }>('ListPendingApprovals', {
    sessionId,
  });
  for (const approval of approvals ?? []) {
    await rpc('ResolveApproval', {
      approvalId: approval.id,
      decision: 'deny',
      message: 'e2e cleanup',
    }).catch(() => {});
  }
}

test.describe('escalation-reasoning', () => {
  test('shows escalation reason for a real no-match hook escalation', async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const title = `escalation-e2e-${Date.now()}`;

    const session = await client.createSession({
      title,
      path: '/tmp',
      program: 'bash',
    });

    // Fire the hook POST as a backgrounded promise — the handler blocks
    // server-side (up to the ~4-minute approval timeout) until a decision is
    // made or it times out, so we must not await it on the main test flow.
    const postPromise = page.request.post('/api/hooks/permission-request', {
      headers: { 'X-CS-Session-ID': session.id },
      data: {
        session_id: `claude-session-${Date.now()}`,
        cwd: '/tmp',
        permission_mode: 'default',
        hook_event_name: 'PermissionRequest',
        tool_name: 'Bash',
        tool_input: { command: UNMATCHED_COMMAND },
      },
      // The handler blocks server-side for up to the ~4-minute approval
      // timeout; APIRequestContext's default request timeout (well under
      // that) would otherwise fire before we get a chance to resolve the
      // approval and let the response come back.
      timeout: 300000,
    });

    let assertionError: unknown;

    try {
      await waitForSessionStarted(client, session.id);
      await simulateApprovalPrompt(session.id);
      await waitForSessionInQueue(client, title);

      await page.goto(`${BASE_URL}/review-queue`, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await page.waitForSelector('[data-testid="review-queue-loaded"]', { timeout: 10000, state: 'attached' });

      // A fresh browser context (no prior localStorage) shows the first-run
      // onboarding modal on top of the page — it can appear a moment after
      // navigation (not synchronously), so this dismisses it before it can
      // intercept clicks on the review-queue card/buttons below. No-op on a
      // context that has already seen onboarding (modal never appears).
      await dismissOnboardingIfPresent(page);

      const card = page.getByTestId(`review-item-${title}`);
      await expect(card).toBeVisible({ timeout: 10000 });

      const reason = page.getByTestId(`escalation-reason-${title}`);
      await expect(reason).toBeVisible({ timeout: 10000 });
      // Verbatim backend copy (pkg/classifier/classifier.go's no-match fallback),
      // prefixed by the "no-match" ❓ emoji (ESCALATION_REASON_EMOJI in
      // ReviewQueuePanel.tsx) — WCAG 1.4.1, category isn't color-only.
      await expect(reason).toContainText('No matching rule; escalated for manual review.');
      await expect(reason).toContainText('❓');
    } catch (err) {
      assertionError = err;
    } finally {
      // Mandatory cleanup regardless of pass/fail (Task 6.1.1c): the e2e
      // suite shares one server instance across the whole run, so an
      // abandoned approval would pollute other specs' queue-state
      // assertions for up to the ~4-minute server-side timeout.
      const denyButton = page.getByTestId(`deny-${title}`);
      const denyVisible = await denyButton.isVisible().catch(() => false);
      if (denyVisible) {
        await denyButton.click().catch(() => {});
        await expect(page.getByTestId(`review-item-${title}`))
          .not.toBeAttached({ timeout: 10000 })
          .catch(() => {});
      } else {
        // UI path unavailable (e.g. an earlier assertion failed before we
        // ever reached /review-queue, or the Deny button never rendered) —
        // resolve directly via RPC instead.
        await resolvePendingApprovalIfAny(session.id).catch((e) => {
          console.error(`[escalation-reasoning] fallback resolve failed for session ${session.id}:`, e);
        });
      }

      // Confirm the hook itself actually returns a response — not just that
      // the queue item disappeared — i.e. ResolveApproval really unblocked
      // the still-pending HTTP handler.
      const response = await postPromise.catch((e) => {
        console.error('[escalation-reasoning] hook POST promise rejected:', e);
        return null;
      });

      await client.deleteSession(session.id, true).catch(() => {});

      if (assertionError) {
        throw assertionError;
      }

      expect(response?.ok()).toBe(true);
      const body = await response!.json();
      expect(body?.hookSpecificOutput?.decision?.behavior).toBe('deny');
    }
  });
});
