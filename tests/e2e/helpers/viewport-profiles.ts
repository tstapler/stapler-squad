import type { Page } from '@playwright/test';
import { devices } from '@playwright/test';

/**
 * The four viewport profiles every notification surface is verified at
 * (design/ux.md XA-1, XA-13). The playwright config's projects are all 1280x800,
 * so a spec opts in per `describe`: `test.use(profiles.V2.use)`.
 */
export interface ViewportProfile {
  name: string;
  /** Spread into `test.use()`. */
  use: {
    viewport: { width: number; height: number };
    isMobile: boolean;
    hasTouch: boolean;
    userAgent?: string;
  };
}

const phone = devices['Pixel 7'];

export const V1_DESKTOP: ViewportProfile = {
  name: 'V1-desktop',
  use: { viewport: { width: 1280, height: 800 }, isMobile: false, hasTouch: false },
};

export const V2_PORTRAIT: ViewportProfile = {
  name: 'V2-portrait',
  use: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, userAgent: phone.userAgent },
};

export const V3_LANDSCAPE: ViewportProfile = {
  name: 'V3-landscape',
  use: { viewport: { width: 844, height: 390 }, isMobile: true, hasTouch: true, userAgent: phone.userAgent },
};

/** V2 with the soft keyboard open: apply with `setKeyboardOpen` after navigation. */
export const V4_PORTRAIT_KEYBOARD: ViewportProfile = {
  name: 'V4-portrait-keyboard',
  use: V2_PORTRAIT.use,
};

export const profiles = {
  V1: V1_DESKTOP,
  V2: V2_PORTRAIT,
  V3: V3_LANDSCAPE,
  V4: V4_PORTRAIT_KEYBOARD,
} as const;

/**
 * Opens or closes the soft keyboard through the same signal ViewportProvider
 * consumes: the visual viewport's height shrinking. Playwright cannot raise a real
 * keyboard, so this overrides `window.visualViewport.height` and fires its resize
 * event; the real-keyboard leg is the operator's DV-3.
 */
export async function setKeyboardOpen(page: Page, heightPx: number): Promise<void> {
  await page.evaluate((keyboardPx) => {
    const vv = window.visualViewport;
    if (!vv) throw new Error('visualViewport is not available in this browser context');
    Object.defineProperty(vv, 'height', {
      configurable: true,
      get: () => window.innerHeight - keyboardPx,
    });
    vv.dispatchEvent(new Event('resize'));
  }, heightPx);
}
