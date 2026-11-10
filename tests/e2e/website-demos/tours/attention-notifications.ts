// @feature website-demos
// Seeded by the `notifications` scenario (route fixture: SendNotification is a live push, not a stored row).
import { expect } from '@playwright/test';
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

const CONTENT = '[data-testid="notifications-content"]';
const UNREAD = '[data-testid="notifications-unread-badge"]';
const MARK_ALL_READ = '[data-testid="notifications-mark-all-read"]';

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
    await caption(page, 'Approvals, finished work and idle agents, newest first');
    await hold(page, 800);

    await expect(page.locator(UNREAD)).toBeVisible();
    await caption(page, 'The unread count tells you how much is waiting');
    await hold(page, 800);

    await page.locator(MARK_ALL_READ).click();
    await expect(page.locator(UNREAD)).toBeHidden();
    await caption(page, 'Mark everything read when you have caught up');
    await hold(page, 800);
  },
  shots: [
    { name: 'attention-notifications-1', route: '/notifications', landmark: CONTENT, annotations: [{ selector: CONTENT, n: 1 }] },
    { name: 'attention-notifications-2', route: '/notifications', landmark: UNREAD, annotations: [{ selector: UNREAD, n: 1 }] },
    {
      name: 'attention-notifications-3',
      route: '/notifications',
      landmark: CONTENT,
      prepare: async (page) => {
        await page.locator(MARK_ALL_READ).click();
        await expect(page.locator(UNREAD)).toBeHidden();
      },
    },
  ],
};
