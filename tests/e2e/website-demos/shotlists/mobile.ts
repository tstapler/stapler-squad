// @feature website-demos
// The phone-layout set (390x844, touch). Each shot is the `tour.phoneFigure` of exactly one
// tour (website/tests/unit/mobile-shots.test.ts keeps the two in step). `backlog` is off by
// default, so the recorder turns it on and reads it back first; `flagsEnabled` mirrors that.
import type { Page } from '@playwright/test';
import type { ShotListSpec, ShotSpec } from '../scene';

export const flagsEnabled: string[] = ['backlog'];

const CARD = '[data-testid="session-card"]';

const mobile = (spec: Omit<ShotSpec, 'formFactor'>): ShotSpec => ({ ...spec, formFactor: 'mobile' });

export const MOBILE_SHOTS: readonly ShotSpec[] = [
  mobile({ name: 'dashboard-mobile', route: '/', landmark: CARD }),
  mobile({ name: 'review-queue-mobile', route: '/review-queue', landmark: '[data-testid="review-queue"]' }),
  mobile({
    // Tapping a card opens its detail pane, whose toolbar is the mobile terminal chrome.
    name: 'terminal-mobile',
    route: '/',
    landmark: CARD,
    prepare: async (page: Page) => {
      await page.locator(CARD).filter({ hasText: 'refactor-session-store' }).first().tap();
      await page.locator('[data-testid="toolbar-toggle"]').waitFor();
    },
  }),
  mobile({ name: 'backlog-mobile', route: '/backlog/board', landmark: 'nav[aria-label="Backlog views"]' }),
];

export const shotlist: ShotListSpec = { name: 'mobile', flags: flagsEnabled, shots: [...MOBILE_SHOTS] };
