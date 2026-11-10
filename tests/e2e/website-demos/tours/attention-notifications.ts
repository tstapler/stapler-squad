// @feature website-demos
// Seeded by the `notifications` scenario (route fixture: SendNotification is a live push, not a stored row).
import { expect, type Page } from '@playwright/test';
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const CONTENT = '[data-testid="notifications-content"]';
const UNREAD = '[data-testid="notifications-unread-badge"]';
const RECENT_ACTIVITY = '[data-testid="collapsible-header-recent-activity"]';
const MARK_ALL_READ = '[data-testid="notifications-mark-all-read"]';

// Marking activity read leaves approvals and questions unread (they need a decision), so the badge
// drops to their count instead of disappearing. Idempotent: the clip has already marked the activity
// read by the time the shots (and a retry) open the page.
async function markAllRead(page: Page): Promise<void> {
  const button = page.locator(MARK_ALL_READ);
  if ((await button.count()) === 0) return;
  const before = await page.locator(UNREAD).textContent();
  await button.click();
  await expect(page.locator(UNREAD)).not.toHaveText(before ?? '');
}

export const scene: Scene = {
  name: 'attention-notifications',
  title: 'Know which agent needs you',
  landing: { path: '/notifications', landmark: CONTENT },
  quotes: ['Approval needed', 'Session finished', 'Waiting for input'],
  seeded: 'route',
  scenes: async (page, clip) => {
    await page.goto('/notifications');
    await page.locator(CONTENT).waitFor({ timeout: 30_000 });
    await clip.ready();
    await page.locator(RECENT_ACTIVITY).click(); // finished sessions sit in the collapsed section
    await caption(page, 'Approvals, finished work and idle agents, newest first');
    await hold(page, 800);

    await expect(page.locator(UNREAD)).toBeVisible();
    await caption(page, 'The unread count tells you how much is waiting');
    await hold(page, 800);

    await markAllRead(page);
    await caption(page, 'Mark the activity read; what needs a decision stays');
    await hold(page, 800);
  },
  shots: [
    { name: 'attention-notifications-1', route: '/notifications', landmark: CONTENT, annotations: [{ selector: CONTENT, n: 1 }] },
    { name: 'attention-notifications-2', route: '/notifications', landmark: UNREAD, annotations: [{ selector: UNREAD, n: 1 }] },
    {
      name: 'attention-notifications-3',
      route: '/notifications',
      landmark: CONTENT,
      prepare: markAllRead,
    },
  ],
};
