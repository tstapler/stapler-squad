import type { Page } from '@playwright/test';

// Closes anything that can steal keyboard focus from the session list: the
// Notification Panel (if auto-opened) and any visible toast alerts. Both
// fire on the same "session creation failed" event accessibility.spec.ts's
// Cancel/Retry test provokes, and either can intercept a Tab-key walk before
// it reaches the session list (BUG-097). Call before (and periodically
// during) any Tab walk that exercises the session list.
export async function dismissNotificationInterference(page: Page): Promise<void> {
  const panelClose = page.getByRole('button', { name: 'Close notification panel' });
  if (await panelClose.isVisible().catch(() => false)) {
    await panelClose.click().catch(() => {});
  }

  // Toasts can stack; dismiss all currently visible ones. Bounded at 5
  // (independent of any caller's own retry count) so a stuck/reappearing
  // toast can't hang the test -- warn rather than fail if it's still there,
  // since this is best-effort mitigation, not the assertion under test.
  const dismissButtons = page.getByRole('alert').getByRole('button', { name: /Dismiss|Close notification/ });
  for (let i = 0; i < 5; i++) {
    if ((await dismissButtons.count()) === 0) return;
    await dismissButtons.first().click().catch(() => {});
  }
  if ((await dismissButtons.count()) > 0) {
    console.warn('[a11y] dismissNotificationInterference: a toast is still visible after 5 dismiss attempts');
  }
}
